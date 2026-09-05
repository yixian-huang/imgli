// Package model 定义全部 GORM 模型与数据库生命周期（Open/Migrate/Seed）。
// 列名/表名是全项目契约，计划②③直接引用，不得随意改动。
package model

import (
	"time"

	"gorm.io/gorm"
)

// WatermarkPref 用户图片水印偏好(preferences.watermark)。
type WatermarkPref struct {
	Enabled  bool    `json:"enabled"`
	Position string  `json:"position"` // 九宫格;空=缺省 br(读取侧规整)
	Opacity  float64 `json:"opacity"`  // [0,1];0=缺省 0.5(读取侧规整)
	Margin   int     `json:"margin"`   // px,[0,256]
}

// Preferences 用户上传偏好（users.preferences JSON 列）。
type Preferences struct {
	DefaultAlbumID    *uint64       `json:"default_album_id"`
	DefaultVisibility string        `json:"default_visibility"` // "" | public | private
	DefaultPolicyID   *uint64       `json:"default_policy_id"`
	AutoCopyFormat    string        `json:"auto_copy_format"` // "" | url | markdown | html | bbcode | share
	Watermark         WatermarkPref `json:"watermark"`
	Lang              string        `json:"lang"` // "" | zh | en；"" = 跟随前端 detect
}

type User struct {
	ID              uint64 `gorm:"primaryKey"`
	Username        string `gorm:"uniqueIndex;size:32"`
	Email           string `gorm:"uniqueIndex;size:255"`
	PasswordHash    string `gorm:"size:255"`
	Nickname        string `gorm:"size:64"`
	AvatarPath      string `gorm:"size:255"`
	WatermarkPath   string `gorm:"size:255"`
	GroupID         uint64 `gorm:"index"`
	IsAdmin         bool
	Status          string `gorm:"size:16;default:active"` // active | banned
	EmailVerifiedAt *time.Time
	UsedStorage     int64 // 反规范化配额计数器，与图片增删同事务原子更新
	// BandwidthUsedMonth / BandwidthPeriod：自然月出站用量（字节）与账期 "2006-01"（Asia/Shanghai）。
	// 账期切换时在累加路径重置 used，不删图。
	BandwidthUsedMonth int64  `gorm:"not null;default:0"`
	BandwidthPeriod    string `gorm:"size:7;not null;default:''"` // YYYY-MM；空=尚未计量
	// Signup*：注册时刻轻量归因（无持续追踪）。Channel: direct|invite|utm|referer|unknown
	SignupChannel      string      `gorm:"size:16;not null;default:''"`
	SignupUTMSource    string      `gorm:"size:64;not null;default:''"`
	SignupUTMMedium    string      `gorm:"size:64;not null;default:''"`
	SignupUTMCampaign  string      `gorm:"size:64;not null;default:''"`
	SignupRefererHost  string      `gorm:"size:255;not null;default:''"` // host only
	SignupInviteCodeID *uint64     `gorm:"index"`
	Preferences        Preferences `gorm:"serializer:json"`
	PublicProfile      bool
	CreatedAt          time.Time
	UpdatedAt          time.Time

	Group *UserGroup `gorm:"foreignKey:GroupID;constraint:OnDelete:RESTRICT" json:"-"`
}

type UserGroup struct {
	ID           uint64 `gorm:"primaryKey"`
	Name         string `gorm:"size:64"`
	IsDefault    bool
	IsGuest      bool
	StorageQuota int64 // bytes
	MaxFileSize  int64 // bytes
	// BandwidthQuotaMonth 本月出站硬顶（字节）；0=不限制。
	BandwidthQuotaMonth int64 `gorm:"not null;default:0"`
	RatePerMinute       int
	RatePerHour         int
	RatePerDay          int
	AllowedExts         []string `gorm:"serializer:json"`
	AllowedPolicyIDs    []uint64 `gorm:"serializer:json"`
	// DefaultExpiresIn 上传默认有效期（秒）；0=默认永久。UI/未传时由后端套用。
	DefaultExpiresIn int `gorm:"not null;default:0"`
	// MaxExpiresIn 有效期上限（秒）；0=允许永久（仍受全局 1 年上限）；>0 时禁止永久且不得超过此值。
	MaxExpiresIn int `gorm:"not null;default:0"`
	// DefaultMaxViews 默认访问次数；0=默认不限。
	DefaultMaxViews int `gorm:"not null;default:0"`
	// MaxMaxViews 访问次数上限；0=允许不限（仍受全局上限）；>0 时禁止不限且不得超过此值。
	MaxMaxViews int `gorm:"not null;default:0"`
	// RetentionDays 自动进回收站天数；0=关闭。按 created_at 对 live 图软删。
	RetentionDays int `gorm:"not null;default:0"`
	// ForceMaxAgeDays 强制最大存活天数；0=关闭。上传钳制/补默认；定时对超龄 live 图永久清理。
	ForceMaxAgeDays int `gorm:"not null;default:0"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// File 物理文件（内容寻址）。去重按 (hash,surface) 唯一：hash 命中即秒传。
type File struct {
	ID              uint64 `gorm:"primaryKey"`
	Hash            string `gorm:"size:64;uniqueIndex:idx_files_hash_surface,priority:1"`               // SHA-256 hex；(hash,surface) 唯一
	Surface         string `gorm:"size:8;default:public;uniqueIndex:idx_files_hash_surface,priority:2"` // public|private，决定对象键前缀与匿名可读性
	StoragePolicyID uint64 `gorm:"index"`
	Path            string `gorm:"size:512"` // 存储键（含 surface 前缀，如 public/2026/...）
	Size            int64
	MIME            string `gorm:"size:64"`
	Width           int
	Height          int
	RefCount        int // 引用计数，归零后由任务删除物理文件
	CreatedAt       time.Time

	Policy *StoragePolicy `gorm:"foreignKey:StoragePolicyID;constraint:OnDelete:RESTRICT" json:"-"`
}

// Surface 值：决定对象键前缀与匿名可读性。public→public/ 前缀（匿名可读，i.img.li 直服）；
// private→private/ 前缀（匿名不可读，仅签名 URL）。取值与 Image.Visibility 对齐。
const (
	SurfacePublic  = "public"
	SurfacePrivate = "private"
)

// Image 用户视角的图片记录。软删（DeletedAt）即回收站。
type Image struct {
	ID      uint64  `gorm:"primaryKey"`
	Key     string  `gorm:"uniqueIndex;size:16"` // 12 位 base62，直链用
	Slug    *string `gorm:"uniqueIndex;size:32"` // 可选 vanity 别名 [a-z0-9-]{3,32}
	UserID  *uint64 `gorm:"index"`               // 游客上传为 nil
	FileID  uint64  `gorm:"index"`
	AlbumID *uint64 `gorm:"index"`
	// AlbumPos 相册内排序：>0 时升序优先；0 表示未手动排序（公开/属主列表回落到 id/时间）。
	AlbumPos      int    `gorm:"not null;default:0;index"`
	Name          string `gorm:"size:255"`
	Ext           string `gorm:"size:8"`
	Visibility    string `gorm:"size:8;default:public"`  // public | private
	Status        string `gorm:"size:16;default:normal"` // normal | pending | rejected
	IsWhitelisted bool
	NSFWScore     *float64
	UploadIP      string     `gorm:"size:64"`
	ExpiresAt     *time.Time `gorm:"index"`
	// MaxViews 0=不限；>0 时成功 /i 对非属主计数，达上限后非属主不可再访。
	// ViewsServed 已消耗次数（仅 max_views>0 时递增）。
	MaxViews    int `gorm:"not null;default:0"`
	ViewsServed int `gorm:"not null;default:0"`
	// AccessPasswordHash 非空时非属主须口令解锁（argon2id PHC）；空=无口令。
	// 永不经 API 返回明文/哈希；DTO 用 has_access_password。
	AccessPasswordHash string `gorm:"size:255"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`

	User  *User  `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"-"`
	File  *File  `gorm:"foreignKey:FileID;constraint:OnDelete:RESTRICT" json:"-"`
	Album *Album `gorm:"foreignKey:AlbumID;constraint:OnDelete:SET NULL" json:"-"`
}

type Album struct {
	ID         uint64 `gorm:"primaryKey"`
	UserID     uint64 `gorm:"index"`
	Name       string `gorm:"size:128"`
	Visibility string `gorm:"size:8;default:private"`
	// DefaultView 公开访客页默认模式：gallery | immersive（空=gallery）。
	DefaultView string `gorm:"size:16;not null;default:gallery"`
	// ClickToImmersive 画廊瀑布流点击单张是否进入沉浸（默认 true）；false 时点图进分享页。
	ClickToImmersive bool `gorm:"not null;default:true"`
	// Description 可选说明（公开页展示）。
	Description string `gorm:"size:2000;not null;default:''"`
	// CoverKey 手动封面图 key；空=自动取最新可展示图。
	CoverKey string `gorm:"size:16;not null;default:''"`
	// AccessPasswordHash 非空时访客须口令才能看 /a 列表（不挡图直链）；空=无口令。
	AccessPasswordHash string `gorm:"size:255;not null;default:''"`
	// ListInPlaza 相册内 public 图是否参与广场（默认 true）；无相册的图不受影响。
	ListInPlaza bool `gorm:"not null;default:true"`
	CreatedAt   time.Time
	UpdatedAt   time.Time

	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"-"`
}

// AlbumAccessStat 相册页 PV 日聚合（仅公开访客）。
type AlbumAccessStat struct {
	ID      uint64 `gorm:"primaryKey"`
	AlbumID uint64 `gorm:"uniqueIndex:idx_album_access,priority:1"`
	Date    string `gorm:"size:10;uniqueIndex:idx_album_access,priority:2"` // 2006-01-02
	Views   int64
}

type APIToken struct {
	ID         uint64 `gorm:"primaryKey"`
	UserID     uint64 `gorm:"index"`
	Name       string `gorm:"size:64"`
	TokenHash  string `gorm:"uniqueIndex;size:64"` // SHA-256(明文)，明文仅创建时返回一次
	Scope      string `gorm:"size:16"`             // upload | full
	LastUsedAt *time.Time
	CreatedAt  time.Time

	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

// Session Web 会话，DB 存储保证多实例无状态。主键即 token 的 SHA-256。
type Session struct {
	ID        string    `gorm:"primaryKey;size:64"`
	UserID    uint64    `gorm:"index"`
	ExpiresAt time.Time `gorm:"index"`
	IP        string    `gorm:"size:64"`
	UA        string    `gorm:"size:255"`
	CreatedAt time.Time

	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

// AuthToken 邮箱验证/密码重置/改邮箱一次性令牌。
type AuthToken struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    uint64 `gorm:"index"`
	Purpose   string `gorm:"size:24"` // verify_email | reset_password | change_email
	TokenHash string `gorm:"uniqueIndex;size:64"`
	Payload   string `gorm:"size:255"` // 如新邮箱地址
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time

	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

type InviteCode struct {
	ID        uint64 `gorm:"primaryKey"`
	Code      string `gorm:"uniqueIndex;size:16"` // IL-XXXX-XXXX
	CreatedBy uint64
	UsedBy    *uint64
	UsedAt    *time.Time
	ExpiresAt *time.Time
	CreatedAt time.Time
}

type StoragePolicy struct {
	ID           uint64            `gorm:"primaryKey"`
	Name         string            `gorm:"size:64"`
	Driver       string            `gorm:"size:16"` // local | s3 | oss | cos | webdav
	Config       map[string]string `gorm:"serializer:json"`
	CDNDomain    string            `gorm:"size:255"`
	PathTemplate string            `gorm:"size:128;default:{Y}/{m}/{d}/{uniqid}.{ext}"`
	Enabled      bool              `gorm:"default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Setting KV 业务设置。Value 为 JSON 文本（含标量的 JSON 编码，如 `"open"`）。
type Setting struct {
	Key       string `gorm:"primaryKey;size:64"`
	Value     string
	UpdatedAt time.Time
}

// Task 异步任务。认领：Postgres 用 FOR UPDATE SKIP LOCKED，SQLite 单写者普通 UPDATE。
type Task struct {
	ID        uint64    `gorm:"primaryKey"`
	Type      string    `gorm:"size:32;index"`
	Payload   string    // JSON
	Status    string    `gorm:"size:16;index;default:pending"` // pending | running | done | failed
	RunAt     time.Time `gorm:"index"`
	Attempts  int
	LastError string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AuditLog struct {
	ID        uint64  `gorm:"primaryKey"`
	ActorID   *uint64 `gorm:"index"`
	ActorType string  `gorm:"size:16"` // user | admin | system
	Action    string  `gorm:"size:64;index"`
	Detail    string  // JSON
	IP        string  `gorm:"size:64"`
	CreatedAt time.Time
}

// SchemaVersion 破坏性迁移的版本记录（AutoMigrate 管增量）。
type SchemaVersion struct {
	Version   int `gorm:"primaryKey"`
	AppliedAt time.Time
}

// StorageMigrateJob 跨策略搬迁任务（落库；重启可续）。
type StorageMigrateJob struct {
	ID            string `gorm:"primaryKey;size:32"`
	FromPolicyID  uint64 `gorm:"index"`
	ToPolicyID    uint64
	DryRun        bool
	DeleteSource  bool
	Limit         int
	UserID        *uint64
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Status        string `gorm:"size:16;index"` // pending|running|done|failed|cancelled
	CursorAfterID uint64
	Scanned       int
	Copied        int
	Skipped       int
	Failed        int
	SamplePaths   []string `gorm:"serializer:json"`
	Errors        []string `gorm:"serializer:json"`
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// D-① 访问统计:(image_id|host, date) 唯一联合索引,upsert 累加
type AccessStat struct {
	ID      uint64 `gorm:"primaryKey"`
	ImageID uint64 `gorm:"uniqueIndex:idx_access,priority:1"`
	Date    string `gorm:"size:10;uniqueIndex:idx_access,priority:2"` // 2006-01-02 服务器本地时区
	Views   int64
}

type RefererStat struct {
	ID    uint64 `gorm:"primaryKey"`
	Host  string `gorm:"size:255;uniqueIndex:idx_ref,priority:1"` // 去端口小写;空 Referer 归一 "(direct)"
	Date  string `gorm:"size:10;uniqueIndex:idx_ref,priority:2"`
	Count int64
}

// RefererImageStat host×image×day 聚合，供 admin「某 host 打了哪些图」。
type RefererImageStat struct {
	ID      uint64 `gorm:"primaryKey"`
	ImageID uint64 `gorm:"uniqueIndex:idx_refimg,priority:1"`
	Host    string `gorm:"size:255;uniqueIndex:idx_refimg,priority:2"`
	Date    string `gorm:"size:10;uniqueIndex:idx_refimg,priority:3"`
	Count   int64
}

// AllModels Migrate 的唯一来源。
func AllModels() []any {
	return []any{
		&User{}, &UserGroup{}, &File{}, &Image{}, &Album{},
		&APIToken{}, &Session{}, &AuthToken{}, &InviteCode{},
		&StoragePolicy{}, &Setting{}, &Task{}, &AuditLog{}, &SchemaVersion{},
		&StorageMigrateJob{},
		&AccessStat{}, &RefererStat{}, &RefererImageStat{}, &AlbumAccessStat{},
	}
}
