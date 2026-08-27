package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/settings"
)

// newUploadTestServer 与 newTestServer 的区别：上传测试需要落盘与可预测的
// 外链域名，故不用共享 newTestServer 的 config.Load("") 默认值
// （BaseURL=http://localhost:8686、DataDir=./data 相对路径——会在仓库里
// 写测试文件），而是仿照 upload_test.go 的 setup() 显式指定临时目录与
// img.li 域名（见命名约定）。
func newUploadTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{BaseURL: "https://img.li", DataDir: t.TempDir()}
	return New(Options{Cfg: cfg, DB: model.TestDB(t)})
}

func pngBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{1, 2, 3, 255})
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func uploadReq(t *testing.T, field, filename string, content []byte) (*http.Request, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile(field, filename)
	fw.Write(content)
	mw.Close()
	req := httptest.NewRequest("POST", "/api/v1/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req, mw.FormDataContentType()
}

// uploadToken 注册并签发 upload-scope token。
func uploadToken(t *testing.T, s *Server) string {
	t.Helper()
	sess := register(t, s)
	_, e := doJSON(t, s, "POST", "/api/v1/user/tokens", `{"name":"picgo","scope":"upload"}`, []*http.Cookie{sess})
	var d struct {
		Token string `json:"token"`
	}
	json.Unmarshal(e.Data, &d)
	return d.Token
}

func TestUploadViaBearerReturnsResolvableLinks(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)

	req, _ := uploadReq(t, "file", "shot.png", pngBytes(300, 200))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	var d struct {
		Key   string `json:"key"`
		Size  int64  `json:"size"`
		Links struct {
			URL, Markdown, ThumbnailURL string
		} `json:"links"`
	}
	json.Unmarshal(e.Data, &d)
	if d.Key == "" || d.Links.URL == "" {
		t.Fatalf("响应缺字段: %s", rec.Body.String())
	}
	if d.Links.URL != "https://img.li/i/"+d.Key+".png" {
		t.Errorf("URL=%q", d.Links.URL)
	}
}

func TestUploadInstantSecondTime(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	content := pngBytes(300, 200)

	up := func() env {
		req, _ := uploadReq(t, "file", "x.png", content)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		var e env
		json.Unmarshal(rec.Body.Bytes(), &e)
		return e
	}
	up()
	var d struct {
		Instant bool `json:"instant"`
	}
	json.Unmarshal(up().Data, &d)
	if !d.Instant {
		t.Error("第二次相同内容应秒传 instant=true")
	}
}

// TestUploadEnqueuesModerateImageTask 验证②d Task 7 的管线接入：上传成功（新文件路径）
// 后应投递一条 moderate_image 任务（入队恒做，disabled/enabled 判断在任务执行时——
// 裁决 10），供 task.Runner 异步跑机审。
func TestUploadEnqueuesModerateImageTask(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)

	var task model.Task
	if err := s.opts.DB.Where("type = ?", "moderate_image").First(&task).Error; err != nil {
		t.Fatalf("未找到 moderate_image 任务: %v", err)
	}
	var p struct {
		ImageID uint64 `json:"image_id"`
	}
	if err := json.Unmarshal([]byte(task.Payload), &p); err != nil {
		t.Fatalf("payload 解析失败: %v, payload=%s", err, task.Payload)
	}
	var img model.Image
	if err := s.opts.DB.First(&img, "key = ?", key).Error; err != nil {
		t.Fatal(err)
	}
	if p.ImageID != img.ID {
		t.Errorf("task image_id = %d, want %d", p.ImageID, img.ID)
	}
}

// TestUploadInstantAlsoEnqueuesModerateImageTask 秒传路径同样要投递（裁决 10 明确
// 两条成功路径——新文件+秒传——都要入队）。
func TestUploadInstantAlsoEnqueuesModerateImageTask(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	content := pngBytes(300, 200)
	up := func() env {
		req, _ := uploadReq(t, "file", "dup.png", content)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		var e env
		json.Unmarshal(rec.Body.Bytes(), &e)
		return e
	}
	up()
	e2 := up()
	var d struct {
		Key     string `json:"key"`
		Instant bool   `json:"instant"`
	}
	json.Unmarshal(e2.Data, &d)
	if !d.Instant {
		t.Fatal("第二次相同内容应秒传")
	}

	var img model.Image
	if err := s.opts.DB.First(&img, "key = ?", d.Key).Error; err != nil {
		t.Fatal(err)
	}
	var tasks []model.Task
	s.opts.DB.Where("type = ?", "moderate_image").Find(&tasks)
	found := false
	for _, task := range tasks {
		var p struct {
			ImageID uint64 `json:"image_id"`
		}
		json.Unmarshal([]byte(task.Payload), &p)
		if p.ImageID == img.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("秒传图 image_id=%d 应有对应 moderate_image 任务, tasks=%+v", img.ID, tasks)
	}
}

// PicGo 契约：lankong 插件 POST multipart 字段名 file，Bearer 认证，读取 data.links.url
func TestPicGoContract(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	req, _ := uploadReq(t, "file", "picgo-upload.png", pngBytes(100, 100))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	if !e.Status {
		t.Fatalf("PicGo 期望 status:true, got %s", rec.Body.String())
	}
	var d struct {
		Links struct{ URL string } `json:"links"`
	}
	json.Unmarshal(e.Data, &d)
	if d.Links.URL == "" {
		t.Error("PicGo 需要 data.links.url")
	}
}

func TestUploadExtNotAllowed(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	// 上传纯文本冒充 png
	req, _ := uploadReq(t, "file", "x.png", []byte("not an image"))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非图片应 400, got %d %s", rec.Code, rec.Body.String())
	}
}

// TestUploadAnonReachesHandlerGuestDisabled 计划 C-①a Task 4：/upload 对匿名开放
// （不再有 RequireUser 401 拦截），但游客开关默认关闭，故 Save 内部拒绝——
// 应是 403 CodeForbidden（游客上传暂未开放），而不是 401（未登录）或 500（内部错误）。
func TestUploadAnonReachesHandlerGuestDisabled(t *testing.T) {
	s := newUploadTestServer(t)
	req, _ := uploadReq(t, "file", "x.png", pngBytes(10, 10))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("游客关闭时匿名上传应 403, got %d %s", rec.Code, rec.Body.String())
	}
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	if code(t, e) != "forbidden" {
		t.Errorf("code = %q, want forbidden", code(t, e))
	}
	if e.Message != "游客上传暂未开放" {
		t.Errorf("message = %q", e.Message)
	}
}

// TestUploadAnonSucceedsWhenGuestEnabled 游客开关打开后，匿名上传应成功，且
// 落库的 image.user_id 为 NULL（游客图无主）。
func TestUploadAnonSucceedsWhenGuestEnabled(t *testing.T) {
	s := newUploadTestServer(t)
	if err := settings.New(s.opts.DB).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	req, _ := uploadReq(t, "file", "x.png", pngBytes(10, 10))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("游客开启时匿名上传应成功, got %d %s", rec.Code, rec.Body.String())
	}
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	var d struct {
		Key string `json:"key"`
	}
	json.Unmarshal(e.Data, &d)
	var img model.Image
	if err := s.opts.DB.First(&img, "key = ?", d.Key).Error; err != nil {
		t.Fatal(err)
	}
	if img.UserID != nil {
		t.Errorf("游客图 user_id 应为 NULL, got %v", *img.UserID)
	}
}

// TestUploadAnonRateLimitedByGuestGroup 匿名连传超游客组 rate_per_day（种子=3）
// 应在第 4 次被三档限速中间件拦截 429（验证 GroupMiddleware 已接线到 /upload，
// 而不再是旧的粗兜底 Middleware("upload",120)）。
func TestUploadAnonRateLimitedByGuestGroup(t *testing.T) {
	s := newUploadTestServer(t)
	if err := settings.New(s.opts.DB).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	up := func() int {
		req, _ := uploadReq(t, "file", "x.png", pngBytes(10, 10))
		req.RemoteAddr = "8.8.4.4:1"
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 3; i++ {
		if c := up(); c != http.StatusOK {
			t.Fatalf("第 %d 次应成功, got %d", i+1, c)
		}
	}
	if c := up(); c != http.StatusTooManyRequests {
		t.Errorf("第 4 次应 429（游客组 rate_per_day=3）, got %d", c)
	}
}

func TestUploadURLFetchRejectsSSRF(t *testing.T) {
	s := newUploadTestServer(t)
	sess := register(t, s)
	rec, e := doJSON(t, s, "POST", "/api/v1/upload/url", `{"url":"http://127.0.0.1/x.png"}`, []*http.Cookie{sess})
	if rec.Code != http.StatusBadRequest || code(t, e) != "invalid_request" {
		t.Errorf("SSRF 应 400 invalid_request, got %d %s", rec.Code, rec.Body.String())
	}
}

// --- 复审修复①：出示了但解析不出身份的凭证（过期/坏 session、坏 Bearer）
// 必须 401，不能被 Auth 悄悄当无凭证放行成游客上传 ---

// TestUploadInvalidSessionCookieReturns401 携带一个不存在/已过期的 session cookie
// 上传：Auth 解析不出 principal，但凭证确实"被出示"了，RequireUserOrAnon 应拦成
// 401（而不是静默滑进游客分支，造成 200 的匿名上传或困惑的 403）。
func TestUploadInvalidSessionCookieReturns401(t *testing.T) {
	s := newUploadTestServer(t)
	req, _ := uploadReq(t, "file", "x.png", pngBytes(10, 10))
	req.AddCookie(&http.Cookie{Name: "imgli_session", Value: "expired-or-bogus-session-token"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("失效 session cookie 应 401, got %d %s", rec.Code, rec.Body.String())
	}
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	if code(t, e) != "unauthorized" {
		t.Errorf("code = %q, want unauthorized", code(t, e))
	}
}

// TestUploadInvalidBearerTokenReturns401 携带一个拼错/已吊销的 Bearer token 上传，
// 同上应 401，不落入游客分支。
func TestUploadInvalidBearerTokenReturns401(t *testing.T) {
	s := newUploadTestServer(t)
	req, _ := uploadReq(t, "file", "x.png", pngBytes(10, 10))
	req.Header.Set("Authorization", "Bearer garbage-token-that-does-not-exist")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无效 Bearer token 应 401, got %d %s", rec.Code, rec.Body.String())
	}
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	if code(t, e) != "unauthorized" {
		t.Errorf("code = %q, want unauthorized", code(t, e))
	}
}

// TestUploadURLInvalidBearerTokenReturns401 同上，覆盖 /upload/url 路由（与
// /upload 共用同一 RequireUserOrAnon 中间件，两条路由都要生效）。
func TestUploadURLInvalidBearerTokenReturns401(t *testing.T) {
	s := newUploadTestServer(t)
	req := httptest.NewRequest("POST", "/api/v1/upload/url", nil)
	req.Header.Set("Authorization", "Bearer garbage-token-that-does-not-exist")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无效 Bearer token 应 401, got %d %s", rec.Code, rec.Body.String())
	}
}

// --- 复审修复②：/upload(/url) 游客关闭时须在抓取/落盘前就 403 ---

// TestUploadURLAnonGuestDisabledSkipsFetch 游客开关关闭（默认）时，匿名
// /upload/url 应在发起远程抓取之前就 403——用一个计数远程请求次数的测试服务器
// 验证请求真的一次都没发出去（而不是先抓完/校验完 SSRF 再拒绝）。
func TestUploadURLAnonGuestDisabledSkipsFetch(t *testing.T) {
	s := newUploadTestServer(t)
	fetched := false
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched = true
		w.WriteHeader(http.StatusOK)
	}))
	defer remote.Close()

	rec, e := doJSON(t, s, "POST", "/api/v1/upload/url", `{"url":"`+remote.URL+`/x.png"}`, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("游客关闭时匿名 /upload/url 应 403, got %d %s", rec.Code, rec.Body.String())
	}
	if code(t, e) != "forbidden" {
		t.Errorf("code = %q, want forbidden", code(t, e))
	}
	if fetched {
		t.Error("游客关闭应在抓取前 403，不应向远程发起请求")
	}
}

func heicFtyp() []byte {
	b := make([]byte, 16)
	b[3] = 16
	copy(b[4:8], "ftyp")
	copy(b[8:12], "heic")
	return b
}

// TestUploadHEICUnsupportedOnPureGo H2：组已允许 heic 时，纯 Go 构建对 16 字节
// ftyp 夹具返回 415 heic_unsupported（不是 ext_not_allowed / invalid_request）。
func TestUploadHEICUnsupportedOnPureGo(t *testing.T) {
	if imaging.HeicDecodeAvailable() {
		t.Skip("vips+heif")
	}
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	if err := s.opts.DB.Model(&model.UserGroup{}).Where("is_default = ?", true).
		Update("allowed_exts", `["png","jpg","jpeg","gif","webp","heic","heif"]`).Error; err != nil {
		t.Fatal(err)
	}
	req, _ := uploadReq(t, "file", "a.heic", heicFtyp())
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415: %s", rec.Code, rec.Body.String())
	}
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	if code(t, e) != "heic_unsupported" {
		t.Errorf("code = %q, want heic_unsupported; body=%s", code(t, e), rec.Body.String())
	}
}
