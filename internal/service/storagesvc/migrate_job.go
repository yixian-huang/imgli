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

	MigrateJobPending = "pending"
	MigrateJobRunning = "running"
	MigrateJobDone    = "done"
	MigrateJobFailed  = "failed"
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

// MigrateJob 进程内搬迁任务（含 mutex，禁止值拷贝；对外用 Snapshot）。
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

// StartMigrateJob 校验参数、占用 from 互斥、异步批跑，立即返回 job 快照。
// 进程重启后内存 job 丢失；重新发起即可（已 cutover 的 file 不再命中 from 策略，天然幂等）。
func (r *Resolver) StartMigrateJob(opts MigrateJobOpts) (*MigrateJobView, error) {
	db := r.db
	if db == nil {
		return nil, errors.New("storagesvc: db 未配置")
	}
	if opts.FromPolicyID == 0 || opts.ToPolicyID == 0 {
		return nil, errors.New("storagesvc: from/to policy id 必填")
	}
	if opts.FromPolicyID == opts.ToPolicyID {
		return nil, errors.New("storagesvc: from 与 to 不能相同")
	}
	if opts.Limit < 0 {
		return nil, errors.New("storagesvc: limit 不能为负")
	}
	// 预检目标启用（真正跑时还会再查）
	var toP model.StoragePolicy
	if err := db.First(&toP, opts.ToPolicyID).Error; err != nil {
		return nil, fmt.Errorf("目标策略: %w", err)
	}
	if !toP.Enabled {
		return nil, fmt.Errorf("%w: id=%d", ErrMigrateTargetDisabled, toP.ID)
	}
	if err := db.First(&model.StoragePolicy{}, opts.FromPolicyID).Error; err != nil {
		return nil, fmt.Errorf("源策略: %w", err)
	}

	if err := r.tryBeginMigrate(opts.FromPolicyID); err != nil {
		return nil, err
	}

	id, err := newMigrateJobID()
	if err != nil {
		r.endMigrate(opts.FromPolicyID)
		return nil, err
	}
	now := time.Now()
	job := &MigrateJob{
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
	r.jobsMu.Lock()
	if r.jobs == nil {
		r.jobs = make(map[string]*MigrateJob)
	}
	r.jobs[id] = job
	r.jobsMu.Unlock()

	go r.runMigrateJob(db, job)
	snap := job.Snapshot()
	return &snap, nil
}

// GetMigrateJob 按 id 取快照；不存在返回 false。
func (r *Resolver) GetMigrateJob(id string) (MigrateJobView, bool) {
	r.jobsMu.Lock()
	j, ok := r.jobs[id]
	r.jobsMu.Unlock()
	if !ok {
		return MigrateJobView{}, false
	}
	return j.Snapshot(), true
}

func (r *Resolver) runMigrateJob(db *gorm.DB, job *MigrateJob) {
	defer r.endMigrate(job.FromPolicyID)
	job.setRunning()

	cursor := uint64(0)
	remaining := job.Limit // 0 = unlimited
	ctx := context.Background()

	for {
		batchLimit := migrateJobBatch
		if remaining > 0 && remaining < batchLimit {
			batchLimit = remaining
		}
		batch, err := r.MigrateFiles(ctx, db, MigrateOpts{
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
		if err != nil {
			slog.Warn("migrate job batch failed", "job", job.ID, "err", err)
			job.finish(MigrateJobFailed, err.Error())
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
	job.finish(MigrateJobDone, "")
}

func newMigrateJobID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
