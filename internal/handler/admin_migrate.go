package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yixian-huang/imgli/internal/service/storagesvc"
)

type migrateJobStartRequest struct {
	FromPolicyID uint64 `json:"from_policy_id"`
	ToPolicyID   uint64 `json:"to_policy_id"`
	DryRun       bool   `json:"dry_run"`
	DeleteSource bool   `json:"delete_source"`
	Limit        int    `json:"limit"`
}

func migrateJobDTO(j storagesvc.MigrateJobView) map[string]any {
	return map[string]any{
		"id":              j.ID,
		"from_policy_id":  j.FromPolicyID,
		"to_policy_id":    j.ToPolicyID,
		"dry_run":         j.DryRun,
		"delete_source":   j.DeleteSource,
		"limit":           j.Limit,
		"status":          j.Status,
		"progress":        j.Progress,
		"cursor_after_id": j.CursorAfterID,
		"error":           j.Error,
		"created_at":      j.CreatedAt.Format(time.RFC3339),
		"updated_at":      j.UpdatedAt.Format(time.RFC3339),
	}
}

// StartStorageMigrate POST /api/v1/admin/storage/migrate
func (h *AdminHandlers) StartStorageMigrate(w http.ResponseWriter, r *http.Request) {
	if h.D.Res == nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	var req migrateJobStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	job, err := h.D.Res.StartMigrateJob(storagesvc.MigrateJobOpts{
		FromPolicyID: req.FromPolicyID,
		ToPolicyID:   req.ToPolicyID,
		DryRun:       req.DryRun,
		DeleteSource: req.DeleteSource,
		Limit:        req.Limit,
	})
	if err != nil {
		if errors.Is(err, storagesvc.ErrMigrateBusy) {
			Fail(w, http.StatusConflict, CodeForbidden, err.Error())
			return
		}
		if errors.Is(err, storagesvc.ErrMigrateTargetDisabled) {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		// 策略不存在等
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	OK(w, migrateJobDTO(*job))
}

// GetStorageMigrate GET /api/v1/admin/storage/migrate/{id}
func (h *AdminHandlers) GetStorageMigrate(w http.ResponseWriter, r *http.Request) {
	if h.D.Res == nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "缺少任务 id")
		return
	}
	job, ok := h.D.Res.GetMigrateJob(id)
	if !ok {
		Fail(w, http.StatusNotFound, CodeNotFound, "任务不存在")
		return
	}
	OK(w, migrateJobDTO(job))
}
