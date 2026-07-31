package handler

import (
	"net/http"

	"github.com/yixian-huang/imgli/internal/mail"
	"github.com/yixian-huang/imgli/internal/service/adminsvc"
	"github.com/yixian-huang/imgli/internal/service/imagesvc"
	"github.com/yixian-huang/imgli/internal/service/moderation"
	"github.com/yixian-huang/imgli/internal/service/stats"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/service/webhook"
)

// AdminDeps 管理端 handler 依赖。
type AdminDeps struct {
	Adm     *adminsvc.Service
	Res     *storagesvc.Resolver
	Img     *imagesvc.Service // 生命周期清理等
	Mail    *mail.Service
	Stats   *stats.Service
	Mod     *moderation.Service // 可选；拒绝通知
	Hooks   *webhook.Service    // 可选；出站 webhook
	OwnHost string              // BaseURL host，用于 referer suspect 排除自站
}

type AdminHandlers struct{ D AdminDeps }

// Stats GET /api/v1/admin/stats
func (h *AdminHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.D.Adm.StatsWithOwnHost(h.D.OwnHost)
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	OK(w, st)
}

// RefererImages GET /api/v1/admin/referers/images?host=&days=&limit=
func (h *AdminHandlers) RefererImages(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "host 必填")
		return
	}
	days, limit := ParseDaysLimit(r, 30, 20, 100)
	rows, err := h.D.Adm.TopImagesByRefererHost(host, days, limit)
	if err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	OK(w, map[string]any{"host": host, "items": rows})
}
