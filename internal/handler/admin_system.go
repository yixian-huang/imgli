package handler

import (
	"net/http"
	"time"

	appver "github.com/yixian-huang/imgli/internal/version"
)

// GetSystemVersion GET /api/v1/admin/system/version
func (h *AdminHandlers) GetSystemVersion(w http.ResponseWriter, r *http.Request) {
	OK(w, map[string]any{
		"current": appver.Version,
		"repo":    appver.DefaultReleaseRepo,
	})
}

// CheckSystemUpdate POST /api/v1/admin/system/check-update
// 操作员显式触发；探测 GitHub releases/latest（无 token、无静默 phone-home）。
func (h *AdminHandlers) CheckSystemUpdate(w http.ResponseWriter, r *http.Request) {
	res := appver.CheckLatestRelease(r.Context(), appver.DefaultReleaseRepo, nil)
	// 始终 200：探测失败写 error 字段，避免网络问题被当成 5xx
	OK(w, map[string]any{
		"current":          res.Current,
		"latest":           res.Latest,
		"update_available": res.UpdateAvailable,
		"release_url":      res.ReleaseURL,
		"checked_at":       res.CheckedAt.Format(time.RFC3339),
		"error":            res.Error,
		"repo":             appver.DefaultReleaseRepo,
	})
}
