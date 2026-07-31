package storagesvc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/storage"
)

// 搬迁安全契约（v0.6 M2）：同 from 互斥、目标必须启用、进度/错误对 Admin 安全（无密钥）。
var (
	// ErrMigrateBusy 同一源策略上已有搬迁在进行（进程内互斥；Admin job 将复用）。
	ErrMigrateBusy = errors.New("storagesvc: 源策略上已有搬迁任务进行中")
	// ErrMigrateTargetDisabled 目标策略 Enabled=false。
	ErrMigrateTargetDisabled = errors.New("storagesvc: 目标策略未启用")
)

// MigrateOpts 跨存储策略搬迁 file 行及其对象键（含缩略图）。
// 路径(file.Path)不变，仅改 storage_policy_id；适合 local→S3 同键复制。
type MigrateOpts struct {
	FromPolicyID uint64
	ToPolicyID   uint64
	// DryRun 只统计将迁移的条数，不写盘、不改库。
	DryRun bool
	// DeleteSource 成功复制并改库后删除源策略上的对象（含 .thumbs）。
	DeleteSource bool
	// Limit 最多处理 N 条；0=不限。
	Limit int
	// AfterID 仅处理 id > AfterID 的行（升序），用于批处理续跑 cursor。
	AfterID uint64
	// SkipMutex 为 true 时不获取/释放 from 互斥（由外层 job 持有锁时使用）。
	SkipMutex bool
	// UserID 非空时只搬该用户图片引用的 file（images.file_id）。
	UserID *uint64
	// CreatedAfter / CreatedBefore 按 files.created_at 过滤（含边界）。
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	// VerifySize Put 后校验目标对象可读长度与 files.size；不一致则不改 policy（计 Failed）。
	// 默认 true；仅测试可关。
	VerifySize *bool
}

// MigrateResult 搬迁汇总（可直接作为 Admin 进度负载的字段源；不含策略 Config）。
type MigrateResult struct {
	Scanned int
	Copied  int
	Skipped int // 已在目标策略 / 源对象缺失
	Failed  int
	// Errors 至多保留前 20 条可读错误，防日志爆炸；不得含存储密钥。
	Errors []string
	// SamplePaths 脱敏后的 path 样例（至多 5 条），供 Admin 展示。
	SamplePaths []string
	// LastFileID 本批处理到的最大 file id（0=未处理任何行），供 cursor 续跑。
	LastFileID uint64
}

// MigrateProgress 是 Admin/API 安全进度视图：仅计数 + 脱敏 path + 错误摘要，无策略凭据。
type MigrateProgress struct {
	Scanned     int      `json:"scanned"`
	Copied      int      `json:"copied"`
	Skipped     int      `json:"skipped"`
	Failed      int      `json:"failed"`
	SamplePaths []string `json:"sample_paths,omitempty"`
	Errors      []string `json:"errors,omitempty"`
}

// Progress 返回可序列化的安全进度（拷贝切片，避免调用方改内部状态）。
func (r MigrateResult) Progress() MigrateProgress {
	p := MigrateProgress{
		Scanned: r.Scanned,
		Copied:  r.Copied,
		Skipped: r.Skipped,
		Failed:  r.Failed,
	}
	if len(r.SamplePaths) > 0 {
		p.SamplePaths = append([]string(nil), r.SamplePaths...)
	}
	if len(r.Errors) > 0 {
		p.Errors = append([]string(nil), r.Errors...)
	}
	return p
}

// RedactStoragePath 脱敏对象键：保留末两段 path，前缀以 … 代替，避免完整盘符/桶内层级泄露过多。
// 空串返回 ""；不含密钥字段——调用方不得把 policy.Config 拼进 path。
func RedactStoragePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	// 统一分隔符
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	if p == "." || p == "/" {
		return p
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) <= 2 {
		return strings.Join(parts, "/")
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}

// MigrateFiles 将 from 策略上的 files 复制到 to 策略并改指向。
// 缩略图：.thumbs/{hash}.jpg 与 .webp 若存在则一并复制（缺失不失败）。
// 同一 FromPolicyID 进程内互斥：并发第二次调用返回 ErrMigrateBusy（SkipMutex 除外）。
func (r *Resolver) MigrateFiles(ctx context.Context, db *gorm.DB, opts MigrateOpts) (MigrateResult, error) {
	var out MigrateResult
	if opts.FromPolicyID == 0 || opts.ToPolicyID == 0 {
		return out, errors.New("storagesvc: from/to policy id 必填")
	}
	if opts.FromPolicyID == opts.ToPolicyID {
		return out, errors.New("storagesvc: from 与 to 不能相同")
	}
	if !opts.SkipMutex {
		if err := r.tryBeginMigrate(opts.FromPolicyID); err != nil {
			return out, err
		}
		defer r.endMigrate(opts.FromPolicyID)
	}

	var fromP, toP model.StoragePolicy
	if err := db.First(&fromP, opts.FromPolicyID).Error; err != nil {
		return out, fmt.Errorf("源策略: %w", err)
	}
	if err := db.First(&toP, opts.ToPolicyID).Error; err != nil {
		return out, fmt.Errorf("目标策略: %w", err)
	}
	if !toP.Enabled {
		return out, fmt.Errorf("%w: id=%d", ErrMigrateTargetDisabled, toP.ID)
	}
	src, err := r.Driver(&fromP)
	if err != nil {
		return out, fmt.Errorf("源驱动: %w", err)
	}
	dst, err := r.Driver(&toP)
	if err != nil {
		return out, fmt.Errorf("目标驱动: %w", err)
	}

	q := db.Model(&model.File{}).Where("storage_policy_id = ?", opts.FromPolicyID).Order("id")
	if opts.AfterID > 0 {
		q = q.Where("id > ?", opts.AfterID)
	}
	if opts.UserID != nil {
		sub := db.Model(&model.Image{}).Select("file_id").Where("user_id = ?", *opts.UserID)
		q = q.Where("id IN (?)", sub)
	}
	if opts.CreatedAfter != nil {
		q = q.Where("created_at >= ?", *opts.CreatedAfter)
	}
	if opts.CreatedBefore != nil {
		q = q.Where("created_at <= ?", *opts.CreatedBefore)
	}
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	verifySize := true
	if opts.VerifySize != nil {
		verifySize = *opts.VerifySize
	}
	var files []model.File
	if err := q.Find(&files).Error; err != nil {
		return out, err
	}

	for i := range files {
		out.Scanned++
		f := &files[i]
		if f.ID > out.LastFileID {
			out.LastFileID = f.ID
		}
		if err := ctx.Err(); err != nil {
			out.addErr(fmt.Sprintf("file#%d: %v", f.ID, err))
			out.Failed++
			return out, err
		}
		out.notePath(f.Path)
		if opts.DryRun {
			// 源对象存在性探测：不存在则记 skipped
			ok, e := src.Exists(ctx, f.Path)
			if e != nil {
				out.Failed++
				out.addErr(fmt.Sprintf("file#%d exists: %v", f.ID, e))
				continue
			}
			if !ok {
				out.Skipped++
				continue
			}
			out.Copied++ // dry-run 语义：将复制
			continue
		}
		if err := copyObject(ctx, src, dst, f.Path); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				out.Skipped++
				slog.Warn("migrate 跳过缺失源对象", "file_id", f.ID, "path", RedactStoragePath(f.Path))
				continue
			}
			out.Failed++
			out.addErr(fmt.Sprintf("file#%d put %s: %v", f.ID, RedactStoragePath(f.Path), err))
			continue
		}
		if verifySize && f.Size > 0 {
			got, serr := objectSize(ctx, dst, f.Path)
			if serr != nil || got != f.Size {
				out.Failed++
				out.addErr(fmt.Sprintf("file#%d size mismatch path=%s want=%d got=%d err=%v",
					f.ID, RedactStoragePath(f.Path), f.Size, got, serr))
				_ = dst.Delete(ctx, f.Path) // 不留下半截对象冒充成功
				continue
			}
		}
		// 缩略图 best-effort：现行世代 + 遗留键一并搬
		for _, tk := range ThumbKeyCandidates(f.Surface, f.Hash) {
			if err := copyObject(ctx, src, dst, tk); err != nil && !errors.Is(err, storage.ErrNotFound) {
				slog.Warn("migrate 缩略图失败(不阻断)", "file_id", f.ID, "key", RedactStoragePath(tk), "err", err)
			}
		}
		res := db.Model(&model.File{}).
			Where("id = ? AND storage_policy_id = ?", f.ID, opts.FromPolicyID).
			Update("storage_policy_id", opts.ToPolicyID)
		if res.Error != nil {
			out.Failed++
			out.addErr(fmt.Sprintf("file#%d update: %v", f.ID, res.Error))
			continue
		}
		if res.RowsAffected == 0 {
			// 并发已迁走
			out.Skipped++
			continue
		}
		out.Copied++
		if opts.DeleteSource {
			_ = src.Delete(ctx, f.Path)
			for _, tk := range ThumbKeyCandidates(f.Surface, f.Hash) {
				_ = src.Delete(ctx, tk)
			}
		}
	}
	return out, nil
}

// tryBeginMigrate 登记源策略上的搬迁占用；已占用则 ErrMigrateBusy。
func (r *Resolver) tryBeginMigrate(fromPolicyID uint64) error {
	r.migrateMu.Lock()
	defer r.migrateMu.Unlock()
	if r.migrateActive == nil {
		r.migrateActive = make(map[uint64]struct{})
	}
	if _, ok := r.migrateActive[fromPolicyID]; ok {
		return fmt.Errorf("%w: from=%d", ErrMigrateBusy, fromPolicyID)
	}
	r.migrateActive[fromPolicyID] = struct{}{}
	return nil
}

func (r *Resolver) endMigrate(fromPolicyID uint64) {
	r.migrateMu.Lock()
	defer r.migrateMu.Unlock()
	delete(r.migrateActive, fromPolicyID)
}

// MigrateActive 报告 from 策略是否正在本进程搬迁（Admin 展示用；不含密钥）。
func (r *Resolver) MigrateActive(fromPolicyID uint64) bool {
	r.migrateMu.Lock()
	defer r.migrateMu.Unlock()
	_, ok := r.migrateActive[fromPolicyID]
	return ok
}

func copyObject(ctx context.Context, src, dst storage.Driver, key string) error {
	rc, err := src.Open(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()
	// 确保从开头读（部分驱动 Seek 支持）
	if _, err := rc.Seek(0, io.SeekStart); err != nil {
		// 不可 seek 时假设已在起点
		_ = err
	}
	return dst.Put(ctx, key, rc)
}

func objectSize(ctx context.Context, d storage.Driver, key string) (int64, error) {
	rc, err := d.Open(ctx, key)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	return rc.Seek(0, io.SeekEnd)
}

func (r *MigrateResult) addErr(msg string) {
	if len(r.Errors) < 20 {
		r.Errors = append(r.Errors, msg)
	}
}

func (r *MigrateResult) notePath(p string) {
	if len(r.SamplePaths) >= 5 {
		return
	}
	rp := RedactStoragePath(p)
	if rp == "" {
		return
	}
	for _, s := range r.SamplePaths {
		if s == rp {
			return
		}
	}
	r.SamplePaths = append(r.SamplePaths, rp)
}
