package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/settings"
)

// configBody 反映 GET /config 的响应形状（子集，够断言用）。
type configBody struct {
	SiteName           string `json:"site_name"`
	RegistrationMode   string `json:"registration_mode"`
	GuestUploadEnabled bool   `json:"guest_upload_enabled"`
	PlazaEnabled       bool   `json:"plaza_enabled"`
	BaseURL            string `json:"base_url"`
	Guest              *struct {
		MaxFileSize int64    `json:"max_file_size"`
		AllowedExts []string `json:"allowed_exts"`
		PerDay      int      `json:"per_day"`
	} `json:"guest"`
	PublicStats *struct {
		Enabled        bool   `json:"enabled"`
		LiveImageCount *int64 `json:"live_image_count"`
	} `json:"public_stats"`
}

// TestConfigDefaults 播种库默认值下：site_name="img.li"、registration_mode="open"、
// guest_upload_enabled=false，guest 限额取自播种的游客组（5MB/3/常见后缀）。
func TestConfigDefaults(t *testing.T) {
	db := model.TestDB(t)
	h := &ConfigHandler{DB: db, BaseURL: "https://img.li/"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	h.Config(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var e struct {
		Status bool            `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode envelope: %v, body=%s", err, rec.Body.String())
	}
	if !e.Status {
		t.Fatalf("status = false, body=%s", rec.Body.String())
	}

	var body configBody
	if err := json.Unmarshal(e.Data, &body); err != nil {
		t.Fatalf("decode data: %v, data=%s", err, e.Data)
	}

	if body.SiteName != "img.li" {
		t.Errorf("site_name = %q, want img.li", body.SiteName)
	}
	if body.BaseURL != "https://img.li" {
		t.Errorf("base_url = %q, want https://img.li (trim slash)", body.BaseURL)
	}
	if body.RegistrationMode != "open" {
		t.Errorf("registration_mode = %q, want open", body.RegistrationMode)
	}
	if body.GuestUploadEnabled != false {
		t.Errorf("guest_upload_enabled = %v, want false", body.GuestUploadEnabled)
	}
	if body.PlazaEnabled != false {
		t.Errorf("plaza_enabled = %v, want false", body.PlazaEnabled)
	}
	if body.Guest == nil {
		t.Fatalf("guest 为 nil，播种库应有游客组")
	}
	if body.Guest.MaxFileSize != 5<<20 {
		t.Errorf("guest.max_file_size = %d, want %d", body.Guest.MaxFileSize, 5<<20)
	}
	if body.Guest.PerDay != 3 {
		t.Errorf("guest.per_day = %d, want 3", body.Guest.PerDay)
	}
	if len(body.Guest.AllowedExts) == 0 {
		t.Errorf("guest.allowed_exts 为空")
	}
	hasHeic, hasHeif := false, false
	for _, e := range body.Guest.AllowedExts {
		if e == "heic" {
			hasHeic = true
		}
		if e == "heif" {
			hasHeif = true
		}
	}
	if !hasHeic || !hasHeif {
		t.Errorf("guest.allowed_exts 缺 heic/heif: %v", body.Guest.AllowedExts)
	}

	// 公开统计默认关闭（自托管零配置不展示数字）
	if body.PublicStats == nil {
		t.Fatal("public_stats missing")
	}
	if body.PublicStats.Enabled {
		t.Error("public_stats.enabled default want false")
	}
	if body.PublicStats.LiveImageCount != nil {
		t.Error("disabled public_stats must not include counts")
	}

	// NO 密钥/私密：响应体不得出现 api_key 等敏感字段。
	raw := rec.Body.String()
	if strings.Contains(raw, "api_key") {
		t.Errorf("响应体泄露 api_key: %s", raw)
	}
}

// TestConfigReflectsSettings PUT 改过的 settings（经 settings.Service 直接写库模拟）应体现在
// GET /config：guest_upload_enabled=true、site_name/registration_mode 更新后的值。
func TestConfigReflectsSettings(t *testing.T) {
	db := model.TestDB(t)
	st := settings.New(db)
	if err := st.Set(model.SettingSiteName, "我的图床"); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(model.SettingRegistrationMode, "closed"); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}

	h := &ConfigHandler{DB: db}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	h.Config(rec, req)

	var e struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &e)
	var body configBody
	if err := json.Unmarshal(e.Data, &body); err != nil {
		t.Fatalf("decode data: %v, data=%s", err, e.Data)
	}

	if body.SiteName != "我的图床" {
		t.Errorf("site_name = %q, want 我的图床", body.SiteName)
	}
	if body.RegistrationMode != "closed" {
		t.Errorf("registration_mode = %q, want closed", body.RegistrationMode)
	}
	if !body.GuestUploadEnabled {
		t.Errorf("guest_upload_enabled = false, want true")
	}
}

// TestConfigNoGuestGroup 没有游客组时不应报错，guest 应降级为 null/空对象（不是 500）。
func TestConfigNoGuestGroup(t *testing.T) {
	db := model.TestDB(t)
	if err := db.Model(&model.UserGroup{}).Where("is_guest = ?", true).Update("is_guest", false).Error; err != nil {
		t.Fatal(err)
	}

	h := &ConfigHandler{DB: db}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	h.Config(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var e struct {
		Status bool            `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode envelope: %v, body=%s", err, rec.Body.String())
	}
	if !e.Status {
		t.Fatalf("status = false when guest group missing, body=%s", rec.Body.String())
	}
}
