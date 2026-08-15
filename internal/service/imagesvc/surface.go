package imagesvc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/storage"
)

func fileSurface(f model.File) string {
	if f.Surface == "" {
		return model.SurfacePublic
	}
	return f.Surface
}

// rehomeWithUpdates 把 img 从 oldFile 重挂到 target surface，并在同一事务写入 updates。
// 复制对象在事务外；调 ref_count，旧 File 归零则投递异步 purge。
func (s *Service) rehomeWithUpdates(img *model.Image, oldFile *model.File, target string, updates map[string]any) error {
	if updates == nil {
		updates = map[string]any{}
	}
	var policy model.StoragePolicy
	if err := s.db.First(&policy, oldFile.StoragePolicyID).Error; err != nil {
		return err
	}
	newFile, err := s.resolveFileForSurface(&policy, oldFile, target)
	if err != nil {
		return err
	}
	updates["file_id"] = newFile.ID
	var pd *physicalDelete
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// CAS 重挂:仅当 img 仍指向 oldFile 才重挂——防并发切换重复调 ref。
		res := tx.Model(&model.Image{}).Where("id = ? AND file_id = ?", img.ID, oldFile.ID).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errRehomeConflict
		}
		r1 := tx.Model(&model.File{}).Where("id = ?", newFile.ID).
			UpdateColumn("ref_count", gorm.Expr("ref_count + 1"))
		if r1.Error != nil {
			return r1.Error
		}
		if r1.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&model.File{}).Where("id = ?", oldFile.ID).
			UpdateColumn("ref_count", gorm.Expr("ref_count - ?", 1)).Error; err != nil {
			return err
		}
		del := tx.Where("id = ? AND ref_count <= 0", oldFile.ID).Delete(&model.File{})
		if del.Error != nil {
			return del.Error
		}
		if del.RowsAffected == 1 {
			pd = &physicalDelete{
				policyID: oldFile.StoragePolicyID, path: oldFile.Path,
				thumbs: storagesvc.ThumbKeyCandidates(oldFile.Surface, oldFile.Hash),
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.enqueuePhysical(pd)
	return nil
}

// RehomeMismatchedSurfaces 修复 visibility 与 files.surface 不一致的 live 图（空 surface ≈ public）。
// 启动时 best-effort：单张失败记日志并跳过，不中断其余。
func (s *Service) RehomeMismatchedSurfaces() (int, error) {
	var imgs []model.Image
	err := s.db.Model(&model.Image{}).
		Select("images.*").
		Joins("JOIN files ON files.id = images.file_id").
		Where("images.deleted_at IS NULL").
		Where("COALESCE(NULLIF(files.surface, ''), ?) <> images.visibility", model.SurfacePublic).
		Find(&imgs).Error
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range imgs {
		img := &imgs[i]
		var oldFile model.File
		if err := s.db.First(&oldFile, img.FileID).Error; err != nil {
			slog.Warn("rehome mismatched: load file", "key", img.Key, "err", err)
			continue
		}
		if fileSurface(oldFile) == img.Visibility {
			continue
		}
		if err := s.rehomeWithUpdates(img, &oldFile, img.Visibility, nil); err != nil {
			if errors.Is(err, errRehomeConflict) {
				n++
				continue
			}
			slog.Warn("rehome mismatched: failed", "key", img.Key, "err", err)
			continue
		}
		n++
	}
	return n, nil
}

// copyObjectKey 把 src 键对象复制到 dst 键。读入内存再 Put:s3 Put 需 Content-Length
// (bodyLen 认 bytes.Reader.Len()),而 driver.Open 返回的 reader(s3 rangeReadSeekCloser)
// 无 Len()→chunked→S3 拒 MissingContentLength。切换罕见、对象 MB 级,缓冲可接受。
func copyObjectKey(ctx context.Context, d storage.Driver, src, dst string) error {
	rc, err := d.Open(ctx, src)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return err
	}
	return d.Put(ctx, dst, bytes.NewReader(data))
}

// copyThumbAcross 把 oldSurface 缩略图复制到 newSurface 同 ext 键。源按候选探测(含 public
// 遗留),命中即复制。仅当所有候选都明确不存在(ErrNotFound)才返 nil 容忍;读/写错误上抛,
// 由调用方中止重挂——防"源存在但复制失败仍提交"致新 surface /t 永久 404。

// copyThumbAcross 把 oldSurface 缩略图复制到 newSurface 同 ext 键。源按候选探测(含 public
// 遗留),命中即复制。仅当所有候选都明确不存在(ErrNotFound)才返 nil 容忍;读/写错误上抛,
// 由调用方中止重挂——防"源存在但复制失败仍提交"致新 surface /t 永久 404。
func (s *Service) copyThumbAcross(ctx context.Context, d storage.Driver, oldSurface, newSurface, hash string) error {
	for _, src := range storagesvc.ThumbKeyCandidates(oldSurface, hash) {
		rc, err := d.Open(ctx, src)
		if errors.Is(err, storage.ErrNotFound) {
			continue // 该候选确实不存在,试下一个
		}
		if err != nil {
			return err // 瞬时/网络错误 → 中止,不当作无缩略图
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		dst := storagesvc.ThumbKey(newSurface, hash)
		if strings.HasSuffix(src, ".webp") {
			dst = storagesvc.ThumbKeyWebP(newSurface, hash)
		}
		return d.Put(ctx, dst, bytes.NewReader(data))
	}
	return nil // 所有候选均不存在 → 无缩略图可复制,容忍
}

// resolveFileForSurface 找到或创建 targetSurface 上与 srcFile 同 hash 的 File。
// 命中:返回既有(调用方负责 ref++)。未命中:复制原图对象与缩略图到 targetSurface 前缀,
// 建 File 行(Surface=target, Path=新键, ref_count=0)返回。
// 非事务:复制走存储 I/O 不占 DB 锁;建行后若外层 ref 事务失败,新 File 为 ref-0 孤儿(可 GC)。

// resolveFileForSurface 找到或创建 targetSurface 上与 srcFile 同 hash 的 File。
// 命中:返回既有(调用方负责 ref++)。未命中:复制原图对象与缩略图到 targetSurface 前缀,
// 建 File 行(Surface=target, Path=新键, ref_count=0)返回。
// 非事务:复制走存储 I/O 不占 DB 锁;建行后若外层 ref 事务失败,新 File 为 ref-0 孤儿(可 GC)。
func (s *Service) resolveFileForSurface(policy *model.StoragePolicy, srcFile *model.File, targetSurface string) (*model.File, error) {
	var existing model.File
	err := s.db.First(&existing, "hash = ? AND surface = ?", srcFile.Hash, targetSurface).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	d, err := s.res.Driver(policy)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	ext := strings.TrimPrefix(filepath.Ext(srcFile.Path), ".")
	relPath, err := s.res.RenderPath(policy.PathTemplate, ext, time.Now())
	if err != nil {
		return nil, err
	}
	newPath := storagesvc.SurfacePrefix(targetSurface) + relPath
	if err := copyObjectKey(ctx, d, srcFile.Path, newPath); err != nil {
		return nil, err
	}
	if err := s.copyThumbAcross(ctx, d, srcFile.Surface, targetSurface, srcFile.Hash); err != nil {
		_ = d.Delete(ctx, newPath) // 补偿:缩略图失败中止,删掉已复制的原图对象,防孤儿泄漏
		return nil, err
	}
	newFile := &model.File{
		Hash: srcFile.Hash, Surface: targetSurface, StoragePolicyID: srcFile.StoragePolicyID,
		Path: newPath, Size: srcFile.Size, MIME: srcFile.MIME, Width: srcFile.Width, Height: srcFile.Height,
		RefCount: 0,
	}
	if err := s.db.Create(newFile).Error; err != nil {
		// 并发未命中同 (hash,targetSurface):唯一键冲突 → 回查胜者返回(不依赖方言是否把
		// 约束错译成 gorm.ErrDuplicatedKey)。胜者已建时补偿删除我方已复制的 newPath 对象,防孤儿泄漏。
		var winner model.File
		if e := s.db.First(&winner, "hash = ? AND surface = ?", srcFile.Hash, targetSurface).Error; e == nil {
			_ = d.Delete(ctx, newPath) // 补偿:胜者已建,删我方孤儿原图对象
			return &winner, nil
		}
		// 非并发原因建行失败(DB/FK 等)且无胜者:已复制的 newPath 原图无 File 行、
		// 无删除任务,ref-0 清理找不到 → 补偿删除防孤儿(缩略图键确定性可能被并发共享,不删)。
		_ = d.Delete(ctx, newPath)
		return nil, err
	}
	return newFile, nil
}
