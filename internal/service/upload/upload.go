// Package upload 实现上传管线：校验→去重秒传→落盘→建记录→缩略图。
package upload

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/settings"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/task"
)

const (
	ThumbMaxEdge = 400
	MaxDimension = 30000 // 单边像素上限，防解压炸弹
)

var (
	ErrExtNotAllowed     = errors.New("upload: 文件类型不被允许")
	ErrFileTooLarge      = errors.New("upload: 文件超过大小上限")
	ErrQuotaExceeded     = errors.New("upload: 存储配额不足")
	ErrBandwidthExceeded = errors.New("upload: 本月流量已用尽")
	ErrDimensionOver     = errors.New("upload: 图片尺寸过大")
	ErrInvalidImage      = errors.New("upload: 不是有效图片")
	ErrGuestNotSupported = errors.New("upload: 游客上传暂未开放")
	ErrPolicyNotAllowed  = errors.New("upload: 存储策略不可用")
	ErrAlbumNotFound     = errors.New("upload: 相册不存在")
	ErrHeicUnavailable   = errors.New("upload: 当前构建无法解码 HEIC")
)

// Opts 上传选项。零值=完全走用户偏好与组默认。
// AlbumID 三态:nil=未指定(回退偏好);指向 0=明确不归档;指向 N=指定相册。
// ExpiresAt 非 nil 时写入 image.expires_at（由 handler 据 expires_in 用后端 time.Now() 算权威时间）。
// MaxViews 0=不限；>0 时 /i 对非属主限次（阅后即焚用 1）。
// AccessPasswordHash 已哈希的访问口令；空=无口令（handler 负责 argon2）。
type Opts struct {
	Visibility         string
	AlbumID            *uint64
	PolicyID           uint64 // 0=未指定(回退偏好→组默认)
	ExpiresAt          *time.Time
	MaxViews           int
	AccessPasswordHash string
}

type Result struct {
	Image   *model.Image
	File    *model.File
	Policy  *model.StoragePolicy
	Instant bool
	// Reused 为 true 表示同用户命中已有 live image（同 file + 选项一致），未新建记录、未二次扣配额。
	// 跨用户秒传仍 Instant=true 且 Reused=false。
	Reused bool
}

type Service struct {
	db   *gorm.DB
	res  *storagesvc.Resolver
	proc imaging.Processor
	run  *task.Runner
	st   *settings.Service // 与 New 同生命周期，避免每次 GuestEnabled/burn 新建

	// WatermarkDir 用户水印图目录 <data_dir>/watermarks(D-② 烧录管线;空=用户水印不可用)。
	WatermarkDir string
}

func New(db *gorm.DB, res *storagesvc.Resolver, proc imaging.Processor, run *task.Runner) *Service {
	return &Service{db: db, res: res, proc: proc, run: run, st: settings.New(db)}
}
