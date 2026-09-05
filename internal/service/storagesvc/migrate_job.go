package storagesvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
)

const (
	migrateJobBatch = 100

	MigrateJobPending   = "pending"
	MigrateJobRunning   = "running"
	MigrateJobDone      = "done"
	MigrateJobFailed    = "failed"
	MigrateJobCancelled = "cancelled"
)

// MigrateJobOpts 启动 Admin/后台搬迁任务的参数（无策略 Config）。
type MigrateJobOpts struct {
	FromPolicyID uint64
	ToPolicyID   uint64
	DryRun       bool
	DeleteSource bool
	// Limit 总处理上限；0=不限。
	Limit         int
	UserID        *uint64
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}

// MigrateJob 进行中的搬迁任务（含 mutex，禁止值拷贝；对外用 Snapshot）。
type MigrateJob struct {
	ID            string
	FromPolicyID  uint64
	ToPolicyID    uint64
	DryRun        bool
	DeleteSource  bool
	Limit         int
	UserID        *uint64
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Status        string
	Progress      MigrateProgress
	CursorAfterID uint64
	Error         string
	CreatedAt     time.Time
	UpdatedAt     time.Time

	mu sync.Mutex
}

type liveMigrateJob struct {
	job    *MigrateJob
	cancel context.CancelFunc
}

// MigrateJobView 无锁 API 视图（可安全拷贝 / JSON）。
type MigrateJobView struct {
	ID            string          `json:"id"`
	FromPolicyID  uint64          `json:"from_policy_id"`
	ToPolicyID    uint64          `json:"to_policy_id"`
	DryRun        bool            `json:"dry_run"`
	DeleteSource  bool            `json:"delete_source"`
	Limit         int             `json:"limit"`
	Status        string          `json:"status"`
	Progress      MigrateProgress `json:"progress"`
	CursorAfterID uint64          `json:"cursor_after_id"`
	Error         string          `json:"error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Snapshot 返回无锁视图，供 API 序列化。
func (j *MigrateJob) Snapshot() MigrateJobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.snapshotLocked()
}

func (j *MigrateJob) snapshotLocked() MigrateJobView {
	out := MigrateJobView{
		ID: j.ID, FromPolicyID: j.FromPolicyID, ToPolicyID: j.ToPolicyID,
		DryRun: j.DryRun, DeleteSource: j.DeleteSource, Limit: j.Limit,
		Status: j.Status, CursorAfterID: j.CursorAfterID, Error: j.Error,
		CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
		Progress: MigrateProgress{
			Scanned: j.Progress.Scanned, Copied: j.Progress.Copied,
			Skipped: j.Progress.Skipped, Failed: j.Progress.Failed,
		},
	}
	if j.Progress.SamplePaths != nil {
		out.Progress.SamplePaths = append([]string(nil), j.Progress.SamplePaths...)
	}
	if j.Progress.Errors != nil {
		out.Progress.Errors = append([]string(nil), j.Progress.Errors...)
	}
	return out
}

func (j *MigrateJob) setRunning() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = MigrateJobRunning
	j.UpdatedAt = time.Now()
}

func (j *MigrateJob) mergeBatch(batch MigrateResult, cursor uint64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress.Scanned += batch.Scanned
	j.Progress.Copied += batch.Copied
	j.Progress.Skipped += batch.Skipped
	j.Progress.Failed += batch.Failed
	for _, p := range batch.SamplePaths {
		if len(j.Progress.SamplePaths) >= 5 {
			break
		}
		dup := false
		for _, e := range j.Progress.SamplePaths {
			if e == p {
				dup = true
				break
			}
		}
		if !dup {
			j.Progress.SamplePaths = append(j.Progress.SamplePaths, p)
		}
	}
	for _, e := range batch.Errors {
		if len(j.Progress.Errors) >= 20 {
			break
		}
		j.Progress.Errors = append(j.Progress.Errors, e)
	}
	j.CursorAfterID = cursor
	j.UpdatedAt = time.Now()
}

func (j *MigrateJob) finish(status, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = status
	j.Error = errMsg
	j.UpdatedAt = time.Now()
}

func jobFromRow(row *model.StorageMigrateJob) *MigrateJob {
	j := &MigrateJob{
		ID:            row.ID,
		FromPolicyID:  row.FromPolicyID,
		ToPolicyID:    row.ToPolicyID,
		DryRun:        row.DryRun,
		DeleteSource:  row.DeleteSource,
		Limit:         row.Limit,
		UserID:        row.UserID,
		CreatedAfter:  row.CreatedAfter,
		CreatedBefore: row.CreatedBefore,
		Status:        row.Status,
		CursorAfterID: row.CursorAfterID,
		Error:         row.LastError,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		Progress: MigrateProgress{
			Scanned: row.Scanned, Copied: row.Copied,
			Skipped: row.Skipped, Failed: row.Failed,
		},
	}
	if row.SamplePaths != nil {
		j.Progress.SamplePaths = append([]string(nil), row.SamplePaths...)
	}
	if row.Errors != nil {
		j.Progress.Errors = append([]string(nil), row.Errors...)
	}
	return j
}

func viewFromRow(row *model.StorageMigrateJob) MigrateJobView {
	return jobFromRow(row).Snapshot()
}

func (r *Resolver) persistJob(j *MigrateJob) (bool, error) {
	if r.db == nil {
		return false, errors.New("storagesvc: db 未配置")
	}
	snap := j.Snapshot()
	res := r.db.Model(&model.StorageMigrateJob{}).
		Where("id = ? AND status IN ?", snap.ID, []string{MigrateJobPending, MigrateJobRunning}).
		Select("status", "cursor_after_id", "scanned", "copied", "skipped", "failed", "sample_paths", "errors", "last_error", "updated_at").
		Updates(model.StorageMigrateJob{
			Status:        snap.Status,
			CursorAfterID: snap.CursorAfterID,
			Scanned:       snap.Progress.Scanned,
			Copied:        snap.Progress.Copied,
			Skipped:       snap.Progress.Skipped,
			Failed:        snap.Progress.Failed,
			SamplePaths:   snap.Progress.SamplePaths,
			Errors:        snap.Progress.Errors,
			LastError:     snap.Error,
			UpdatedAt:     snap.UpdatedAt,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *Resolver) finishJob(j *MigrateJob, status, errMsg string) {
	j.finish(status, errMsg)
	snap := j.Snapshot()
	res := r.db.Model(&model.StorageMigrateJob{}).
		Where("id = ? AND status IN ?", j.ID, []string{MigrateJobPending, MigrateJobRunning}).
		Select("status", "cursor_after_id", "scanned", "copied", "skipped", "failed", "sample_paths", "errors", "last_error", "updated_at").
		Updates(model.StorageMigrateJob{
			Status:        snap.Status,
			CursorAfterID: snap.CursorAfterID,
			Scanned:       snap.Progress.Scanned,
			Copied:        snap.Progress.Copied,
			Skipped:       snap.Progress.Skipped,
			Failed:        snap.Progress.Failed,
			SamplePaths:   snap.Progress.SamplePaths,
			Errors:        snap.Progress.Errors,
			LastError:     snap.Error,
			UpdatedAt:     snap.UpdatedAt,
		})
	if res.Error != nil {
		slog.Warn("migrate job persist finish failed", "job", j.ID, "err", res.Error)
		return
	}
	if res.RowsAffected == 0 {
		var row model.StorageMigrateJob
		if err := r.db.First(&row, "id = ?", j.ID).Error; err == nil {
			j.mu.Lock()
			j.Status = row.Status
			j.Error = row.LastError
			j.UpdatedAt = row.UpdatedAt
			j.mu.Unlock()
		}
	}
}

func (r *Resolver) errIfActiveJob(fromPolicyID uint64) error {
	if r.db == nil {
		return nil
	}
	var n int64
	if err := r.db.Model(&model.StorageMigrateJob{}).
		Where("from_policy_id = ? AND status IN ?", fromPolicyID, []string{MigrateJobPending, MigrateJobRunning}).
		Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: from=%d", ErrMigrateBusy, fromPolicyID)
	}
	return nil
}

func (r *Resolver) insertActiveJob(row *model.StorageMigrateJob) error {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&model.StorageMigrateJob{}).
			Where("from_policy_id = ? AND status IN ?", row.FromPolicyID, []string{MigrateJobPending, MigrateJobRunning}).
			Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("%w: from=%d", ErrMigrateBusy, row.FromPolicyID)
		}
		return tx.Create(row).Error
	})
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("%w: from=%d", ErrMigrateBusy, row.FromPolicyID)
	}
	return err
}

func (r *Resolver) validateMigrateOpts(opts MigrateJobOpts) error {
	if r.db == nil {
		return errors.New("storagesvc: db 未配置")
	}
	if opts.FromPolicyID == 0 || opts.ToPolicyID == 0 {
		return errors.New("storagesvc: from/to policy id 必填")
	}
	if opts.FromPolicyID == opts.ToPolicyID {
		return errors.New("storagesvc: from 与 to 不能相同")
	}
	if opts.Limit < 0 {
		return errors.New("storagesvc: limit 不能为负")
	}
	var toP model.StoragePolicy
	if err := r.db.First(&toP, opts.ToPolicyID).Error; err != nil {
		return fmt.Errorf("目标策略: %w", err)
	}
	if !toP.Enabled {
		return fmt.Errorf("%w: id=%d", ErrMigrateTargetDisabled, toP.ID)
	}
	if err := r.db.First(&model.StoragePolicy{}, opts.FromPolicyID).Error; err != nil {
		return fmt.Errorf("源策略: %w", err)
	}
	return nil
}

// StartMigrateJob 校验参数、占用 from 互斥、异步批跑，立即返回 job 快照。
func (r *Resolver) StartMigrateJob(opts MigrateJobOpts) (*MigrateJobView, error) {
	if err := r.validateMigrateOpts(opts); err != nil {
		return nil, err
	}
	id, err := newMigrateJobID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	row := &model.StorageMigrateJob{
		ID:            id,
		FromPolicyID:  opts.FromPolicyID,
		ToPolicyID:    opts.ToPolicyID,
		DryRun:        opts.DryRun,
		DeleteSource:  opts.DeleteSource,
		Limit:         opts.Limit,
		UserID:        opts.UserID,
		CreatedAfter:  opts.CreatedAfter,
		CreatedBefore: opts.CreatedBefore,
		Status:        MigrateJobPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := r.insertActiveJob(row); err != nil {
		return nil, err
	}
	go r.runPersistJob(id)
	view := viewFromRow(row)
	return &view, nil
}

// GetMigrateJob 按 id 取快照；不存在返回 false。
func (r *Resolver) GetMigrateJob(id string) (MigrateJobView, bool) {
	r.liveMu.Lock()
	live, ok := r.live[id]
	r.liveMu.Unlock()
	if ok && live.job != nil {
		return live.job.Snapshot(), true
	}
	if r.db == nil {
		return MigrateJobView{}, false
	}
	var row model.StorageMigrateJob
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		return MigrateJobView{}, false
	}
	return viewFromRow(&row), true
}

// ListMigrateJobs 最近任务（updated_at 倒序）。
func (r *Resolver) ListMigrateJobs(limit int) ([]MigrateJobView, error) {
	if r.db == nil {
		return nil, errors.New("storagesvc: db 未配置")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var rows []model.StorageMigrateJob
	if err := r.db.Order("updated_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]MigrateJobView, 0, len(rows))
	for i := range rows {
		if v, ok := r.GetMigrateJob(rows[i].ID); ok {
			out = append(out, v)
			continue
		}
		out = append(out, viewFromRow(&rows[i]))
	}
	return out, nil
}

// CancelMigrateJob 批间协作取消；不回滚已搬对象。
func (r *Resolver) CancelMigrateJob(id string) (MigrateJobView, error) {
	r.liveMu.Lock()
	if live, ok := r.live[id]; ok && live.cancel != nil {
		live.cancel()
	}
	r.liveMu.Unlock()

	now := time.Now()
	res := r.db.Model(&model.StorageMigrateJob{}).
		Where("id = ? AND status IN ?", id, []string{MigrateJobPending, MigrateJobRunning}).
		Updates(map[string]any{"status": MigrateJobCancelled, "updated_at": now})
	if res.Error != nil {
		return MigrateJobView{}, res.Error
	}
	var row model.StorageMigrateJob
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MigrateJobView{}, err
		}
		return MigrateJobView{}, err
	}
	if res.RowsAffected == 0 && row.Status != MigrateJobCancelled {
		return viewFromRow(&row), ErrMigrateNotCancellable
	}
	if live, ok := r.liveSnapshot(id); ok {
		live.finish(MigrateJobCancelled, "")
	}
	return viewFromRow(&row), nil
}

func (r *Resolver) liveSnapshot(id string) (*MigrateJob, bool) {
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	live, ok := r.live[id]
	if !ok || live.job == nil {
		return nil, false
	}
	return live.job, true
}

// ResumeMigrateJob 从 cursor 继续一条 failed 任务。
func (r *Resolver) ResumeMigrateJob(id string) (*MigrateJobView, error) {
	var row model.StorageMigrateJob
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if row.Status != MigrateJobFailed {
		return nil, ErrMigrateNotFailed
	}
	now := time.Now()
	res := r.db.Model(&model.StorageMigrateJob{}).
		Where("id = ? AND status = ?", id, MigrateJobFailed).
		Updates(map[string]any{"status": MigrateJobPending, "last_error": "", "updated_at": now})
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
			return nil, fmt.Errorf("%w: from=%d", ErrMigrateBusy, row.FromPolicyID)
		}
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrMigrateNotFailed
	}
	row.Status = MigrateJobPending
	row.LastError = ""
	row.UpdatedAt = now
	go r.runPersistJob(id)
	view := viewFromRow(&row)
	return &view, nil
}

// RecoverMigrateJobs 启动时把 pending|running 收回继续跑。返回收回条数。
func (r *Resolver) RecoverMigrateJobs() int {
	if r.db == nil {
		return 0
	}
	var rows []model.StorageMigrateJob
	if err := r.db.Where("status IN ?", []string{MigrateJobPending, MigrateJobRunning}).
		Find(&rows).Error; err != nil {
		slog.Warn("recover migrate jobs list failed", "err", err)
		return 0
	}
	for i := range rows {
		id := rows[i].ID
		go r.runPersistJob(id)
	}
	return len(rows)
}

func (r *Resolver) runPersistJob(id string) {
	var row model.StorageMigrateJob
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		slog.Warn("migrate job missing on run", "job", id, "err", err)
		return
	}
	if row.Status != MigrateJobPending && row.Status != MigrateJobRunning {
		return
	}
	job := jobFromRow(&row)
	ctx, cancel := context.WithCancel(context.Background())
	r.liveMu.Lock()
	if r.live == nil {
		r.live = map[string]*liveMigrateJob{}
	}
	r.live[id] = &liveMigrateJob{job: job, cancel: cancel}
	r.liveMu.Unlock()
	defer func() {
		cancel()
		r.liveMu.Lock()
		delete(r.live, id)
		r.liveMu.Unlock()
	}()

	job.setRunning()
	ok, err := r.persistJob(job)
	if err != nil {
		slog.Warn("migrate job persist running failed", "job", id, "err", err)
	}
	if !ok {
		r.finishJob(job, MigrateJobCancelled, "")
		return
	}

	cursor := job.CursorAfterID
	remaining := 0
	if job.Limit > 0 {
		remaining = job.Limit - job.Progress.Scanned
		if remaining <= 0 {
			r.finishJob(job, MigrateJobDone, "")
			return
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			r.finishJob(job, MigrateJobCancelled, "")
			return
		}
		var dbStatus string
		if err := r.db.Model(&model.StorageMigrateJob{}).Select("status").
			Where("id = ?", id).Scan(&dbStatus).Error; err == nil && dbStatus == MigrateJobCancelled {
			r.finishJob(job, MigrateJobCancelled, "")
			return
		}
		batchLimit := migrateJobBatch
		if remaining > 0 && remaining < batchLimit {
			batchLimit = remaining
		}
		batch, err := r.MigrateFiles(ctx, r.db, MigrateOpts{
			FromPolicyID:  job.FromPolicyID,
			ToPolicyID:    job.ToPolicyID,
			DryRun:        job.DryRun,
			DeleteSource:  job.DeleteSource,
			Limit:         batchLimit,
			AfterID:       cursor,
			SkipMutex:     true,
			UserID:        job.UserID,
			CreatedAfter:  job.CreatedAfter,
			CreatedBefore: job.CreatedBefore,
		})
		if batch.LastFileID > 0 {
			cursor = batch.LastFileID
		}
		job.mergeBatch(batch, cursor)
		if ok, perr := r.persistJob(job); perr != nil {
			slog.Warn("migrate job persist progress failed", "job", id, "err", perr)
		} else if !ok {
			r.finishJob(job, MigrateJobCancelled, "")
			return
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				r.finishJob(job, MigrateJobCancelled, "")
				return
			}
			slog.Warn("migrate job batch failed", "job", job.ID, "err", err)
			r.finishJob(job, MigrateJobFailed, err.Error())
			return
		}
		if remaining > 0 {
			remaining -= batch.Scanned
			if remaining <= 0 {
				break
			}
		}
		if batch.Scanned == 0 || batch.Scanned < batchLimit {
			break
		}
	}
	r.finishJob(job, MigrateJobDone, "")
}

func newMigrateJobID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
