package upload

import (
	"bytes"
	"context"
	"errors"
	"os"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/bandwidth"
	"github.com/yixian-huang/imgli/internal/service/settings"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
)

// GuestEnabled 返回 guest_upload_enabled 开关当前值；键缺失或读取异常一律按 false
// 处理（fail closed）。经 settings 30s 缓存；Set 后立即失效。供 Save 与 handler 共用。
func (s *Service) GuestEnabled() bool {
	return s.st.GuestUploadEnabled()
}

// Save 处理一个已落盘的临时文件。临时文件在返回前会被删除。

// Save 处理一个已落盘的临时文件。临时文件在返回前会被删除。
func (s *Service) Save(ctx context.Context, tmpPath, filename string, u *model.User, opts Opts, ip string) (*Result, error) {
	defer os.Remove(tmpPath)
	if u != nil && u.ID == 0 {
		u = nil // 规整零值 ID：使下文 u==nil 的游客判定口径统一
	}

	filename = truncateName(filename)

	// 用户组：配额、单文件上限、允许后缀、可用策略
	var group model.UserGroup
	if u == nil {
		if !s.GuestEnabled() {
			return nil, ErrGuestNotSupported
		}
		if err := s.db.Where("is_guest = ?", true).First(&group).Error; err != nil {
			return nil, err
		}
	} else {
		if err := s.db.First(&group, u.GroupID).Error; err != nil {
			return nil, err
		}
	}
	// 组级有效期 / 访问次数默认与上限（含游客 ForceMaxAge）。
	if err := ApplyGroupAccess(&group, &opts, time.Now()); err != nil {
		return nil, err
	}

	fi, err := os.Stat(tmpPath)
	if err != nil {
		return nil, err
	}
	size := fi.Size()
	if group.MaxFileSize > 0 && size > group.MaxFileSize {
		return nil, ErrFileTooLarge
	}

	// HEIF：allowlist 用原始 heic/heif，转 JPEG 后再 burn/hash。嗅探走文件前缀，不依赖 Probe。
	prefix, err := readPrefix(tmpPath, 64)
	if err != nil {
		return nil, err
	}
	heif := imaging.SniffHEIF(prefix)
	var meta imaging.Meta
	if heif {
		allow := imaging.HEIFAllowExt(filename)
		if !extAllowed(group.AllowedExts, allow) {
			return nil, ErrExtNotAllowed
		}
		if !imaging.HeicDecodeAvailable() {
			return nil, ErrHeicUnavailable
		}
		raw, rerr := os.ReadFile(tmpPath)
		if rerr != nil {
			return nil, rerr
		}
		pmeta, perr := imaging.ProbeHEIF(raw)
		if errors.Is(perr, imaging.ErrHeicUnavailable) {
			return nil, ErrHeicUnavailable
		}
		if perr != nil {
			return nil, ErrInvalidImage
		}
		if pmeta.Width > MaxDimension || pmeta.Height > MaxDimension {
			return nil, ErrDimensionOver
		}
		var proc Processing
		if gerr := s.st.Get(model.SettingProcessing, &proc); gerr != nil {
			if !errors.Is(gerr, settings.ErrNotFound) {
				return nil, gerr
			}
			proc = DefaultProcessing()
		}
		jpeg, jmeta, jerr := imaging.DecodeHEIFToJPEG(raw, proc.EffectiveJPEGQuality())
		if errors.Is(jerr, imaging.ErrHeicUnavailable) {
			return nil, ErrHeicUnavailable
		}
		if errors.Is(jerr, imaging.ErrDimensionOver) {
			return nil, ErrDimensionOver
		}
		if jerr != nil {
			return nil, ErrInvalidImage
		}
		if jmeta.Width > MaxDimension || jmeta.Height > MaxDimension {
			return nil, ErrDimensionOver
		}
		if err := os.WriteFile(tmpPath, jpeg, 0o600); err != nil {
			return nil, err
		}
		meta = jmeta
		size = int64(len(jpeg))
	} else {
		meta, err = s.probe(tmpPath)
		if err != nil {
			return nil, ErrInvalidImage
		}
		if meta.Width > MaxDimension || meta.Height > MaxDimension {
			return nil, ErrDimensionOver
		}
		if !extAllowed(group.AllowedExts, meta.Ext) {
			return nil, ErrExtNotAllowed
		}
	}

	// D-② 处理管线:单次解码→缩放/水印→末次编码(keep|webp),hash 之前烧录(与秒传去重兼容)。
	// 仅 jpg/jpeg/png;gif/webp 不转;无处理项生效时字节完全不动。
	if changed, perr := s.burn(tmpPath, meta.Ext, u); perr != nil {
		return nil, perr
	} else if changed {
		if meta, err = s.probe(tmpPath); err != nil { // 尺寸可能已变,重探
			return nil, ErrInvalidImage
		}
		// 重编码改变了字节数:刷新 size,否则 files.size/配额累加/秒传计费全部用
		// 烧录前旧值长期漂移(codex 评审 Task4)。MaxFileSize 不复检——上限约束的是
		// 用户输入,站点加工的增量不归咎用户。
		fi2, serr := os.Stat(tmpPath)
		if serr != nil {
			return nil, serr
		}
		size = fi2.Size()
	}

	// 内容哈希（独立打开）
	hash, err := s.hashFile(tmpPath)
	if err != nil {
		return nil, err
	}

	// 可见性回退链:显式 > 偏好 > public(visibilityFor 内游客恒 public)
	vis := opts.Visibility
	if vis == "" && u != nil {
		vis = u.Preferences.DefaultVisibility
	}

	// 相册回退链(游客忽略):显式(0=明确不归档) > 偏好;显式无效 400,偏好悬空静默忽略
	albumID, err := s.resolveAlbum(u, opts.AlbumID)
	if err != nil {
		return nil, err
	}
	// 私密相册强制图为 private（须在 surface 计算之前），直链 /i 也不能匿名打开。
	vis = applyAlbumVisibility(s.db, albumID, vis)

	// 策略回退链:显式(须组内 enabled) > 偏好(悬空静默降级) > 组默认
	policy, err := s.resolvePolicy(&group, u, opts.PolicyID)
	if err != nil {
		return nil, err
	}

	// 配额：按 image 记录计，秒传与新传一视同仁（spec §6：去重省的是站点磁盘，不是用户配额）。
	// 快速预检减少无谓落盘；事务内 addUsedStorage 是权威门禁，堵住并发 TOCTOU。
	// 游客无账号、不计配额（游客组 storage_quota=0），跳过读取避免以 id=0 误查。
	if u != nil && group.StorageQuota > 0 {
		var freshUsed int64
		if err := s.db.Model(&model.User{}).Where("id = ?", u.ID).
			Select("used_storage").Scan(&freshUsed).Error; err != nil {
			return nil, err
		}
		if freshUsed+size > group.StorageQuota {
			return nil, ErrQuotaExceeded
		}
	}
	// 月流量硬顶：到顶禁上传（出图侧 serve 另拦）。
	if u != nil {
		if err := bandwidth.Check(s.db, u.ID); errors.Is(err, bandwidth.ErrExceeded) {
			return nil, ErrBandwidthExceeded
		} else if err != nil {
			return nil, err
		}
	}

	// 秒传：按 (hash, surface) 命中——私密只与私密去重，撞公开字节则走落盘分支复制独立私密对象。
	surface := visibilityFor(u, vis)
	var existing model.File
	hit := s.db.First(&existing, "hash = ? AND surface = ?", hash, surface).Error == nil
	if hit {
		// 链接策略：复用既有文件所在策略(架构 spec「全局去重优先」)。
		var filePolicy model.StoragePolicy
		if err := s.db.First(&filePolicy, existing.StoragePolicyID).Error; err == nil {
			policy = &filePolicy
		}
		// 同用户 + 同选项 live 图 → 幂等返回原 key（图库不重复、不二次扣配额）。
		// 跨用户/游客/选项不同仍走 commitInstant 新建 image。
		if u != nil {
			if prev, ok := s.findReusableLiveImage(u.ID, existing.ID, surface, albumID, opts); ok {
				if filename != "" && prev.Name != filename {
					_ = s.db.Model(prev).Update("name", filename).Error
					prev.Name = filename
				}
				// 刷新 File.RefCount 展示用（未 +1）
				_ = s.db.First(&existing, existing.ID).Error
				return &Result{Image: prev, File: &existing, Policy: policy, Instant: true, Reused: true}, nil
			}
		}
		img, err := s.commitInstant(u, &existing, filename, meta.Ext, vis, ip, size, albumID, opts.ExpiresAt, opts.MaxViews, opts.AccessPasswordHash, group.StorageQuota)
		if err != nil {
			return nil, err
		}
		// 只要没继承到 rejected/pending 实结论就自审:机审先写 score 再改 status(两次独立写),
		// "有分" 可能是机审中途(score 已写、status 未改),据此跳过会让新图永久停在 normal。
		// 仅继承到 rejected/pending(status 非 normal)才跳过——那才是真定论。
		if img.Status == "normal" {
			s.enqueueModerate(img.ID)
		}
		return &Result{Image: img, File: &existing, Policy: policy, Instant: true, Reused: false}, nil
	}

	// 落盘：渲染路径 → 驱动 Put
	driver, err := s.res.Driver(policy)
	if err != nil {
		return nil, err
	}
	relPath, err := s.res.RenderPath(policy.PathTemplate, meta.Ext, time.Now())
	if err != nil {
		return nil, err
	}
	relPath = storagesvc.SurfacePrefix(surface) + relPath // surface 前缀由写入代码加,不进模板
	if err := s.putFile(ctx, driver, relPath, tmpPath); err != nil {
		return nil, err
	}

	// 缩略图（同步）：失败不阻断上传——生产可接受缩略图缺失后台补
	thumb, terr := s.thumbnail(tmpPath)

	// 事务：建 file + image + 累加 used_storage
	// 内容安全 M-A：新建分支跨 surface 按 hash 继承同字节已有的最严审核结论——
	// scoped-dedup 下私密撞公开(或反之)字节会新建独立 File,不继承则已拒/待审内容
	// 会以新 surface 复活为 normal(绕过 commitInstant 的同 file 继承)。
	inhStatus, inhScore := inheritModerationByHash(s.db, hash)
	img := &model.Image{
		Name: filename, Ext: meta.Ext, Visibility: visibilityFor(u, vis),
		Status: inhStatus, NSFWScore: inhScore, UploadIP: ip, AlbumID: albumID, ExpiresAt: opts.ExpiresAt,
		MaxViews:           opts.MaxViews,
		AccessPasswordHash: opts.AccessPasswordHash,
	}
	if u != nil {
		img.UserID = &u.ID
	}
	file := &model.File{
		Hash: hash, Surface: surface, StoragePolicyID: policy.ID, Path: relPath,
		Size: size, MIME: meta.MIME, Width: meta.Width, Height: meta.Height, RefCount: 1,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(file).Error; err != nil {
			return err
		}
		img.FileID = file.ID
		key, err := s.uniqueKey(tx)
		if err != nil {
			return err
		}
		img.Key = key
		if err := tx.Create(img).Error; err != nil {
			return err
		}
		if u == nil {
			return nil // 游客不计配额，不累加 used_storage
		}
		return addUsedStorage(tx, u.ID, size, group.StorageQuota)
	})
	if err != nil {
		// 回滚：物理文件已落盘但记录失败 → 投递删除任务
		s.enqueueDelete(policy.ID, relPath)
		return nil, err
	}

	// 缩略图落盘（事务外，按文件哈希寻址：去重图共享同一缩略图）
	if terr == nil && thumb != nil {
		thumbKey := storagesvc.ThumbKey(surface, hash)
		if s.proc.ThumbExt() == "webp" {
			thumbKey = storagesvc.ThumbKeyWebP(surface, hash) // vips 构建产 WebP,键随格式(D-② 双探测对齐)
		}
		if err := driver.Put(ctx, thumbKey, bytes.NewReader(thumb)); err != nil {
			// 缩略图失败不影响主流程，直链 /t 会 404，可接受
			_ = err
		}
	}
	// 只要没继承到 rejected/pending 实结论就自审:机审先写 score 再改 status(两次独立写),
	// "有分" 可能是机审中途(score 已写、status 未改),据此跳过会让新图永久停在 normal。
	// 仅继承到 rejected/pending(status 非 normal)才跳过——那才是真定论;唯一 hash 的
	// 普通上传(无 sibling)仍 normal 且无分,照常入队。
	if img.Status == "normal" {
		s.enqueueModerate(img.ID)
	}
	return &Result{Image: img, File: file, Policy: policy, Instant: false}, nil
}

// burn 就地处理临时文件(缩放→站点文字水印→用户图片水印),返回是否发生改写。
// 仅 jpg/jpeg/png。settings 仅「键不存在」按默认降级,其余读取错误上抛中止——
// DB 故障静默全关会绕过已启用的站点水印策略(codex 评审 Task4)。
// 处理链任一步 ErrUnsupported(内容与扩展名不符等)→ 整链放弃、原字节不动、
// 记 slog.Warn:不写回部分处理的中间结果(原子降级语义,codex 评审 Task4)。
