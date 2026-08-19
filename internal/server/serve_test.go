package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
)

// uploadAndGetKey 上传一张图返回其 key（复用 api_upload_test.go 的助手）。
func uploadAndGetKey(t *testing.T, s *Server, tok string) string {
	t.Helper()
	req, _ := uploadReq(t, "file", "shot.png", pngBytes(120, 90))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	var d struct {
		Key string `json:"key"`
	}
	json.Unmarshal(e.Data, &d)
	if d.Key == "" {
		t.Fatalf("upload failed: %s", rec.Body.String())
	}
	return d.Key
}

func TestServeOriginalImage(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/i/"+key+".png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("直链应 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type=%q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("应返回图片字节")
	}
}

func TestServeThumbnail(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/t/"+key+".jpg", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("缩略图应 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("缩略图 Content-Type=%q", ct)
	}
}

func TestServeThumbnailGeneratesWhenMissing(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)

	var img model.Image
	if err := s.opts.DB.Where("key = ?", key).First(&img).Error; err != nil {
		t.Fatal(err)
	}
	var file model.File
	if err := s.opts.DB.First(&file, img.FileID).Error; err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(s.opts.Cfg.DataDir, "uploads")
	for _, tk := range storagesvc.ThumbKeyCandidates(file.Surface, file.Hash) {
		_ = os.Remove(filepath.Join(root, filepath.FromSlash(tk)))
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/t/"+key+".jpg", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("缺省 /t 应从原图现场生成, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" && ct != "image/webp" {
		t.Errorf("Content-Type=%q", ct)
	}
	if rec.Body.Len() < 32 {
		t.Errorf("生成缩略图过短: %d", rec.Body.Len())
	}
}

func TestServeThumbnailDriverErrorIsSVG(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)
	if err := s.opts.DB.Model(&model.StoragePolicy{}).Where("id = 1").Update("driver", "nope").Error; err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/t/"+key+".jpg", nil)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("存储不可用应 500, got %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg") {
		t.Errorf("图片 500 应为 SVG, got %q body=%s", ct, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("500 占位不应缓存, Cache-Control=%q", cc)
	}
}

func TestServeNotFound(t *testing.T) {
	s := newUploadTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/i/nonexistent0.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在应 404, got %d", rec.Code)
	}
}

func TestServePrivateImageBlocked(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	// 上传一张私密图
	req, _ := uploadReq(t, "file", "secret.png", pngBytes(120, 90))
	req.Header.Set("Authorization", "Bearer "+tok)
	// multipart 里加 visibility=private 较繁琐，这里上传后直接改库
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	var d struct {
		Key string `json:"key"`
	}
	json.Unmarshal(e.Data, &d)
	s.opts.DB.Exec("UPDATE images SET visibility = 'private' WHERE key = ?", d.Key)

	// 匿名访问私密图 → 401
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/i/"+d.Key+".png", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("匿名访问私密图应 401, got %d", rec.Code)
	}
}

func TestServeDeletedImageGone(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)
	// 软删
	s.opts.DB.Exec("UPDATE images SET deleted_at = CURRENT_TIMESTAMP WHERE key = ?", key)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/i/"+key+".png", nil))
	if rec.Code != http.StatusGone {
		t.Errorf("软删图应 410, got %d", rec.Code)
	}
}

func TestServeRejectedImageGone(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)
	s.opts.DB.Exec("UPDATE images SET status = 'rejected' WHERE key = ?", key)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/i/"+key+".png", nil))
	if rec.Code != http.StatusGone {
		t.Errorf("rejected 图直链应 410, got %d", rec.Code)
	}
}

func TestServeRejectedThumbnailGone(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)
	s.opts.DB.Exec("UPDATE images SET status = 'rejected' WHERE key = ?", key)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/t/"+key+".jpg", nil))
	if rec.Code != http.StatusGone {
		t.Errorf("rejected 图缩略图应 410, got %d", rec.Code)
	}
}

func TestServePendingImageBlockedForAnonymous(t *testing.T) {
	// 内容安全 P1：pending 仅属主可看，匿名外链 410。
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)
	s.opts.DB.Exec("UPDATE images SET status = 'pending' WHERE key = ?", key)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/i/"+key+".png", nil))
	if rec.Code != http.StatusGone {
		t.Errorf("pending 匿名直链应 410, got %d", rec.Code)
	}
}

func TestServePendingThumbnailBlockedForAnonymous(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)
	s.opts.DB.Exec("UPDATE images SET status = 'pending' WHERE key = ?", key)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/t/"+key+".jpg", nil))
	if rec.Code != http.StatusGone {
		t.Errorf("pending 匿名缩略图应 410, got %d", rec.Code)
	}
}

func TestServePendingOwnerCanView(t *testing.T) {
	// 属主带 session 可看自己的 pending；他人 session 仍 410。
	s := newUploadTestServer(t)
	ownerSess := registerAndCookie(t, s, "owner1", "owner1@img.li", "passw0rd")
	_, e := doJSON(t, s, "POST", "/api/v1/user/tokens", `{"name":"picgo","scope":"upload"}`, []*http.Cookie{ownerSess})
	var d struct {
		Token string `json:"token"`
	}
	json.Unmarshal(e.Data, &d)
	key := uploadAndGetKey(t, s, d.Token)
	s.opts.DB.Exec("UPDATE images SET status = 'pending' WHERE key = ?", key)

	req := httptest.NewRequest("GET", "/i/"+key+".png", nil)
	req.AddCookie(ownerSess)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("属主看 pending 应 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	otherSess := registerAndCookie(t, s, "other2", "other2@img.li", "passw0rd")
	req2 := httptest.NewRequest("GET", "/i/"+key+".png", nil)
	req2.AddCookie(otherSess)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusGone {
		t.Errorf("非属主看 pending 应 410, got %d", rec2.Code)
	}
}

func TestServePlaceholderIsImageForImageRequest(t *testing.T) {
	s := newUploadTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/i/nope12345678.png", nil)
	req.Header.Set("Accept", "image/*")
	s.Handler().ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg") {
		t.Errorf("图片请求的占位应为 SVG 图片, got %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("占位图不应被缓存, Cache-Control=%q", cc)
	}
}

func TestServeDeletedThumbnailGone(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)
	// 软删
	s.opts.DB.Exec("UPDATE images SET deleted_at = CURRENT_TIMESTAMP WHERE key = ?", key)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/t/"+key+".jpg", nil))
	if rec.Code != http.StatusGone {
		t.Errorf("软删缩略图应 410, got %d", rec.Code)
	}
}

func TestServePlaceholderJSONForAPIClient(t *testing.T) {
	s := newUploadTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/i/missingkey123.png", nil)
	req.Header.Set("Accept", "application/json")
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("显式 Accept: application/json 应 404, got %d", rec.Code)
	}
	var b struct {
		Status bool `json:"status"`
		Data   struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("响应体应可解析为信封 JSON: %v, body=%s", err, rec.Body.String())
	}
	if b.Status {
		t.Errorf("status 应为 false, body=%s", rec.Body.String())
	}
	if b.Data.Code != "not_found" {
		t.Errorf("data.code=%q, want not_found", b.Data.Code)
	}
}

func TestServeDedupThumbnailResolves(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	content := pngBytes(300, 200)
	up := func() string {
		req, _ := uploadReq(t, "file", "x.png", content)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		var e env
		json.Unmarshal(rec.Body.Bytes(), &e)
		var d struct {
			Key     string `json:"key"`
			Instant bool   `json:"instant"`
		}
		json.Unmarshal(e.Data, &d)
		return d.Key
	}
	up()         // first upload writes thumbnail under file hash
	key2 := up() // dedup image: different image key, same file
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/t/"+key2+".jpg", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("去重图缩略图应 200(按文件哈希共享), got %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type=%q", ct)
	}
}

func TestServeCacheControlByVisibility(t *testing.T) {
	s := newUploadTestServer(t)
	tok := uploadToken(t, s)
	key := uploadAndGetKey(t, s, tok)

	// 公开图 → 可被共享缓存 + ETag
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/i/"+key+".png", nil))
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("公开图 Cache-Control=%q，应含 immutable", cc)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" || etag == `""` {
		t.Fatalf("公开图应有 ETag, got %q", etag)
	}
	// If-None-Match → 304
	req304 := httptest.NewRequest("GET", "/i/"+key+".png", nil)
	req304.Header.Set("If-None-Match", etag)
	rec304 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match 应 304, got %d", rec304.Code)
	}

	// 转私密后，所有者访问 → 禁止共享缓存
	s.opts.DB.Exec("UPDATE images SET visibility = 'private' WHERE key = ?", key)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/i/"+key+".png", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("所有者访问私密图应 200, got %d", rec.Code)
	}
	cc := rec.Header().Get("Cache-Control")
	if !strings.Contains(cc, "private") {
		t.Errorf("私密图 Cache-Control=%q，应含 private", cc)
	}
	if strings.Contains(cc, "immutable") {
		t.Errorf("私密图 Cache-Control=%q，不应含 immutable", cc)
	}
}
