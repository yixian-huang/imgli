package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"

	"gorm.io/gorm"
)

// adminTestServer 起全量 server + 注册两用户（首个自动 admin，第二个普通），返回各自 session cookie。
func adminTestServer(t *testing.T) (s *Server, adminCookie, userCookie *http.Cookie) {
	t.Helper()
	cfg, _ := config.Load("")
	db := model.TestDB(t)
	s = New(Options{Cfg: cfg, DB: db})
	adminCookie = registerAndCookie(t, s, "boss", "boss@x.li", "passw0rd1")
	userCookie = registerAndCookie(t, s, "pleb", "pleb@x.li", "passw0rd1")
	return
}

// registerAndCookie 注册一个用户并返回其 session cookie（复用 api_auth_test.go 的 doJSON 助手）。
func registerAndCookie(t *testing.T, s *Server, name, email, pwd string) *http.Cookie {
	t.Helper()
	rec, _ := doJSON(t, s, "POST", "/api/v1/auth/register",
		`{"username":"`+name+`","email":"`+email+`","password":"`+pwd+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("register %s = %d: %s", name, rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "imgli_session" {
			return c
		}
	}
	t.Fatal("无 session cookie")
	return nil
}

func TestAdminUsersListFilters(t *testing.T) {
	s, admin, user := adminTestServer(t)
	_ = user

	// 非 admin 403
	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/users", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin = %d, want 403", rec.Code)
	}

	// admin 全量列表：boss + pleb 共 2 人
	rec, e := doJSON(t, s, "GET", "/api/v1/admin/users", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
		Page  int              `json:"page"`
		Limit int              `json:"limit"`
	}
	if err := json.Unmarshal(e.Data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Errorf("total=%d items=%d, want 2/2", page.Total, len(page.Items))
	}
	if page.Page != 1 || page.Limit != 50 {
		t.Errorf("page=%d limit=%d, want 1/50 默认", page.Page, page.Limit)
	}
	for _, it := range page.Items {
		if _, ok := it["image_count"]; !ok {
			t.Errorf("item 缺 image_count: %+v", it)
		}
	}

	// q 筛选
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/users?q=pleb", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("q filter = %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(e.Data, &page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0]["username"] != "pleb" {
		t.Errorf("q=pleb: total=%d items=%+v", page.Total, page.Items)
	}

	// status 筛选
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/users?status=banned", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("status filter = %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(e.Data, &page)
	if page.Total != 0 {
		t.Errorf("status=banned 初始应为 0, got %d", page.Total)
	}

	// limit 上限 200
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/users?limit=500", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("limit clamp = %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(e.Data, &page)
	if page.Limit != 200 {
		t.Errorf("limit=500 应截至 200, got %d", page.Limit)
	}
}

// userIDFromSession 从 /auth/session 读出当前登录用户 ID（用于按 id 构造 PATCH 路径）。
func userIDFromSession(t *testing.T, s *Server, cookie *http.Cookie) uint64 {
	t.Helper()
	rec, e := doJSON(t, s, "GET", "/api/v1/auth/session", "", []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("session = %d: %s", rec.Code, rec.Body.String())
	}
	var u struct {
		ID uint64 `json:"id"`
	}
	json.Unmarshal(e.Data, &u)
	if u.ID == 0 {
		t.Fatal("session 未返回 id")
	}
	return u.ID
}

func TestAdminUpdateUserSelfBanRejected(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	adminID := userIDFromSession(t, s, admin)

	rec, e := doJSON(t, s, "PATCH", "/api/v1/admin/users/"+itoa(adminID),
		`{"status":"banned"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("自封禁 = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	_ = e
}

func TestAdminUpdateUserNotFoundAndBadGroup(t *testing.T) {
	s, admin, user := adminTestServer(t)

	rec, _ := doJSON(t, s, "PATCH", "/api/v1/admin/users/999999",
		`{"status":"active"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在用户 = %d, want 404", rec.Code)
	}

	userID := userIDFromSession(t, s, user)
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/users/"+itoa(userID),
		`{"group_id":999999}`, []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在组 = %d, want 404", rec.Code)
	}
}

func itoa(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// TestAdminBanChain 验证封禁连锁：admin 把普通用户置 banned 后，
// 该用户带旧 cookie 访问受保护路由应被拒绝（②a UserBySession 对 banned 用户
// 返回 (nil,nil)——视同匿名，由 RequireUser 统一拦截为 401 unauthorized；
// ②a 设计明确注释为“无效/过期/封禁一律 (nil, nil)——匿名继续，由上层拦截”，
// 故此处按该既有语义断言 401，而非登录端点专用的 403 banned）。
func TestAdminBanChain(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)

	rec, e := doJSON(t, s, "PATCH", "/api/v1/admin/users/"+itoa(pleb),
		`{"status":"banned"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("ban = %d: %s", rec.Code, rec.Body.String())
	}
	var updated struct {
		Status string `json:"status"`
	}
	json.Unmarshal(e.Data, &updated)
	if updated.Status != "banned" {
		t.Errorf("响应 status = %s, want banned", updated.Status)
	}

	// 被封用户带旧 cookie 访问受保护路由：ban 未清 session 行，但 UserBySession
	// 判 banned 即返回匿名，RequireUser 统一 401。
	rec, _ = doJSON(t, s, "GET", "/api/v1/auth/session", "", []*http.Cookie{user})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("被封用户访问 /auth/session = %d, want 401", rec.Code)
	}

	// 被封用户登录也应被拒绝（403 banned，②a 既有行为）
	rec, e = doJSON(t, s, "POST", "/api/v1/auth/login", `{"account":"pleb","password":"passw0rd1"}`, nil)
	if rec.Code != http.StatusForbidden || code(t, e) != "banned" {
		t.Errorf("被封用户登录 = %d %s, want 403 banned", rec.Code, rec.Body.String())
	}

	// audit 落 user_update
	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "user_update").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("audit user_update 行数 = %d, want 1", len(logs))
	}
	if logs[0].ActorType != "admin" {
		t.Errorf("audit actor_type = %s, want admin", logs[0].ActorType)
	}
}

func TestAdminUpdateUserGroupAndAudit(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)

	vip := model.UserGroup{Name: "vip", StorageQuota: 1 << 30}
	if err := s.opts.DB.Create(&vip).Error; err != nil {
		t.Fatal(err)
	}

	rec, e := doJSON(t, s, "PATCH", "/api/v1/admin/users/"+itoa(pleb),
		`{"group_id":`+itoa(vip.ID)+`}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("change group = %d: %s", rec.Code, rec.Body.String())
	}
	var updated struct {
		GroupID uint64 `json:"group_id"`
	}
	json.Unmarshal(e.Data, &updated)
	if updated.GroupID != vip.ID {
		t.Errorf("group_id = %d, want %d", updated.GroupID, vip.ID)
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "user_update").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("audit 行数 = %d, want 1", len(logs))
	}
	if strings.Contains(logs[0].Detail, "password") {
		t.Errorf("audit detail 不应含密码相关字段: %s", logs[0].Detail)
	}
}

// TestAdminResetPasswordFlow 验证重置密码：响应含 >=16 位明文；旧密码登录 401；
// 新密码登录 200；且强制登出（重置前的 session 立即失效）。
func TestAdminResetPasswordFlow(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)

	rec, e := doJSON(t, s, "POST", "/api/v1/admin/users/"+itoa(pleb)+"/reset-password",
		"", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("reset = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Password string `json:"password"`
	}
	json.Unmarshal(e.Data, &resp)
	if len(resp.Password) < 16 {
		t.Errorf("password 长度 = %d, want >=16", len(resp.Password))
	}

	// 旧 session 应已失效（强制登出）
	rec, _ = doJSON(t, s, "GET", "/api/v1/auth/session", "", []*http.Cookie{user})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("重置密码后旧 session = %d, want 401", rec.Code)
	}

	// 旧密码登录 401
	rec, e = doJSON(t, s, "POST", "/api/v1/auth/login", `{"account":"pleb","password":"passw0rd1"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("旧密码登录 = %d, want 401: %s", rec.Code, rec.Body.String())
	}

	// 新密码登录 200
	rec, _ = doJSON(t, s, "POST", "/api/v1/auth/login",
		`{"account":"pleb","password":"`+resp.Password+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("新密码登录 = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// audit 落 user_reset_password，且不含密码明文/哈希
	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "user_reset_password").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("audit 行数 = %d, want 1", len(logs))
	}
	if strings.Contains(logs[0].Detail, resp.Password) {
		t.Errorf("audit detail 不得含密码明文: %s", logs[0].Detail)
	}
}

func TestAdminResetPasswordNotFound(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	rec, _ := doJSON(t, s, "POST", "/api/v1/admin/users/999999/reset-password", "", []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在用户 reset = %d, want 404", rec.Code)
	}
}

func TestAdminStatsGate(t *testing.T) {
	s, admin, user := adminTestServer(t)

	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/stats", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin = %d, want 403", rec.Code)
	}

	rec, e := doJSON(t, s, "GET", "/api/v1/admin/stats", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("admin = %d: %s", rec.Code, rec.Body.String())
	}
	var data struct {
		Users        int64            `json:"users"`
		Images       int64            `json:"images"`
		Storage      int64            `json:"storage"`
		TodayUploads int64            `json:"today_uploads"`
		Daily        []map[string]any `json:"daily"`
	}
	if err := json.Unmarshal(e.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Users != 2 {
		t.Errorf("users = %d, want 2", data.Users)
	}
}

// seedAdminImage 直接建 file+image 记录（测试夹具，同 T1 测试模式，绕过真实上传服务）。
func seedAdminImage(t *testing.T, s *Server, key string, userID uint64, status string) {
	t.Helper()
	f := &model.File{Hash: key + "hash", StoragePolicyID: 1, Path: "p/" + key, Size: 100, MIME: "image/png", RefCount: 1}
	if err := s.opts.DB.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: key, UserID: &userID, FileID: f.ID, Name: key, Ext: "png", Visibility: "public", Status: status}
	if err := s.opts.DB.Create(img).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAdminImagesListNonAdmin403(t *testing.T) {
	s, _, user := adminTestServer(t)
	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/images", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin = %d, want 403", rec.Code)
	}
}

func TestAdminImagesListFilters(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "img0000000001", pleb, "normal")
	seedAdminImage(t, s, "img0000000002", pleb, "pending")

	rec, e := doJSON(t, s, "GET", "/api/v1/admin/images", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	json.Unmarshal(e.Data, &page)
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("total=%d items=%d, want 2/2", page.Total, len(page.Items))
	}
	for _, it := range page.Items {
		for _, field := range []string{"key", "name", "ext", "size", "visibility", "status",
			"is_whitelisted", "nsfw_score", "username", "user_id", "created_at", "links",
			"policy_id", "policy_name", "policy_driver", "surface", "path", "in_trash"} {
			if _, ok := it[field]; !ok {
				t.Errorf("item 缺字段 %s: %+v", field, it)
			}
		}
		if it["username"] != "pleb" {
			t.Errorf("username = %v, want pleb", it["username"])
		}
		// seedAdminImage 默认 StoragePolicyID=1, Path="p/"+key
		if n, ok := it["policy_id"].(float64); !ok || n != 1 {
			t.Errorf("policy_id = %v, want 1", it["policy_id"])
		}
		if path, _ := it["path"].(string); path == "" || !strings.HasPrefix(path, "p/") {
			t.Errorf("path = %v, want p/<key>", it["path"])
		}
		if name, _ := it["policy_name"].(string); name == "" {
			t.Errorf("policy_name 应非空: %+v", it)
		}
		if it["in_trash"] != false {
			t.Errorf("live 列表 in_trash 应为 false, got %v", it["in_trash"])
		}
	}

	// status 筛选
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/images?status=pending", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("status filter = %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(e.Data, &page)
	if page.Total != 1 || page.Items[0]["key"] != "img0000000002" {
		t.Errorf("status=pending: total=%d items=%+v", page.Total, page.Items)
	}

	// 非法 status → 400
	rec, _ = doJSON(t, s, "GET", "/api/v1/admin/images?status=bogus", "", []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 status = %d, want 400", rec.Code)
	}

	// user 筛选
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/images?user="+itoa(pleb), "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("user filter = %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(e.Data, &page)
	if page.Total != 2 {
		t.Errorf("user filter total=%d, want 2", page.Total)
	}
}

// TestAdminGuestDeleteIsPermanent 游客图无回收站：默认 DELETE 即彻底删除。
func TestAdminGuestDeleteIsPermanent(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	// userID=0 在 seed 里会写成 &0；改用直接插入 nil UserID
	f := &model.File{Hash: "guesthash0001", StoragePolicyID: 1, Path: "p/guestkey00001", Size: 100, MIME: "image/png", RefCount: 1}
	if err := s.opts.DB.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "guestkey00001", UserID: nil, FileID: f.ID, Name: "g", Ext: "png", Visibility: "public", Status: "normal"}
	if err := s.opts.DB.Create(img).Error; err != nil {
		t.Fatal(err)
	}

	rec, e := doJSON(t, s, "DELETE", "/api/v1/admin/images/guestkey00001", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete guest = %d: %s", rec.Code, rec.Body.String())
	}
	var d struct {
		Permanent bool `json:"permanent"`
		Deleted   bool `json:"deleted"`
	}
	json.Unmarshal(e.Data, &d)
	if !d.Deleted || !d.Permanent {
		t.Errorf("游客删除应 permanent: %+v", d)
	}
	var cnt int64
	s.opts.DB.Unscoped().Model(&model.Image{}).Where("key = ?", "guestkey00001").Count(&cnt)
	if cnt != 0 {
		t.Errorf("游客图应硬删, still %d", cnt)
	}
}

// TestAdminPermanentDeletePurgesFile 管理端 permanent=1 硬删 image+file 行（物理 delete_file 任务异步）。
func TestAdminPermanentDeletePurgesFile(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "prgekey000001", pleb, "normal")

	var beforeFiles int64
	s.opts.DB.Model(&model.File{}).Where("path = ?", "p/prgekey000001").Count(&beforeFiles)
	if beforeFiles != 1 {
		t.Fatalf("seed file count = %d", beforeFiles)
	}

	rec, e := doJSON(t, s, "DELETE", "/api/v1/admin/images/prgekey000001?permanent=1", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("purge = %d: %s", rec.Code, rec.Body.String())
	}
	var d struct {
		Key       string `json:"key"`
		Deleted   bool   `json:"deleted"`
		Permanent bool   `json:"permanent"`
	}
	json.Unmarshal(e.Data, &d)
	if !d.Deleted || !d.Permanent || d.Key != "prgekey000001" {
		t.Errorf("响应 = %+v", d)
	}

	var imgCnt, fileCnt int64
	s.opts.DB.Unscoped().Model(&model.Image{}).Where("key = ?", "prgekey000001").Count(&imgCnt)
	s.opts.DB.Model(&model.File{}).Where("path = ?", "p/prgekey000001").Count(&fileCnt)
	if imgCnt != 0 {
		t.Errorf("image 行应硬删, still %d", imgCnt)
	}
	if fileCnt != 0 {
		t.Errorf("file 行应删(ref 归零), still %d", fileCnt)
	}

	// 属主回收站也不应再有
	_, e = doJSON(t, s, "GET", "/api/v1/trash", "", []*http.Cookie{user})
	var tr struct {
		Items []struct {
			Key string `json:"key"`
		} `json:"items"`
	}
	json.Unmarshal(e.Data, &tr)
	for _, it := range tr.Items {
		if it.Key == "prgekey000001" {
			t.Error("彻底删除后回收站不应再有该图")
		}
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "image_admin_purge").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("audit purge 行数 = %d, want 1", len(logs))
	}

	// 重复彻底删除 → 404
	rec, _ = doJSON(t, s, "DELETE", "/api/v1/admin/images/prgekey000001?permanent=1", "", []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("重复 purge = %d, want 404", rec.Code)
	}
}

// TestAdminSoftDeleteImageMovesToOwnerTrashAnd410 验证管理员软删他人图后：
// 属主 GET /trash 可见该图、原直链 GET /i/{key}.{ext} 返回 410、audit 落库、重复软删 404。
func TestAdminSoftDeleteImageMovesToOwnerTrashAnd410(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "delkey000001", pleb, "normal")

	rec, e := doJSON(t, s, "DELETE", "/api/v1/admin/images/delkey000001", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body.String())
	}
	var d struct {
		Key     string `json:"key"`
		Deleted bool   `json:"deleted"`
	}
	json.Unmarshal(e.Data, &d)
	if !d.Deleted || d.Key != "delkey000001" {
		t.Errorf("响应 = %+v", d)
	}

	// 属主回收站可见
	_, e = doJSON(t, s, "GET", "/api/v1/trash", "", []*http.Cookie{user})
	var tr struct {
		Items []struct {
			Key string `json:"key"`
		} `json:"items"`
	}
	json.Unmarshal(e.Data, &tr)
	found := false
	for _, it := range tr.Items {
		if it.Key == "delkey000001" {
			found = true
		}
	}
	if !found {
		t.Errorf("属主回收站应见该图: %+v", tr.Items)
	}

	// 原直链 410
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/i/delkey000001.png", nil))
	if rec2.Code != http.StatusGone {
		t.Errorf("直链 = %d, want 410", rec2.Code)
	}

	// audit 落 image_admin_delete
	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "image_admin_delete").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("audit 行数 = %d, want 1", len(logs))
	}
	if logs[0].ActorType != "admin" {
		t.Errorf("actor_type = %s, want admin", logs[0].ActorType)
	}
	if !strings.Contains(logs[0].Detail, "delkey000001") {
		t.Errorf("detail 应含 key: %s", logs[0].Detail)
	}

	// 重复软删 → 404（幂等裁决修正：已软删按 404 处理）
	rec, _ = doJSON(t, s, "DELETE", "/api/v1/admin/images/delkey000001", "", []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("重复软删 = %d, want 404", rec.Code)
	}
}

// TestAdminWhitelistRepositionsStatus 验证加白 pending 图后 status 复位 normal，
// 响应含 links，audit 落 image_whitelist，非 admin 403。
func TestAdminWhitelistRepositionsStatus(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "pendkey000001", pleb, "pending")

	rec, e := doJSON(t, s, "PATCH", "/api/v1/admin/images/pendkey000001",
		`{"is_whitelisted":true}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("whitelist = %d: %s", rec.Code, rec.Body.String())
	}
	var item struct {
		Status        string         `json:"status"`
		IsWhitelisted bool           `json:"is_whitelisted"`
		Links         map[string]any `json:"links"`
	}
	json.Unmarshal(e.Data, &item)
	if item.Status != "normal" {
		t.Errorf("status = %s, want normal（加白复位）", item.Status)
	}
	if !item.IsWhitelisted {
		t.Errorf("is_whitelisted = false, want true")
	}
	if len(item.Links) == 0 {
		t.Errorf("响应应含 links")
	}

	var got model.Image
	s.opts.DB.Where("key = ?", "pendkey000001").First(&got)
	if got.Status != "normal" || !got.IsWhitelisted {
		t.Errorf("db 中 status=%s is_whitelisted=%v", got.Status, got.IsWhitelisted)
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "image_whitelist").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("audit 行数 = %d, want 1", len(logs))
	}
	if logs[0].ActorType != "admin" {
		t.Errorf("actor_type = %s, want admin", logs[0].ActorType)
	}

	// 非 admin 403
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/images/pendkey000001",
		`{"is_whitelisted":false}`, []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin PATCH = %d, want 403", rec.Code)
	}
}

func TestAdminDeleteImageNotFound(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	rec, _ := doJSON(t, s, "DELETE", "/api/v1/admin/images/nosuchkey0001", "", []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在图片 delete = %d, want 404", rec.Code)
	}
}

// validGroupBody 是通过校验的最小合法组请求体（供各用例复用后按需覆盖字段）。
const validGroupBody = `{"name":"vip","storage_quota":1073741824,"max_file_size":10485760,` +
	`"rate_per_minute":10,"rate_per_hour":100,"rate_per_day":500,"allowed_exts":["png","jpg"]}`

func TestAdminGroupsListNonAdmin403(t *testing.T) {
	s, _, user := adminTestServer(t)
	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/groups", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin = %d, want 403", rec.Code)
	}
}

// TestAdminGroupsCRUDFlow 验证创建后 GET 列表可见含 user_count、PATCH 生效、
// 删除在用组 400、删除空组成功且 audit 落库。
func TestAdminGroupsCRUDFlow(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)

	rec, e := doJSON(t, s, "POST", "/api/v1/admin/groups", validGroupBody, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID               uint64   `json:"id"`
		Name             string   `json:"name"`
		IsDefault        bool     `json:"is_default"`
		IsGuest          bool     `json:"is_guest"`
		AllowedExts      []string `json:"allowed_exts"`
		AllowedPolicyIDs []uint64 `json:"allowed_policy_ids"`
	}
	if err := json.Unmarshal(e.Data, &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Name != "vip" || created.IsDefault || created.IsGuest {
		t.Errorf("created = %+v", created)
	}
	if len(created.AllowedExts) != 2 {
		t.Errorf("allowed_exts = %v, want 2 项", created.AllowedExts)
	}

	// GET 列表可见，含 user_count=0
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/groups", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(e.Data, &page); err != nil {
		t.Fatal(err)
	}
	var found map[string]any
	for _, it := range page.Items {
		if uint64(it["id"].(float64)) == created.ID {
			found = it
		}
	}
	if found == nil {
		t.Fatalf("列表中未找到新建组: %+v", page.Items)
	}
	if found["user_count"].(float64) != 0 {
		t.Errorf("user_count = %v, want 0", found["user_count"])
	}

	// PATCH 改配额
	rec, e = doJSON(t, s, "PATCH", "/api/v1/admin/groups/"+itoa(created.ID),
		`{"storage_quota":2147483648}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", rec.Code, rec.Body.String())
	}
	var patched struct {
		StorageQuota int64 `json:"storage_quota"`
	}
	json.Unmarshal(e.Data, &patched)
	if patched.StorageQuota != 2147483648 {
		t.Errorf("storage_quota = %d, want 2147483648", patched.StorageQuota)
	}

	// 把该组分配给一个用户后，删除应 400（在用）
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/users/"+itoa(pleb),
		`{"group_id":`+itoa(created.ID)+`}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("assign group = %d", rec.Code)
	}
	rec, _ = doJSON(t, s, "DELETE", "/api/v1/admin/groups/"+itoa(created.ID), "", []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("删在用组 = %d, want 400", rec.Code)
	}

	// 挪走用户后删除应成功
	var def model.UserGroup
	s.opts.DB.Where("is_default = ?", true).First(&def)
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/users/"+itoa(pleb),
		`{"group_id":`+itoa(def.ID)+`}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("挪走用户 = %d", rec.Code)
	}
	rec, e = doJSON(t, s, "DELETE", "/api/v1/admin/groups/"+itoa(created.ID), "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body.String())
	}
	var deleted struct {
		ID      uint64 `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	json.Unmarshal(e.Data, &deleted)
	if deleted.ID != created.ID || !deleted.Deleted {
		t.Errorf("deleted = %+v", deleted)
	}

	// audit 三条都落库
	var creates, updates, deletes []model.AuditLog
	s.opts.DB.Where("action = ?", "group_create").Find(&creates)
	s.opts.DB.Where("action = ?", "group_update").Find(&updates)
	s.opts.DB.Where("action = ?", "group_delete").Find(&deletes)
	if len(creates) != 1 || len(updates) != 1 || len(deletes) != 1 {
		t.Errorf("audit 行数: create=%d update=%d delete=%d, want 1/1/1", len(creates), len(updates), len(deletes))
	}
	if !strings.Contains(deletes[0].Detail, "vip") {
		t.Errorf("group_delete detail 应含组名: %s", deletes[0].Detail)
	}
}

func TestAdminGroupsValidationErrors(t *testing.T) {
	s, admin, _ := adminTestServer(t)

	// exts 为空 → 400
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/groups",
		`{"name":"bad","storage_quota":1,"max_file_size":1,"allowed_exts":[]}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("exts=[] create = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if code(t, e) != "invalid_request" {
		t.Errorf("code = %s, want invalid_request", code(t, e))
	}

	// 未知策略 id → 404
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/groups",
		`{"name":"bad2","storage_quota":1,"max_file_size":1,"allowed_exts":["png"],"allowed_policy_ids":[999999]}`,
		[]*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("未知策略 create = %d, want 404", rec.Code)
	}

	// 内置组改名 → 400
	var def model.UserGroup
	s.opts.DB.Where("is_default = ?", true).First(&def)
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/groups/"+itoa(def.ID),
		`{"name":"改了"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("内置组改名 = %d, want 400", rec.Code)
	}

	// 内置组改配额 → 200（允许）
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/groups/"+itoa(def.ID),
		`{"storage_quota":123456789}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Errorf("内置组改配额 = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// 删内置组 → 400
	rec, _ = doJSON(t, s, "DELETE", "/api/v1/admin/groups/"+itoa(def.ID), "", []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("删内置组 = %d, want 400", rec.Code)
	}

	// 不存在的组 PATCH/DELETE → 404
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/groups/999999", `{"name":"x"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在组 patch = %d, want 404", rec.Code)
	}
	rec, _ = doJSON(t, s, "DELETE", "/api/v1/admin/groups/999999", "", []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在组 delete = %d, want 404", rec.Code)
	}

	// 匿名建组：未登录先被 RequireUser 拦截 401
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/groups", validGroupBody, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("匿名建组 = %d, want 401", rec.Code)
	}
}

// policyConfigStr 把 root 编码为 config 字段的 JSON 字符串值（config 在请求体/响应体中
// 均是"JSON 编码的字符串"，见 adminPolicyDTO 注释）。
func policyConfigStr(t *testing.T, root string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"root": root})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// policyCreateBody 构造 POST /admin/policies 请求体（local 驱动，root 用调用方传入的目录）。
func policyCreateBody(t *testing.T, name, root string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"name": name, "driver": "local", "config": policyConfigStr(t, root),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAdminPoliciesListNonAdmin403(t *testing.T) {
	s, _, user := adminTestServer(t)
	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/policies", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin = %d, want 403", rec.Code)
	}
}

// TestAdminPoliciesCreateEnabledFalseRespected 锁定 CreatePolicy 的修复：POST 传
// enabled:false 时，响应与 DB 均须持久化为 false——而不是被 GORM 对 `default:true`
// 标签字段的零值回填行为静默改回 true（见 adminsvc.CreatePolicy 的 wantDisabled 逻辑）。
func TestAdminPoliciesCreateEnabledFalseRespected(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	root := t.TempDir()

	body, err := json.Marshal(map[string]any{
		"name": "disabled-from-birth", "driver": "local",
		"config": policyConfigStr(t, root), "enabled": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/policies", string(body), []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("create enabled:false = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID      uint64 `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(e.Data, &created); err != nil {
		t.Fatal(err)
	}
	if created.Enabled {
		t.Errorf("响应 enabled = true, want false")
	}

	var reload model.StoragePolicy
	if err := s.opts.DB.First(&reload, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reload.Enabled {
		t.Errorf("DB 中 enabled = true, want false（GORM default 覆盖未被纠正）")
	}
}

// TestAdminPoliciesCRUDFlow 验证创建含 config 原样 JSON 字符串、列表可见 file_count/used_bytes、
// PATCH 部分字段生效、删除成功且三条 audit 落库。
func TestAdminPoliciesCRUDFlow(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	root := t.TempDir()

	rec, e := doJSON(t, s, "POST", "/api/v1/admin/policies", policyCreateBody(t, "our-local", root), []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID           uint64 `json:"id"`
		Name         string `json:"name"`
		Driver       string `json:"driver"`
		Config       string `json:"config"`
		Enabled      bool   `json:"enabled"`
		PathTemplate string `json:"path_template"`
	}
	if err := json.Unmarshal(e.Data, &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Name != "our-local" || created.Driver != "local" || !created.Enabled {
		t.Errorf("created = %+v", created)
	}
	if created.PathTemplate != "{Y}/{m}/{d}/{uniqid}.{ext}" {
		t.Errorf("path_template 空时应用②b默认, got %q", created.PathTemplate)
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(created.Config), &cfg); err != nil {
		t.Fatalf("config 应是合法 JSON 字符串: %s: %v", created.Config, err)
	}
	if cfg["root"] != root {
		t.Errorf("config.root = %q, want %q", cfg["root"], root)
	}

	// GET 列表可见，含 file_count=0/used_bytes=0
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/policies", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(e.Data, &page); err != nil {
		t.Fatal(err)
	}
	var found map[string]any
	for _, it := range page.Items {
		if uint64(it["id"].(float64)) == created.ID {
			found = it
		}
	}
	if found == nil {
		t.Fatalf("列表中未找到新建策略: %+v", page.Items)
	}
	if found["file_count"].(float64) != 0 || found["used_bytes"].(float64) != 0 {
		t.Errorf("file_count/used_bytes = %v/%v, want 0/0", found["file_count"], found["used_bytes"])
	}

	// PATCH 改名 + cdn_domain + enabled=false
	rec, e = doJSON(t, s, "PATCH", "/api/v1/admin/policies/"+itoa(created.ID),
		`{"name":"renamed","cdn_domain":"https://cdn.example.com","enabled":false}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", rec.Code, rec.Body.String())
	}
	var patched struct {
		Name      string `json:"name"`
		CDNDomain string `json:"cdn_domain"`
		Enabled   bool   `json:"enabled"`
	}
	json.Unmarshal(e.Data, &patched)
	if patched.Name != "renamed" || patched.CDNDomain != "https://cdn.example.com" || patched.Enabled {
		t.Errorf("patched = %+v", patched)
	}

	// DELETE 成功
	rec, e = doJSON(t, s, "DELETE", "/api/v1/admin/policies/"+itoa(created.ID), "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body.String())
	}
	var deleted struct {
		ID      uint64 `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	json.Unmarshal(e.Data, &deleted)
	if deleted.ID != created.ID || !deleted.Deleted {
		t.Errorf("deleted = %+v", deleted)
	}

	// audit 三条都落库
	var creates, updates, deletes []model.AuditLog
	s.opts.DB.Where("action = ?", "policy_create").Find(&creates)
	s.opts.DB.Where("action = ?", "policy_update").Find(&updates)
	s.opts.DB.Where("action = ?", "policy_delete").Find(&deletes)
	if len(creates) != 1 || len(updates) != 1 || len(deletes) != 1 {
		t.Errorf("audit 行数: create=%d update=%d delete=%d, want 1/1/1", len(creates), len(updates), len(deletes))
	}
	if !strings.Contains(deletes[0].Detail, "renamed") {
		t.Errorf("policy_delete detail 应含策略名: %s", deletes[0].Detail)
	}
}

func TestAdminPoliciesValidationErrors(t *testing.T) {
	s, admin, _ := adminTestServer(t)

	// 空名 → 400
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/policies", policyCreateBody(t, "", t.TempDir()), []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空名 create = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if code(t, e) != "invalid_request" {
		t.Errorf("code = %s, want invalid_request", code(t, e))
	}

	// driver != local → 400
	rec, e = doJSON(t, s, "POST", "/api/v1/admin/policies",
		`{"name":"s3one","driver":"s3","config":`+quoteJSON(t, policyConfigStr(t, "x"))+`}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("driver!=local create = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if code(t, e) != "invalid_request" {
		t.Errorf("code = %s, want invalid_request", code(t, e))
	}

	// config 缺 root → 400
	rec, e = doJSON(t, s, "POST", "/api/v1/admin/policies",
		`{"name":"noroot","driver":"local","config":"{}"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("config 缺 root create = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if code(t, e) != "invalid_request" {
		t.Errorf("code = %s, want invalid_request", code(t, e))
	}

	// 不存在的策略 PATCH/DELETE → 404
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/policies/999999", `{"name":"x"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在策略 patch = %d, want 404", rec.Code)
	}
	rec, _ = doJSON(t, s, "DELETE", "/api/v1/admin/policies/999999", "", []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在策略 delete = %d, want 404", rec.Code)
	}
}

// quoteJSON 把一个字符串编码为 JSON 字符串字面量（供拼接原始 JSON body 用）。
func quoteJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestAdminPolicyDeleteInUseRejected 验证仍被 files 引用（COUNT>0）的策略不可删除，
// 走查用 seed id=1 本地策略——TestDB 播种，符合裁决 7"改用 enabled=false 下线"。
func TestAdminPolicyDeleteInUseRejected(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	if err := s.opts.DB.Create(&model.File{Hash: "inuse", StoragePolicyID: 1, Path: "p", Size: 1, RefCount: 1}).Error; err != nil {
		t.Fatal(err)
	}
	rec, e := doJSON(t, s, "DELETE", "/api/v1/admin/policies/1", "", []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("删在用策略 = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if code(t, e) != "invalid_request" {
		t.Errorf("code = %s, want invalid_request", code(t, e))
	}
}

// TestAdminPolicyTestProbeOkAndBadRootFails 验证 test 探针对真实可写目录返回 200+latency_ms，
// 对不可写 root 返回 400；两次调用均落 policy_test audit（含 ok 字段）。
func TestAdminPolicyTestProbeOkAndBadRootFails(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	root := t.TempDir()
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/policies", policyCreateBody(t, "probe", root), []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID uint64 `json:"id"`
	}
	json.Unmarshal(e.Data, &created)

	rec, e = doJSON(t, s, "POST", "/api/v1/admin/policies/"+itoa(created.ID)+"/test", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("test 探针 = %d: %s", rec.Code, rec.Body.String())
	}
	var okResp struct {
		OK        bool  `json:"ok"`
		LatencyMs int64 `json:"latency_ms"`
	}
	json.Unmarshal(e.Data, &okResp)
	if !okResp.OK || okResp.LatencyMs < 0 {
		t.Errorf("okResp = %+v", okResp)
	}

	// 把 root 改成不可写路径（一个已存在的普通文件挡住，MkdirAll 必失败）
	base := t.TempDir()
	blocker := base + "/blocker"
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	badRoot := blocker + "/sub"
	rec, _ = doJSON(t, s, "PATCH", "/api/v1/admin/policies/"+itoa(created.ID),
		`{"config":`+quoteJSON(t, policyConfigStr(t, badRoot))+`}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch bad root = %d: %s", rec.Code, rec.Body.String())
	}

	rec, e = doJSON(t, s, "POST", "/api/v1/admin/policies/"+itoa(created.ID)+"/test", "", []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("坏 root test 探针 = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if code(t, e) != "invalid_request" {
		t.Errorf("code = %s, want invalid_request", code(t, e))
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ? AND actor_id = ?", "policy_test", userIDFromSession(t, s, admin)).Find(&logs)
	if len(logs) != 2 {
		t.Fatalf("policy_test audit 行数 = %d, want 2", len(logs))
	}
	if !strings.Contains(logs[0].Detail, `"ok":true`) {
		t.Errorf("第一条应 ok:true: %s", logs[0].Detail)
	}
	if !strings.Contains(logs[1].Detail, `"ok":false`) {
		t.Errorf("第二条应 ok:false: %s", logs[1].Detail)
	}
	// 失败 audit 只多记一句 error（可读全文），不堆结构化字段
	if !strings.Contains(logs[1].Detail, `"error"`) {
		t.Errorf("失败 audit 应含 error: %s", logs[1].Detail)
	}
	if !strings.Contains(e.Message, "root 不可写") {
		t.Errorf("失败 message 应说明问题: %q", e.Message)
	}
}

// TestAdminPolicyTestNotFound404 对不存在策略调用 test 探针应 404，不落 audit。
func TestAdminPolicyTestNotFound404(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	rec, _ := doJSON(t, s, "POST", "/api/v1/admin/policies/999999/test", "", []*http.Cookie{admin})
	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在策略 test = %d, want 404", rec.Code)
	}
	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "policy_test").Find(&logs)
	if len(logs) != 0 {
		t.Errorf("不存在策略不应落 audit, got %d 条", len(logs))
	}
}

// TestAdminSettingsGateAndDefaults 非 admin 403（GET/PUT 均门禁）；admin GET 返回播种默认值。
func TestAdminSettingsGateAndDefaults(t *testing.T) {
	s, admin, user := adminTestServer(t)

	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/settings", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin GET = %d, want 403", rec.Code)
	}
	rec, _ = doJSON(t, s, "PUT", "/api/v1/admin/settings", `{"site_name":"x"}`, []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin PUT = %d, want 403", rec.Code)
	}

	rec, e := doJSON(t, s, "GET", "/api/v1/admin/settings", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		SiteName         string `json:"site_name"`
		RegistrationMode string `json:"registration_mode"`
		PlazaEnabled     bool   `json:"plaza_enabled"`
		Moderation       struct {
			Enabled   bool    `json:"enabled"`
			Provider  string  `json:"provider"`
			Endpoint  string  `json:"endpoint"`
			APIKey    string  `json:"api_key"`
			Threshold float64 `json:"threshold"`
			Action    string  `json:"action"`
		} `json:"moderation"`
	}
	if err := json.Unmarshal(e.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.SiteName != "img.li" || got.RegistrationMode != "open" {
		t.Errorf("默认值 = %+v", got)
	}
	if got.PlazaEnabled {
		t.Errorf("plaza_enabled 默认应为 false, got %v", got.PlazaEnabled)
	}
	if got.Moderation.Enabled || got.Moderation.Provider != "webhook" || got.Moderation.Threshold != 0.8 || got.Moderation.Action != "pending" {
		t.Errorf("moderation 默认值 = %+v", got.Moderation)
	}
	if got.Moderation.APIKey != "" {
		t.Errorf("api_key 默认应为空: %q", got.Moderation.APIKey)
	}
}

// TestAdminSettingsPutValidationAndAudit 覆盖 PUT 校验矩阵（未知键/非法 registration_mode/
// threshold 越界/action 非法/enabled 无 endpoint）与成功路径的 audit（detail 只含变更键名，不含值）。
func TestAdminSettingsPutValidationAndAudit(t *testing.T) {
	s, admin, _ := adminTestServer(t)

	rec, e := doJSON(t, s, "PUT", "/api/v1/admin/settings", `{"bogus_key":true}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("未知键 = %d, want 400", rec.Code)
	}
	if code(t, e) != "invalid_request" {
		t.Errorf("未知键 code = %s, want invalid_request", code(t, e))
	}

	// invite 已为合法三值之一；此处用非法 mode 覆盖 registration_mode 校验。
	rec, _ = doJSON(t, s, "PUT", "/api/v1/admin/settings", `{"registration_mode":"bogus"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 registration_mode = %d, want 400", rec.Code)
	}

	rec, _ = doJSON(t, s, "PUT", "/api/v1/admin/settings",
		`{"moderation":{"enabled":false,"provider":"webhook","endpoint":"","api_key":"","threshold":1.5,"action":"pending"}}`,
		[]*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("threshold 越界 = %d, want 400", rec.Code)
	}

	rec, _ = doJSON(t, s, "PUT", "/api/v1/admin/settings",
		`{"moderation":{"enabled":false,"provider":"webhook","endpoint":"","api_key":"","threshold":0.5,"action":"approved"}}`,
		[]*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("action 非法 = %d, want 400", rec.Code)
	}

	rec, _ = doJSON(t, s, "PUT", "/api/v1/admin/settings",
		`{"moderation":{"enabled":true,"provider":"webhook","endpoint":"","api_key":"","threshold":0.5,"action":"pending"}}`,
		[]*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("enabled 无 endpoint = %d, want 400", rec.Code)
	}

	// 上面这些校验失败请求都不含 site_name，此时 GET 应仍是播种默认值——不落库的直接验证。
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/settings", "", []*http.Cookie{admin})
	var stillDefault struct {
		SiteName string `json:"site_name"`
	}
	json.Unmarshal(e.Data, &stillDefault)
	if stillDefault.SiteName != "img.li" {
		t.Errorf("校验失败的请求不应产生任何副作用: site_name = %q", stillDefault.SiteName)
	}

	rec, e = doJSON(t, s, "PUT", "/api/v1/admin/settings",
		`{"site_name":"我的图床","registration_mode":"closed"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("成功 PUT = %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		SiteName         string `json:"site_name"`
		RegistrationMode string `json:"registration_mode"`
	}
	json.Unmarshal(e.Data, &got)
	if got.SiteName != "我的图床" || got.RegistrationMode != "closed" {
		t.Errorf("写入后 = %+v", got)
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "settings_update").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("settings_update audit 行数 = %d, want 1（校验失败的 PUT 不应落 audit）", len(logs))
	}
	if !strings.Contains(logs[0].Detail, "site_name") || !strings.Contains(logs[0].Detail, "registration_mode") {
		t.Errorf("detail 应含变更键名: %s", logs[0].Detail)
	}
	if strings.Contains(logs[0].Detail, "我的图床") || strings.Contains(logs[0].Detail, "closed") {
		t.Errorf("detail 不应含变更值: %s", logs[0].Detail)
	}
}

// TestAdminSettingsModerationAPIKeyMaskAndRetain 端到端验证 api_key 打码 + 保留语义：
// GET 只吐打码值；前端把打码值原样回传 PUT 时，库内明文不被打码串覆盖；audit 全程不泄露
// 明文或打码后的密钥片段。
func TestAdminSettingsModerationAPIKeyMaskAndRetain(t *testing.T) {
	s, admin, _ := adminTestServer(t)

	body := `{"moderation":{"enabled":true,"provider":"webhook","endpoint":"https://mod.example.com/score","api_key":"sk-supersecretkey9999","threshold":0.7,"action":"rejected"}}`
	rec, e := doJSON(t, s, "PUT", "/api/v1/admin/settings", body, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Moderation struct {
			APIKey string `json:"api_key"`
		} `json:"moderation"`
	}
	json.Unmarshal(e.Data, &resp)
	if resp.Moderation.APIKey != "****9999" {
		t.Fatalf("api_key 打码 = %q, want ****9999", resp.Moderation.APIKey)
	}

	// 用打码值原样回传（模拟前端只读到打码展示值又提交），只改 threshold。
	body2 := `{"moderation":{"enabled":true,"provider":"webhook","endpoint":"https://mod.example.com/score","api_key":"` +
		resp.Moderation.APIKey + `","threshold":0.95,"action":"rejected"}}`
	rec, e = doJSON(t, s, "PUT", "/api/v1/admin/settings", body2, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("第二次 PUT = %d: %s", rec.Code, rec.Body.String())
	}
	var resp2 struct {
		Moderation struct {
			APIKey    string  `json:"api_key"`
			Threshold float64 `json:"threshold"`
		} `json:"moderation"`
	}
	json.Unmarshal(e.Data, &resp2)
	if resp2.Moderation.APIKey != "****9999" {
		t.Errorf("保留语义后 api_key 打码仍应是 ****9999, got %q", resp2.Moderation.APIKey)
	}
	if resp2.Moderation.Threshold != 0.95 {
		t.Errorf("threshold 未生效: %v", resp2.Moderation.Threshold)
	}

	var row model.Setting
	if err := s.opts.DB.First(&row, "key = ?", model.SettingModeration).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(row.Value, "sk-supersecretkey9999") {
		t.Errorf("库中明文 api_key 应保留（未被打码串覆盖）: %s", row.Value)
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "settings_update").Find(&logs)
	if len(logs) != 2 {
		t.Fatalf("settings_update audit 行数 = %d, want 2", len(logs))
	}
	for _, l := range logs {
		if strings.Contains(l.Detail, "sk-supersecretkey9999") || strings.Contains(l.Detail, "9999") {
			t.Errorf("audit detail 泄露 api_key 片段（明文或打码值）: %s", l.Detail)
		}
		if !strings.Contains(l.Detail, "moderation") {
			t.Errorf("detail 应含变更键名 moderation: %s", l.Detail)
		}
	}
}

func TestAdminReviewNonAdmin403(t *testing.T) {
	s, _, user := adminTestServer(t)
	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/review", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin = %d, want 403", rec.Code)
	}
}

func TestAdminReviewListOnlyPending(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "rvw0000000001", pleb, "normal")
	seedAdminImage(t, s, "rvw0000000002", pleb, "pending")
	seedAdminImage(t, s, "rvw0000000003", pleb, "rejected")

	rec, e := doJSON(t, s, "GET", "/api/v1/admin/review", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
		Page  int              `json:"page"`
		Limit int              `json:"limit"`
	}
	if err := json.Unmarshal(e.Data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("total=%d items=%d, want 1/1（仅 pending）", page.Total, len(page.Items))
	}
	if page.Items[0]["key"] != "rvw0000000002" {
		t.Errorf("item key = %v, want rvw0000000002", page.Items[0]["key"])
	}
	for _, field := range []string{"key", "name", "ext", "size", "visibility", "status",
		"is_whitelisted", "nsfw_score", "username", "user_id", "created_at", "links"} {
		if _, ok := page.Items[0][field]; !ok {
			t.Errorf("item 缺字段 %s", field)
		}
	}
}

// TestAdminReviewDecideApproveUpdatesDTOAndAudit 验证 approve 流转：返回更新后的 item DTO
// （status=normal），audit 落 review_approve 且 detail 含 key/score（score 可为 null）。
func TestAdminReviewDecideApproveUpdatesDTOAndAudit(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "apv0000000001", pleb, "pending")

	rec, e := doJSON(t, s, "POST", "/api/v1/admin/review/apv0000000001", `{"action":"approve"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("decide = %d: %s", rec.Code, rec.Body.String())
	}
	var item map[string]any
	if err := json.Unmarshal(e.Data, &item); err != nil {
		t.Fatal(err)
	}
	if item["status"] != "normal" {
		t.Errorf("status = %v, want normal", item["status"])
	}
	if item["nsfw_score"] != nil {
		t.Errorf("nsfw_score = %v, want null（未打分）", item["nsfw_score"])
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "review_approve").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("audit 行数 = %d, want 1", len(logs))
	}
	if !strings.Contains(logs[0].Detail, "apv0000000001") {
		t.Errorf("audit detail 应含 key: %s", logs[0].Detail)
	}
	if !strings.Contains(logs[0].Detail, `"score":null`) {
		t.Errorf("audit detail 应含 score:null: %s", logs[0].Detail)
	}
	if logs[0].ActorType != "admin" {
		t.Errorf("ActorType = %s, want admin", logs[0].ActorType)
	}
}

// TestAdminReviewDecideRejectThen410Chain 验证 reject 后原直链 410（复用 T7 的 serve 拦截）。
func TestAdminReviewDecideRejectThen410Chain(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "rjt0000000001", pleb, "pending")

	rec, _ := doJSON(t, s, "POST", "/api/v1/admin/review/rjt0000000001", `{"action":"reject"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("decide = %d: %s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/i/rjt0000000001.png", nil))
	if rec2.Code != http.StatusGone {
		t.Errorf("直链 = %d, want 410", rec2.Code)
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "review_reject").Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("audit 行数 = %d, want 1", len(logs))
	}
}

func TestAdminReviewDecideNotPendingAndNotFound(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "npd0000000001", pleb, "normal") // 已是 normal，非 pending

	rec, _ := doJSON(t, s, "POST", "/api/v1/admin/review/npd0000000001", `{"action":"approve"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非 pending = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	rec2, _ := doJSON(t, s, "POST", "/api/v1/admin/review/nope0000001", `{"action":"approve"}`, []*http.Cookie{admin})
	if rec2.Code != http.StatusNotFound {
		t.Errorf("不存在 = %d, want 404: %s", rec2.Code, rec2.Body.String())
	}
}

func TestAdminReviewDecideInvalidAction400(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "act0000000001", pleb, "pending")

	rec, _ := doJSON(t, s, "POST", "/api/v1/admin/review/act0000000001", `{"action":"delete"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 action = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminReviewBatchPartialSuccess 验证 batch 部分成功：混合 pending/非 pending/不存在
// 三种键，成功项各落一条 audit，失败项 error 非空。
func TestAdminReviewBatchPartialSuccess(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "btc0000000001", pleb, "pending")
	seedAdminImage(t, s, "btc0000000002", pleb, "pending")
	seedAdminImage(t, s, "btc0000000003", pleb, "normal") // 非 pending，会失败

	body := `{"keys":["btc0000000001","btc0000000002","btc0000000003","nope00000009"],"action":"reject"}`
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/review/batch", body, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("batch = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			Key   string `json:"key"`
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal(e.Data, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 4 {
		t.Fatalf("results 长度 = %d, want 4", len(resp.Results))
	}
	want := map[string]bool{"btc0000000001": true, "btc0000000002": true, "btc0000000003": false, "nope00000009": false}
	for _, r := range resp.Results {
		if r.OK != want[r.Key] {
			t.Errorf("key=%s ok=%v, want %v", r.Key, r.OK, want[r.Key])
		}
		if !r.OK && r.Error == "" {
			t.Errorf("key=%s 失败但 error 为空", r.Key)
		}
	}

	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "review_reject").Find(&logs)
	if len(logs) != 2 {
		t.Fatalf("audit 行数 = %d, want 2（仅成功项各落一条）", len(logs))
	}
}

func TestAdminReviewBatchTooManyKeys400(t *testing.T) {
	s, admin, _ := adminTestServer(t)
	keys := make([]string, 101)
	for i := range keys {
		keys[i] = `"k` + itoa(uint64(i)) + `"`
	}
	body := `{"keys":[` + strings.Join(keys, ",") + `],"action":"approve"}`
	rec, _ := doJSON(t, s, "POST", "/api/v1/admin/review/batch", body, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("超限 = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminReviewDecideDegradesWhenDTORefetchFails 复现"裁决已提交+已审计，但构造
// 响应 DTO 用的 GetImageRow 补查失败"这一并发窗口（admin.go 注释所述：图在补查窗口
// 内被并发软删）。Task 3 起 images.file_id/user_id 均加了 DB 级 FK（RESTRICT），
// 不能再像先前那样直接删掉该图引用的 files 行来制造"JOIN 查零行"（会被 FK 拒绝，
// 且这也不是真实场景——真实场景是 images 行本身被并发软删，不是它引用的 file 消失）。
// 改用 GORM after-update 回调（而非 SQL 触发器，后者的 CREATE TRIGGER 语法是
// SQLite-only、在 Postgres 矩阵上会直接语法错误）：在 Decide 的 UPDATE images
// SET status=... 提交之后，确定性地把该行 deleted_at 置位，精确复现"裁决 UPDATE
// 与 handler 的 GetImageRow 补查之间，该图被另一请求并发软删"这一窗口，而不依赖
// 真并发计时；回调对 SQLite/Postgres 两种方言一视同仁，无需方言专属 DDL。
// decideRefetch 用 Unscoped 兜底取回（见 review.go 注释），所以 Decide 本身仍
// 成功；随后 GetImageRow 的默认 scope（deleted_at IS NULL）查不到该行，触发降级。
// 期望：handler 不得把这次纯粹为了拼 DTO 的读失败报成 500——裁决与审计均已生效，
// 应降级返回最小成功体 {key,status}。
func TestAdminReviewDecideDegradesWhenDTORefetchFails(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "dgd0000000001", pleb, "pending")

	// 在 Decide 的 UPDATE images 提交后，模拟并发软删该图（补查窗口内）——
	// 用 GORM 回调而非 SQLite-only 的 CREATE TRIGGER，两种方言下行为一致。
	var fired bool
	s.opts.DB.Callback().Update().After("gorm:update").Register("test_dgd1_concurrent_softdelete", func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "images" {
			return
		}
		fired = true
		// raw Exec 绕过 GORM 回调（不会递归触发本回调）；CURRENT_TIMESTAMP 两种方言均合法。
		s.opts.DB.Exec("UPDATE images SET deleted_at = CURRENT_TIMESTAMP WHERE key = ?", "dgd0000000001")
	})

	rec, e := doJSON(t, s, "POST", "/api/v1/admin/review/dgd0000000001", `{"action":"approve"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("decide = %d: %s, want 200（DTO 补查失败应降级而非 500）", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(e.Data, &body); err != nil {
		t.Fatal(err)
	}
	if body["key"] != "dgd0000000001" || body["status"] != "normal" {
		t.Errorf("降级体 = %+v, want key=dgd0000000001 status=normal", body)
	}

	// 裁决与审计均已生效——降级只改响应体形状，不代表操作本身失败。该行已被上面的
	// 触发器模拟并发软删，须 Unscoped 才能取回核对（与 decideRefetch 的取法一致）。
	var img model.Image
	if err := s.opts.DB.Unscoped().First(&img, "key = ?", "dgd0000000001").Error; err != nil {
		t.Fatal(err)
	}
	if img.Status != "normal" {
		t.Errorf("db status = %s, want normal", img.Status)
	}
	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "review_approve").Find(&logs)
	if len(logs) != 1 {
		t.Errorf("audit 行数 = %d, want 1（降级不应影响已落的审计）", len(logs))
	}
}

// TestAdminUpdateImageWhitelistDegradesWhenDTORefetchFails 同上，覆盖
// UpdateImageWhitelist 的同款降级（SetWhitelist 成功+已审计后，GetImageRow 因该图
// 被并发软删而查不到，应降级返回最小成功体而非 500）。同上不删 files 行制造孤儿
// ——images.file_id 现有 DB 级 FK（RESTRICT），且真实场景本就是并发软删 images
// 行，不是它引用的 file 消失；用 GORM after-update 回调（而非 SQLite-only 的
// CREATE TRIGGER）模拟该并发软删，两种方言下行为一致。
func TestAdminUpdateImageWhitelistDegradesWhenDTORefetchFails(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)
	seedAdminImage(t, s, "dgd0000000002", pleb, "normal")

	// 在 SetWhitelist 的 UPDATE images 提交后，模拟并发软删该图（补查窗口内）。
	var fired bool
	s.opts.DB.Callback().Update().After("gorm:update").Register("test_dgd2_concurrent_softdelete", func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "images" {
			return
		}
		fired = true
		// raw Exec 绕过 GORM 回调（不会递归触发本回调）；CURRENT_TIMESTAMP 两种方言均合法。
		s.opts.DB.Exec("UPDATE images SET deleted_at = CURRENT_TIMESTAMP WHERE key = ?", "dgd0000000002")
	})

	rec, e := doJSON(t, s, "PATCH", "/api/v1/admin/images/dgd0000000002", `{"is_whitelisted":true}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("whitelist = %d: %s, want 200（DTO 补查失败应降级而非 500）", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(e.Data, &body); err != nil {
		t.Fatal(err)
	}
	if body["key"] != "dgd0000000002" || body["is_whitelisted"] != true {
		t.Errorf("降级体 = %+v, want key=dgd0000000002 is_whitelisted=true", body)
	}

	// 该行已被上面的触发器模拟并发软删，须 Unscoped 才能取回核对。
	var img model.Image
	if err := s.opts.DB.Unscoped().First(&img, "key = ?", "dgd0000000002").Error; err != nil {
		t.Fatal(err)
	}
	if !img.IsWhitelisted {
		t.Errorf("db is_whitelisted = %v, want true", img.IsWhitelisted)
	}
	var logs []model.AuditLog
	s.opts.DB.Where("action = ?", "image_whitelist").Find(&logs)
	if len(logs) != 1 {
		t.Errorf("audit 行数 = %d, want 1（降级不应影响已落的审计）", len(logs))
	}
}

func TestAdminLogsNonAdmin403(t *testing.T) {
	s, _, user := adminTestServer(t)

	// 普通用户访问 /admin/logs 应 403
	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/logs", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin 访问 /logs = %d, want 403", rec.Code)
	}
}

func TestAdminLogsAfterRealWrites(t *testing.T) {
	s, admin, user := adminTestServer(t)
	pleb := userIDFromSession(t, s, user)

	// 触发 user_update：改用户 status
	banned := "banned"
	rec, e := doJSON(t, s, "PATCH", "/api/v1/admin/users/"+itoa(pleb),
		`{"status":"`+banned+`"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("user_update = %d: %s", rec.Code, rec.Body.String())
	}

	// 触发 group_create：建新组
	rec, e = doJSON(t, s, "POST", "/api/v1/admin/groups",
		`{"name":"test_group","storage_quota":1073741824,"max_file_size":10485760,`+
			`"rate_per_minute":60,"rate_per_hour":1000,"rate_per_day":10000,"allowed_exts":["jpg","png"],`+
			`"allowed_policy_ids":[1]}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("group_create = %d: %s", rec.Code, rec.Body.String())
	}

	// 查询全量日志（应包含上述两条）
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/logs", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /logs = %d: %s", rec.Code, rec.Body.String())
	}

	var page struct {
		Items []struct {
			ID        uint64 `json:"id"`
			ActorID   any    `json:"actor_id"`
			ActorType string `json:"actor_type"`
			Action    string `json:"action"`
			Detail    string `json:"detail"`
			IP        string `json:"ip"`
			CreatedAt string `json:"created_at"`
		} `json:"items"`
		Total int64 `json:"total"`
		Page  int   `json:"page"`
		Limit int   `json:"limit"`
	}
	if err := json.Unmarshal(e.Data, &page); err != nil {
		t.Fatalf("unmarshal /logs = %v", err)
	}

	// ① 验证 total 和 items 计数
	if page.Total < 2 {
		t.Errorf("total = %d, want >=2 (user_update + group_create)", page.Total)
	}
	if len(page.Items) < 2 {
		t.Errorf("items 数 = %d, want >=2", len(page.Items))
	}

	// ② 验证倒序（后触发的在前）
	// 最后一个是 group_create，倒数第二个是 user_update
	found := false
	for i, item := range page.Items {
		if item.Action == "group_create" && i == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("group_create 不在首位（倒序失败）")
	}

	// ③ 首条 item 字段形状齐全 & detail 是合法 JSON
	if len(page.Items) > 0 {
		first := page.Items[0]
		if first.ID == 0 {
			t.Errorf("首条 item id = 0")
		}
		if first.ActorType != "admin" {
			t.Errorf("首条 item actor_type = %s, want 'admin'", first.ActorType)
		}
		if first.Action == "" {
			t.Errorf("首条 item action 为空")
		}
		if first.Detail == "" {
			t.Errorf("首条 item detail 为空")
		}
		// detail 应是合法 JSON 字符串
		var detail map[string]any
		if err := json.Unmarshal([]byte(first.Detail), &detail); err != nil {
			t.Errorf("首条 item detail 不是合法 JSON: %v", err)
		}
		if first.CreatedAt == "" {
			t.Errorf("首条 item created_at 为空")
		}
	}

	// ④ 按 action=group_create 筛选，应只剩对应行
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/logs?action=group_create", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /logs?action=group_create = %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(e.Data, &page)
	if page.Total != 1 {
		t.Errorf("?action=group_create total = %d, want 1", page.Total)
	}
	if len(page.Items) != 1 || page.Items[0].Action != "group_create" {
		t.Errorf("?action=group_create items 不匹配: %+v", page.Items)
	}
}

// doJSONBearer 同 doJSON，但用 Authorization: Bearer 而非 cookie 认证（供 API Token 测试）。
func doJSONBearer(t *testing.T, s *Server, method, path, body, token string) (*httptest.ResponseRecorder, env) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var e env
	json.Unmarshal(rec.Body.Bytes(), &e)
	return rec, e
}

// TestAdminGateMatrix 枚举 /api/v1/admin 路由（逐条对照 server.go mountAPI 的
// /admin 路由注册，缺一不可），断言三重身份门禁：
//
//	a) 普通用户 session（非 admin）→ 全部 403，body 含 "forbidden"（RequireAdmin 拦）；
//	b) upload-scope Bearer token（既有 token API 为普通用户所建）→ 全部 403，body 含
//	   "forbidden"（RequireFullScope 先于 RequireAdmin 拦截）；
//	c) 无凭证 → 全部 401 unauthorized（RequireUser 拦）。
//
// 三种身份的请求均在到达 handler 前被中间件拦下，故 id/key 占位符（1、xxxxxxxxxxxx）
// 与请求体的具体取值不影响断言结果；写请求仍给出合法最小 JSON body，避免可能的 400
// 抢在门禁之前触发（虽然中间件在 handler 之前执行，理论上无碍，但更贴近真实调用）。
func TestAdminGateMatrix(t *testing.T) {
	routes := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/admin/stats", ""},
		{"GET", "/api/v1/admin/referers/images", ""},
		{"GET", "/api/v1/admin/users", ""},
		{"PATCH", "/api/v1/admin/users/1", `{"status":"active"}`},
		{"POST", "/api/v1/admin/users/1/reset-password", ""},
		{"GET", "/api/v1/admin/images", ""},
		{"DELETE", "/api/v1/admin/images/xxxxxxxxxxxx", ""},
		{"PATCH", "/api/v1/admin/images/xxxxxxxxxxxx", `{"is_whitelisted":true}`},
		{"GET", "/api/v1/admin/review", ""},
		{"POST", "/api/v1/admin/review/batch", `{"keys":["xxxxxxxxxxxx"],"action":"approve"}`},
		{"POST", "/api/v1/admin/review/xxxxxxxxxxxx", `{"action":"approve"}`},
		{"GET", "/api/v1/admin/groups", ""},
		{"POST", "/api/v1/admin/groups", validGroupBody},
		{"PATCH", "/api/v1/admin/groups/1", `{"storage_quota":1073741824}`},
		{"DELETE", "/api/v1/admin/groups/1", ""},
		{"GET", "/api/v1/admin/policies", ""},
		{"POST", "/api/v1/admin/policies", `{"name":"p","driver":"local","config":"{\"root\":\"/tmp/x\"}"}`},
		{"PATCH", "/api/v1/admin/policies/1", `{"name":"renamed"}`},
		{"DELETE", "/api/v1/admin/policies/1", ""},
		{"POST", "/api/v1/admin/policies/1/test", ""},
		{"GET", "/api/v1/admin/settings", ""},
		{"PUT", "/api/v1/admin/settings", `{"site_name":"x"}`},
		{"POST", "/api/v1/admin/settings/smtp/test", `{"to":"a@b.c"}`},
		{"POST", "/api/v1/admin/settings/mail/preview", `{"kind":"welcome","lang":"zh"}`},
		{"POST", "/api/v1/admin/settings/mail/test", `{"to":"a@b.c","kind":"welcome","lang":"zh"}`},
		{"GET", "/api/v1/admin/logs", ""},
	}
	if len(routes) != 26 {
		t.Fatalf("路由清单应为 26 条（对照 server.go mountAPI 的 /admin Route 注册）, got %d", len(routes))
	}

	s, _, user := adminTestServer(t)

	// 用既有 token API 为普通用户建一个 upload-scope token。
	rec, e := doJSON(t, s, "POST", "/api/v1/user/tokens", `{"name":"m","scope":"upload"}`, []*http.Cookie{user})
	if rec.Code != http.StatusOK {
		t.Fatalf("创建 upload token = %d: %s", rec.Code, rec.Body.String())
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(e.Data, &tok); err != nil || tok.Token == "" {
		t.Fatalf("token 明文缺失: %v, data=%s", err, e.Data)
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			// a) 普通用户 session → 403 forbidden
			rec, e := doJSON(t, s, rt.method, rt.path, rt.body, []*http.Cookie{user})
			if rec.Code != http.StatusForbidden {
				t.Errorf("非 admin session = %d, want 403: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "forbidden") {
				t.Errorf("非 admin session body 应含 forbidden: %s", rec.Body.String())
			}
			if code(t, e) != "forbidden" {
				t.Errorf("非 admin session code = %s, want forbidden", code(t, e))
			}

			// b) upload-scope Bearer token → 403 forbidden（RequireFullScope 拦）
			rec, e = doJSONBearer(t, s, rt.method, rt.path, rt.body, tok.Token)
			if rec.Code != http.StatusForbidden {
				t.Errorf("upload-scope token = %d, want 403: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "forbidden") {
				t.Errorf("upload-scope token body 应含 forbidden: %s", rec.Body.String())
			}
			if code(t, e) != "forbidden" {
				t.Errorf("upload-scope token code = %s, want forbidden", code(t, e))
			}

			// c) 无凭证 → 401 unauthorized
			rec, e = doJSON(t, s, rt.method, rt.path, rt.body, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("匿名 = %d, want 401: %s", rec.Code, rec.Body.String())
			}
			if code(t, e) != "unauthorized" {
				t.Errorf("匿名 code = %s, want unauthorized", code(t, e))
			}
		})
	}
}

// TestAdminInvitesAPI 端到端：批量生成 → 列表 → 撤销未用码；非 admin 三端点 403。
func TestAdminInvitesAPI(t *testing.T) {
	s, admin, user := adminTestServer(t)
	codeFmt := regexp.MustCompile(`^IL-[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)

	// 非 admin 访问三端点均 403
	for _, rt := range []struct {
		method, path, body string
	}{
		{"GET", "/api/v1/admin/invites", ""},
		{"POST", "/api/v1/admin/invites", `{"count":1}`},
		{"DELETE", "/api/v1/admin/invites/1", ""},
	} {
		rec, _ := doJSON(t, s, rt.method, rt.path, rt.body, []*http.Cookie{user})
		if rec.Code != http.StatusForbidden {
			t.Errorf("非 admin %s %s = %d, want 403", rt.method, rt.path, rec.Code)
		}
	}

	// admin POST 生成 2 张
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/invites", `{"count":2}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST invites = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Codes []string `json:"codes"`
	}
	if err := json.Unmarshal(e.Data, &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Codes) != 2 {
		t.Fatalf("codes len = %d, want 2", len(created.Codes))
	}
	for _, c := range created.Codes {
		if !codeFmt.MatchString(c) {
			t.Errorf("码格式不符: %q", c)
		}
	}

	// GET 列表 total>=2
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/invites", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET invites = %d: %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []struct {
			ID     uint64 `json:"id"`
			Code   string `json:"code"`
			Status string `json:"status"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(e.Data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total < 2 || len(page.Items) < 2 {
		t.Fatalf("total=%d items=%d, want >=2", page.Total, len(page.Items))
	}

	// limit 超上限应 clamp 到 200 并在响应中回显生效值
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/invites?limit=500", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET limit=500 = %d: %s", rec.Code, rec.Body.String())
	}
	var clamped struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(e.Data, &clamped); err != nil {
		t.Fatal(err)
	}
	if clamped.Limit != 200 {
		t.Errorf("limit=500 应截至 200, got %d", clamped.Limit)
	}

	// 撤销其中一张
	revokeID := page.Items[0].ID
	revokeCode := page.Items[0].Code
	rec, _ = doJSON(t, s, "DELETE", "/api/v1/admin/invites/"+strconv.FormatUint(revokeID, 10), "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE invite = %d: %s", rec.Code, rec.Body.String())
	}

	// GET ?status=unused 数量减一
	rec, e = doJSON(t, s, "GET", "/api/v1/admin/invites?status=unused", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET unused = %d: %s", rec.Code, rec.Body.String())
	}
	var unused struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(e.Data, &unused); err != nil {
		t.Fatal(err)
	}
	// 生成 2 张均为 unused；撤销 1 张后应剩 1
	if unused.Total != 1 {
		t.Errorf("unused total = %d, want 1（撤销后减一）", unused.Total)
	}

	// 审计：invite_create / invite_revoke（revoke detail 含被撤销码明文）
	var logs []model.AuditLog
	s.opts.DB.Where("action IN ?", []string{"invite_create", "invite_revoke"}).Find(&logs)
	var hasCreate, hasRevoke bool
	for _, l := range logs {
		if l.Action == "invite_create" && strings.Contains(l.Detail, `"count":2`) {
			hasCreate = true
		}
		if l.Action == "invite_revoke" && strings.Contains(l.Detail, `"id":`) && strings.Contains(l.Detail, revokeCode) {
			hasRevoke = true
		}
	}
	if !hasCreate {
		t.Error("缺 invite_create 审计")
	}
	if !hasRevoke {
		t.Error("缺 invite_revoke 审计（应含被撤销码明文）")
	}
}

// TestAdminSMTPTestEndpoint 端点级：未配置→400「SMTP 未配置」；非 admin→403；to 缺失/非邮箱→400。
// 真发送路径不在 API 测试覆盖（sender 桩在服务级）。
func TestAdminSMTPTestEndpoint(t *testing.T) {
	s, admin, user := adminTestServer(t)

	// 未配置时 admin 发测试 → 400，message 含「SMTP 未配置」
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/settings/smtp/test", `{"to":"a@b.c"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("未配置 = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SMTP 未配置") {
		t.Errorf("message 应含「SMTP 未配置」: %s", rec.Body.String())
	}
	if code(t, e) != "invalid_request" {
		t.Errorf("code = %s, want invalid_request", code(t, e))
	}

	// 非 admin → 403
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/settings/smtp/test", `{"to":"a@b.c"}`, []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin = %d, want 403", rec.Code)
	}

	// to 缺失
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/settings/smtp/test", `{}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("to 缺失 = %d, want 400", rec.Code)
	}

	// to 非邮箱
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/settings/smtp/test", `{"to":"not-email"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("to 非邮箱 = %d, want 400", rec.Code)
	}

	// 覆盖配置带掩码密码但库中身份不同 → 要求重输，不能拿旧密码打到新用户名
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/settings/smtp/test",
		`{"to":"a@b.c","smtp":{"host":"smtp.larksuite.com","port":465,"username":"noreply@qqqu.de","password":"****pwxx","from":"noreply@qqqu.de","encryption":"ssl"}}`,
		[]*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("掩码+新身份 test = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "请重新输入密码") {
		t.Errorf("应提示重输密码: %s", rec.Body.String())
	}

	// 覆盖配置但 host 空：测发信用表单值，应提示填写服务器，而不是「请先保存」
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/settings/smtp/test",
		`{"to":"a@b.c","smtp":{"host":"","port":587,"username":"","password":"","from":"","encryption":"starttls"}}`,
		[]*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 host 覆盖 = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "请填写 SMTP 服务器") {
		t.Errorf("空 host 应提示填写服务器: %s", rec.Body.String())
	}
}

func TestAdminMailPreviewEndpoint(t *testing.T) {
	s, admin, user := adminTestServer(t)
	rec, _ := doJSON(t, s, "POST", "/api/v1/admin/settings/mail/preview",
		`{"kind":"welcome","lang":"zh"}`, []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Errorf("非 admin preview = %d, want 403", rec.Code)
	}
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/settings/mail/preview",
		`{"kind":"welcome","lang":"zh"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body.String())
	}
	var data struct {
		Subject string `json:"subject"`
		HTML    string `json:"html"`
	}
	if err := json.Unmarshal(e.Data, &data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data.Subject, "欢迎使用") || !strings.Contains(data.HTML, "打开设置") {
		t.Errorf("内置欢迎信: %+v", data)
	}
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/settings/mail/preview",
		`{"kind":"nope","lang":"zh"}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("坏 kind = %d", rec.Code)
	}
}

func TestAdminSystemVersion(t *testing.T) {
	s, admin, user := adminTestServer(t)
	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/system/version", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user version = %d", rec.Code)
	}
	rec, e := doJSON(t, s, "GET", "/api/v1/admin/system/version", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("admin version = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Current string `json:"current"`
		Repo    string `json:"repo"`
	}
	if err := json.Unmarshal(e.Data, &body); err != nil {
		t.Fatal(err)
	}
	if body.Current == "" || body.Repo == "" {
		t.Fatalf("empty version payload: %+v", body)
	}
}

func TestAdminStorageMigrateJob(t *testing.T) {
	s, admin, user := adminTestServer(t)
	// non-admin forbidden
	rec, _ := doJSON(t, s, "POST", "/api/v1/admin/storage/migrate",
		`{"from_policy_id":1,"to_policy_id":2,"dry_run":true}`, []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user migrate = %d body=%s", rec.Code, rec.Body.String())
	}
	// create second policy (use TempDir root for isolation)
	root := t.TempDir()
	rec, e := doJSON(t, s, "POST", "/api/v1/admin/policies", policyCreateBody(t, "mig-to", root), []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("create policy = %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID uint64 `json:"id"`
	}
	if err := json.Unmarshal(e.Data, &created); err != nil || created.ID == 0 {
		t.Fatalf("parse create: %v data=%s", err, string(e.Data))
	}
	// same from/to should 400
	rec, _ = doJSON(t, s, "POST", "/api/v1/admin/storage/migrate",
		`{"from_policy_id":1,"to_policy_id":1,"dry_run":true}`, []*http.Cookie{admin})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("same policy = %d %s", rec.Code, rec.Body.String())
	}
	body := `{"from_policy_id":1,"to_policy_id":` + itoa(created.ID) + `,"dry_run":true,"limit":10}`
	rec, e = doJSON(t, s, "POST", "/api/v1/admin/storage/migrate", body, []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("start migrate = %d %s", rec.Code, rec.Body.String())
	}
	var job struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(e.Data, &job); err != nil || job.ID == "" {
		t.Fatalf("parse job: %v data=%s", err, string(e.Data))
	}
	// poll until terminal
	for i := 0; i < 50; i++ {
		rec, e = doJSON(t, s, "GET", "/api/v1/admin/storage/migrate/"+job.ID, "", []*http.Cookie{admin})
		if rec.Code != http.StatusOK {
			t.Fatalf("get job = %d %s", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(e.Data, &job); err != nil {
			t.Fatal(err)
		}
		if job.Status == "done" || job.Status == "failed" {
			if job.Status != "done" {
				t.Fatalf("job failed: %s", string(e.Data))
			}
			rec, e = doJSON(t, s, "GET", "/api/v1/admin/storage/migrate", "", []*http.Cookie{admin})
			if rec.Code != http.StatusOK {
				t.Fatalf("list migrate = %d %s", rec.Code, rec.Body.String())
			}
			var listed struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			if err := json.Unmarshal(e.Data, &listed); err != nil {
				t.Fatal(err)
			}
			if len(listed.Items) == 0 || listed.Items[0].ID != job.ID {
				t.Fatalf("list items=%+v want %s", listed.Items, job.ID)
			}
			return
		}
	}
	t.Fatal("job did not finish")
}
