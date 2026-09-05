package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/service/storagesvc"
)

type migrateJobStartRequest struct {
	FromPolicyID  uint64  `json:"from_policy_id"`
	ToPolicyID    uint64  `json:"to_policy_id"`
	DryRun        bool    `json:"dry_run"`
	DeleteSource  bool    `json:"delete_source"`
	Limit         int     `json:"limit"`
	UserID        *uint64 `json:"user_id"`
	CreatedAfter  *string `json:"created_after"`  // RFC3339
	CreatedBefore *string `json:"created_before"` // RFC3339
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

func writeMigrateJobErr(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "任务不存在")
		return
	}
	if errors.Is(err, storagesvc.ErrMigrateBusy) {
		Fail(w, http.StatusConflict, CodeForbidden, err.Error())
		return
	}
	if errors.Is(err, storagesvc.ErrMigrateTargetDisabled) ||
		errors.Is(err, storagesvc.ErrMigrateNotFailed) ||
		errors.Is(err, storagesvc.ErrMigrateNotCancellable) {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
}

func parseMigrateOpts(req migrateJobStartRequest) (storagesvc.MigrateJobOpts, error) {
	opts := storagesvc.MigrateJobOpts{
		FromPolicyID: req.FromPolicyID,
		ToPolicyID:   req.ToPolicyID,
		DryRun:       req.DryRun,
		DeleteSource: req.DeleteSource,
		Limit:        req.Limit,
		UserID:       req.UserID,
	}
	if req.CreatedAfter != nil && *req.CreatedAfter != "" {
		t, err := time.Parse(time.RFC3339, *req.CreatedAfter)
		if err != nil {
			return opts, errors.New("created_after 需 RFC3339")
		}
		opts.CreatedAfter = &t
	}
	if req.CreatedBefore != nil && *req.CreatedBefore != "" {
		t, err := time.Parse(time.RFC3339, *req.CreatedBefore)
		if err != nil {
			return opts, errors.New("created_before 需 RFC3339")
		}
		opts.CreatedBefore = &t
	}
	return opts, nil
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
	opts, err := parseMigrateOpts(req)
	if err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	job, err := h.D.Res.StartMigrateJob(opts)
	if err != nil {
		writeMigrateJobErr(w, err)
		return
	}
	actor := PrincipalFrom(r).User
	h.D.Adm.Audit(&actor.ID, "admin", "storage_migrate_start",
		map[string]any{"id": job.ID, "from": job.FromPolicyID, "to": job.ToPolicyID, "dry_run": job.DryRun}, ClientIP(r))
	OK(w, migrateJobDTO(*job))
}

// ListStorageMigrate GET /api/v1/admin/storage/migrate
func (h *AdminHandlers) ListStorageMigrate(w http.ResponseWriter, r *http.Request) {
	if h.D.Res == nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	jobs, err := h.D.Res.ListMigrateJobs(20)
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	items := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, migrateJobDTO(j))
	}
	OK(w, map[string]any{"items": items})
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

// CancelStorageMigrate POST /api/v1/admin/storage/migrate/{id}/cancel
func (h *AdminHandlers) CancelStorageMigrate(w http.ResponseWriter, r *http.Request) {
	if h.D.Res == nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	id := chi.URLParam(r, "id")
	job, err := h.D.Res.CancelMigrateJob(id)
	if err != nil {
		writeMigrateJobErr(w, err)
		return
	}
	actor := PrincipalFrom(r).User
	h.D.Adm.Audit(&actor.ID, "admin", "storage_migrate_cancel",
		map[string]any{"id": job.ID, "from": job.FromPolicyID}, ClientIP(r))
	OK(w, migrateJobDTO(job))
}

// ResumeStorageMigrate POST /api/v1/admin/storage/migrate/{id}/resume
func (h *AdminHandlers) ResumeStorageMigrate(w http.ResponseWriter, r *http.Request) {
	if h.D.Res == nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	id := chi.URLParam(r, "id")
	job, err := h.D.Res.ResumeMigrateJob(id)
	if err != nil {
		writeMigrateJobErr(w, err)
		return
	}
	actor := PrincipalFrom(r).User
	h.D.Adm.Audit(&actor.ID, "admin", "storage_migrate_resume",
		map[string]any{"id": job.ID, "from": job.FromPolicyID}, ClientIP(r))
	OK(w, migrateJobDTO(*job))
}
