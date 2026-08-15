// Package imagesvc 实现用户侧图片管理（列表/详情/变更/回收站/批量）。
package imagesvc

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/task"
)

var (
	ErrInvalidSort       = errors.New("imagesvc: 非法排序")
	ErrBadCursor         = errors.New("imagesvc: 非法游标")
	ErrInvalidFilter     = errors.New("imagesvc: 非法筛选参数")
	ErrNotFound          = errors.New("imagesvc: 图片不存在")
	ErrInvalidVisibility = errors.New("imagesvc: 可见性仅 public|private")
	ErrAlbumNotFound     = errors.New("imagesvc: 相册不存在")
	ErrAlbumForcesPrivate = errors.New("imagesvc: 私密相册内的图必须为 private")
	ErrInvalidName       = errors.New("imagesvc: 名称需 1-255 字节")
	ErrInvalidAction     = errors.New("imagesvc: 未知批量操作")
	ErrInvalidSlug       = errors.New("imagesvc: slug 需 3-32 位小写字母数字与连字符")
	ErrSlugTaken         = errors.New("imagesvc: slug 已被占用")
	slugRe               = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,30}[a-z0-9])?$`)
)

// errRehomeConflict 并发切换已把本图重挂到目标 surface——本次回滚 ref 变更,返回当前态。
var errRehomeConflict = errors.New("imagesvc: 并发重挂冲突")

// Filter 是列表筛选条件。空字段=不筛。
type Filter struct {
	Q, Format, Album, Visibility, Sort string
}

// Row 是一条图片连同其物理文件与存储策略（供构造链接与展示元信息）。
type Row struct {
	Img    model.Image
	File   model.File
	Policy model.StoragePolicy
}

type Service struct {
	db  *gorm.DB
	res *storagesvc.Resolver // 复制对象/取 driver 用（S2 surface 切换）
	run *task.Runner         // 可为 nil（服务级单测无需投递物理删除）
}

func New(db *gorm.DB, res *storagesvc.Resolver, run *task.Runner) *Service {
	return &Service{db: db, res: res, run: run}
}

// formatExts 把 UI 的 format chip 值映射为 ext 集合；空/ALL 返回 nil（不筛）。

// formatExts 把 UI 的 format chip 值映射为 ext 集合；空/ALL 返回 nil（不筛）。
func formatExts(format string) []string {
	switch strings.ToUpper(format) {
	case "", "ALL":
		return nil
	case "JPG", "JPEG":
		return []string{"jpg", "jpeg"}
	case "PNG":
		return []string{"png"}
	case "GIF":
		return []string{"gif"}
	case "WEBP":
		return []string{"webp"}
	default:
		return []string{strings.ToLower(format)}
	}
}

// orderSpec 返回 (排序列, 是否倒序)。未知返回 ok=false。

// orderSpec 返回 (排序列, 是否倒序)。未知返回 ok=false。
func orderSpec(sort string) (col string, desc, ok bool) {
	switch sort {
	case "", "date":
		// 用 images.id（单调自增、与 created_at 同步写入）做 keyset 代理，
		// 避免时间戳精度/类型在跨方言下与游标绑定值比较时失真（见 Finding 1）。
		return "images.id", true, true
	case "size":
		return "files.size", true, true
	case "name":
		return "images.name", false, true
	case "position":
		// 相册内排序：album_pos 升序，同 pos 再按 id 降序
		return "images.album_pos", false, true
	default:
		return "", false, false
	}
}

// applyFilters 加公共 where（不含软删/归属，由调用方先加）。

// applyFilters 加公共 where（不含软删/归属，由调用方先加）。
func applyFilters(q *gorm.DB, f Filter) (*gorm.DB, error) {
	if f.Q != "" {
		q = q.Where("images.name LIKE ?", "%"+f.Q+"%")
	}
	if exts := formatExts(f.Format); exts != nil {
		q = q.Where("images.ext IN ?", exts)
	}
	switch f.Album {
	case "":
		// 不筛
	case "none":
		q = q.Where("images.album_id IS NULL")
	default:
		id, err := strconv.ParseUint(f.Album, 10, 64)
		if err != nil {
			return nil, ErrInvalidFilter
		}
		q = q.Where("images.album_id = ?", id)
	}
	if f.Visibility == "public" || f.Visibility == "private" {
		q = q.Where("images.visibility = ?", f.Visibility)
	}
	return q, nil
}

// listScan 承接 images.* 加派生排序值。
type listScan struct {
	model.Image
	SortSize int64 `gorm:"column:sort_size"`
}
