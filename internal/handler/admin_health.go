package handler

import (
	"net/http"
	"strings"

	"github.com/yixian-huang/imgli/internal/doctor"
	"github.com/yixian-huang/imgli/internal/imaging"
	appver "github.com/yixian-huang/imgli/internal/version"
)

// GetSystemHealth GET /api/v1/admin/system/health
// 管理员只读：复用 imgli doctor 检查 + 当前请求/配置的运行时摘要。
// 不包含密钥；不修改任何配置。
func (h *AdminHandlers) GetSystemHealth(w http.ResponseWriter, r *http.Request) {
	if h.D.Cfg == nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	rep := doctor.Run(h.D.Cfg)
	checks := make([]map[string]any, 0, len(rep.Checks))
	for _, c := range rep.Checks {
		checks = append(checks, map[string]any{
			"name":    c.Name,
			"level":   string(c.Level),
			"message": c.Message,
		})
	}

	install := "binary"
	if appver.IsDockerish() {
		install = "docker"
	}

	fwdProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if i := strings.IndexByte(fwdProto, ','); i >= 0 {
		fwdProto = strings.TrimSpace(fwdProto[:i])
	}
	fwdFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))

	OK(w, map[string]any{
		"doctor": map[string]any{
			"hard_fail": rep.HardFail,
			"checks":    checks,
		},
		"runtime": map[string]any{
			"version":           appver.Version,
			"base_url":          strings.TrimRight(strings.TrimSpace(h.D.Cfg.BaseURL), "/"),
			"trust_proxy":       h.D.Cfg.TrustProxy,
			"listen":            h.D.Cfg.Listen,
			"data_dir":          h.D.Cfg.DataDir,
			"install":           install,
			"request_host":      r.Host,
			"forwarded_proto":   fwdProto,
			"forwarded_for_set": fwdFor != "",
			// 图像后端：Docker 默认发行带 vips；纯 Go 二进制为 pure-go。
			"imaging_backend":   imaging.Backend(),
			"webp_encode":       imaging.WebPEncodeAvailable(),
			"heic_decode":       imaging.HeicDecodeAvailable(),
			"thumb_ext":         imaging.New().ThumbExt(),
		},
	})
}
