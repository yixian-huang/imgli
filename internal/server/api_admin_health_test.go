package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/imaging"
)

func TestAdminSystemHealth(t *testing.T) {
	s, admin, user := adminTestServer(t)

	// non-admin 403
	rec, _ := doJSON(t, s, "GET", "/api/v1/admin/system/health", "", []*http.Cookie{user})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin health = %d, want 403", rec.Code)
	}

	rec, e := doJSON(t, s, "GET", "/api/v1/admin/system/health", "", []*http.Cookie{admin})
	if rec.Code != http.StatusOK {
		t.Fatalf("admin health = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Doctor struct {
			HardFail bool `json:"hard_fail"`
			Checks   []struct {
				Name    string `json:"name"`
				Level   string `json:"level"`
				Message string `json:"message"`
			} `json:"checks"`
		} `json:"doctor"`
		Runtime struct {
			Version        string `json:"version"`
			BaseURL        string `json:"base_url"`
			TrustProxy     bool   `json:"trust_proxy"`
			Listen         string `json:"listen"`
			DataDir        string `json:"data_dir"`
			Install        string `json:"install"`
			RequestHost    string `json:"request_host"`
			ImagingBackend string `json:"imaging_backend"`
			WebPEncode     bool   `json:"webp_encode"`
			HeicDecode     bool   `json:"heic_decode"`
			ThumbExt       string `json:"thumb_ext"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal(e.Data, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Doctor.Checks) == 0 {
		t.Error("expected doctor checks")
	}
	if body.Runtime.BaseURL == "" {
		t.Error("runtime.base_url empty")
	}
	if body.Runtime.Listen == "" {
		t.Error("runtime.listen empty")
	}
	if body.Runtime.Install != "binary" && body.Runtime.Install != "docker" {
		t.Errorf("install = %q", body.Runtime.Install)
	}
	// CI/单测默认纯 Go；Docker 发行为 vips
	if body.Runtime.ImagingBackend != "pure-go" && body.Runtime.ImagingBackend != "vips" {
		t.Errorf("imaging_backend = %q", body.Runtime.ImagingBackend)
	}
	if body.Runtime.ThumbExt != "jpg" && body.Runtime.ThumbExt != "webp" {
		t.Errorf("thumb_ext = %q", body.Runtime.ThumbExt)
	}
	if body.Runtime.ImagingBackend == "pure-go" && body.Runtime.WebPEncode {
		t.Error("pure-go build should not report webp_encode")
	}
	if body.Runtime.ImagingBackend == "vips" && !body.Runtime.WebPEncode {
		t.Error("vips build should report webp_encode")
	}
	if !strings.Contains(rec.Body.String(), `"heic_decode"`) {
		t.Error("runtime missing heic_decode")
	}
	if body.Runtime.HeicDecode != imaging.HeicDecodeAvailable() {
		t.Errorf("heic_decode = %v, want %v", body.Runtime.HeicDecode, imaging.HeicDecodeAvailable())
	}
	if body.Runtime.ImagingBackend == "pure-go" && body.Runtime.HeicDecode {
		t.Error("pure-go build should not report heic_decode")
	}
	// default test config is localhost-shaped → expect a base_url check present
	found := false
	for _, c := range body.Doctor.Checks {
		if c.Name == "base_url" {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing base_url doctor check")
	}
}
