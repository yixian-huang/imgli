package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/settings"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/service/upload"
	"github.com/yixian-huang/imgli/internal/task"
)

// TestFailUploadMapping 校验 failUpload 对各上传错误哨兵的状态码/错误码映射，
// 确保每一对都落在项目约定的绑定表内（见 respond.go 与 upload.go 顶部注释）。
func TestFailUploadMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"file too large", upload.ErrFileTooLarge, 413, CodeFileTooLarge},
		{"quota exceeded", upload.ErrQuotaExceeded, 413, CodeQuotaExceeded},
		{"bandwidth exceeded", upload.ErrBandwidthExceeded, 429, CodeBandwidthExceeded},
		{"ext not allowed", upload.ErrExtNotAllowed, 415, CodeExtNotAllowed},
		{"heic unavailable", upload.ErrHeicUnavailable, 415, CodeHeicUnsupported},
		{"dimension over", upload.ErrDimensionOver, 400, CodeInvalidRequest},
		{"invalid image", upload.ErrInvalidImage, 400, CodeInvalidRequest},
		{"guest not supported", upload.ErrGuestNotSupported, 403, CodeForbidden},
		{"generic error", errors.New("boom"), 500, CodeInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			failUpload(rec, tc.err)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			var envelope struct {
				Status bool `json:"status"`
				Data   struct {
					Code string `json:"code"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if envelope.Status {
				t.Fatalf("status field = true, want false for error response")
			}
			if envelope.Data.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", envelope.Data.Code, tc.wantCode)
			}
		})
	}
}

// TestFailUploadGuestNotSupportedMessage 游客上传关闭时应带哨兵原文案（"游客上传暂未开放"），
// 而非泛化的服务器内部错误文案，前端好直接展示。
func TestFailUploadGuestNotSupportedMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	failUpload(rec, upload.ErrGuestNotSupported)
	var envelope struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if envelope.Message != "游客上传暂未开放" {
		t.Errorf("message = %q, want 游客上传暂未开放", envelope.Message)
	}
}

// TestFailUploadHeicUnavailableMessage 纯 Go / 无 libheif 时走 415 heic_unsupported，
// 文案须与 spec 完全一致（前端 en 靠 i18n，zh 也走同一 code）。
func TestFailUploadHeicUnavailableMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	failUpload(rec, upload.ErrHeicUnavailable)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
	var envelope struct {
		Message string `json:"message"`
		Data    struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	want := "当前构建无法解码 HEIC，请使用官方 Docker 镜像或 make build-vips（需 libheif）"
	if envelope.Message != want {
		t.Errorf("message = %q, want %q", envelope.Message, want)
	}
	if envelope.Data.Code != CodeHeicUnsupported {
		t.Errorf("code = %q, want %q", envelope.Data.Code, CodeHeicUnsupported)
	}
}

func setupUploadHandler(t *testing.T) (*gorm.DB, *UploadHandlers, *model.User, *chi.Mux) {
	t.Helper()
	db := model.TestDB(t)
	cfg := &config.Config{DataDir: t.TempDir(), BaseURL: "https://img.li"}
	res := storagesvc.New(cfg, db)
	runner := task.New(db, 1)
	svc := upload.New(db, res, imaging.NewGo(), runner)
	runner.Register("delete_file", svc.DeleteFileTask)
	u := &model.User{Username: "up", Email: "up@img.li", GroupID: 1, Status: "active"}
	if err := db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	h := &UploadHandlers{D: UploadDeps{
		Svc: svc, Res: res, MaxBytes: 10 << 20, FetchClient: NewFetchClient(nil),
	}}
	mux := chi.NewRouter()
	mux.With(withPrincipal(u)).Post("/api/v1/upload", h.Upload)
	mux.Post("/api/v1/upload/anon", h.Upload)
	mux.With(withPrincipal(u)).Post("/api/v1/upload/url", h.UploadURL)
	return db, h, u, mux
}

func multipartUploadBody(t *testing.T, filename string, data []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, mw.FormDataContentType()
}

func uploadKeyFromRec(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Key == "" {
		t.Fatalf("empty key body=%s", rec.Body.String())
	}
	return env.Data.Key
}

// TestUploadExpiresIn86400 expires_in=86400 → Save 后 image.ExpiresAt≈now+1d。
func TestUploadExpiresIn86400(t *testing.T) {
	db, _, _, mux := setupUploadHandler(t)
	before := time.Now()
	body, ctype := multipartUploadBody(t, "day.png", encodeTestPNG(t, 32, 20), map[string]string{"expires_in": "86400"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	after := time.Now()
	var img model.Image
	if err := db.First(&img, "key = ?", uploadKeyFromRec(t, rec)).Error; err != nil {
		t.Fatal(err)
	}
	if img.ExpiresAt == nil {
		t.Fatal("ExpiresAt 应非 nil")
	}
	lo := before.Add(24 * time.Hour).Add(-3 * time.Second)
	hi := after.Add(24 * time.Hour).Add(3 * time.Second)
	if img.ExpiresAt.Before(lo) || img.ExpiresAt.After(hi) {
		t.Errorf("ExpiresAt=%v want in [%v,%v]", img.ExpiresAt, lo, hi)
	}
}

// TestUploadExpiresInZeroAndMissingNil expires_in=0 或缺省 → nil。
func TestUploadExpiresInZeroAndMissingNil(t *testing.T) {
	db, _, _, mux := setupUploadHandler(t)
	for name, fields := range map[string]map[string]string{
		"zero":    {"expires_in": "0"},
		"missing": {},
	} {
		t.Run(name, func(t *testing.T) {
			// 不同尺寸避免秒传共享同一 image 断言混乱
			w := 30
			if name == "missing" {
				w = 31
			}
			body, ctype := multipartUploadBody(t, name+".png", encodeTestPNG(t, w, 20), fields)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", body)
			req.Header.Set("Content-Type", ctype)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var img model.Image
			if err := db.First(&img, "key = ?", uploadKeyFromRec(t, rec)).Error; err != nil {
				t.Fatal(err)
			}
			if img.ExpiresAt != nil {
				t.Errorf("ExpiresAt 应 nil, got %v", img.ExpiresAt)
			}
		})
	}
}

// TestUploadExpiresInNegative400 expires_in=-1 → 400。
func TestUploadExpiresInNegative400(t *testing.T) {
	_, _, _, mux := setupUploadHandler(t)
	body, ctype := multipartUploadBody(t, "bad.png", encodeTestPNG(t, 10, 10), map[string]string{"expires_in": "-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.Code != CodeInvalidRequest {
		t.Errorf("code=%q want %q", env.Data.Code, CodeInvalidRequest)
	}
}

// TestUploadGuestExpiresIn 游客上传设过期生效。
func TestUploadGuestExpiresIn(t *testing.T) {
	db, _, _, mux := setupUploadHandler(t)
	if err := settings.New(db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	body, ctype := multipartUploadBody(t, "g.png", encodeTestPNG(t, 33, 20), map[string]string{"expires_in": "3600"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/anon", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var img model.Image
	if err := db.First(&img, "key = ?", uploadKeyFromRec(t, rec)).Error; err != nil {
		t.Fatal(err)
	}
	if img.UserID != nil {
		t.Error("游客 UserID 应 nil")
	}
	if img.ExpiresAt == nil {
		t.Fatal("游客 ExpiresAt 应写入")
	}
}

// TestUploadURLExpiresInNegative400 /upload/url JSON expires_in<0 → 400（在抓取前）。
func TestUploadURLExpiresInNegative400(t *testing.T) {
	_, _, _, mux := setupUploadHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload/url",
		bytes.NewBufferString(`{"url":"https://example.com/a.png","expires_in":-1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
		Message string `json:"message"`
	}
	json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.Code != CodeInvalidRequest {
		t.Errorf("code=%q want %q", env.Data.Code, CodeInvalidRequest)
	}
	if env.Message != "expires_in 不合法" {
		t.Errorf("message=%q", env.Message)
	}
}
