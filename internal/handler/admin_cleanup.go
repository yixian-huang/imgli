package handler

import (
	"encoding/json"
	"net/http"

	"github.com/yixian-huang/imgli/internal/service/imagesvc"
)

// AdminCleanupDeps 需要 Image service。
// 通过 AdminDeps 扩展 Img 字段更干净，这里复用已有 D 上可能没有 Img——改为从 server 注入。

type cleanupPreviewRequest struct {
	Kinds []string `json:"kinds"`
}

type cleanupRunRequest struct {
	Kinds   []string `json:"kinds"`
	Limit   int      `json:"limit"`
	Confirm bool     `json:"confirm"`
}

// PreviewCleanup POST /api/v1/admin/cleanup/preview
func (h *AdminHandlers) PreviewCleanup(w http.ResponseWriter, r *http.Request) {
	if h.D.Img == nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	var req cleanupPreviewRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	previews, err := h.D.Img.PreviewCleanup(req.Kinds, 10)
	if err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	OK(w, map[string]any{"items": previews})
}

// RunCleanup POST /api/v1/admin/cleanup/run
func (h *AdminHandlers) RunCleanup(w http.ResponseWriter, r *http.Request) {
	if h.D.Img == nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	var req cleanupRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	if !req.Confirm {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "confirm=true required")
		return
	}
	results, err := h.D.Img.RunCleanup(r.Context(), req.Kinds, req.Limit, true)
	if err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	OK(w, map[string]any{"items": results})
}

// silence unused import if kinds const referenced in docs only
var _ = imagesvc.CleanupExpired
