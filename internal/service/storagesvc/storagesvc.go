// Package storagesvc 按存储策略解析出 storage.Driver 并渲染存储路径。
package storagesvc

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/storage"
	"github.com/yixian-huang/imgli/internal/storage/ftp"
	"github.com/yixian-huang/imgli/internal/storage/local"
	"github.com/yixian-huang/imgli/internal/storage/s3"
	"github.com/yixian-huang/imgli/internal/storage/webdav"
)

type cachedDriver struct {
	d  storage.Driver
	fp string // 策略 driver+config 指纹;变了即缓存失效(codex 终审:改凭据/桶后不再用旧驱动)
}

type Resolver struct {
	cfg   *config.Config
	db    *gorm.DB
	mu    sync.Mutex
	cache map[uint64]cachedDriver // policyID → 缓存驱动+指纹

	// migrateMu / migrateActive：跨策略搬迁进程内互斥（按 from policy id）。
	// 与 cache 锁分离，避免 Driver 解析与搬迁长时间持锁交叉死锁。
	migrateMu     sync.Mutex
	migrateActive map[uint64]struct{}

	// jobsMu / jobs：Admin 异步搬迁任务（进程内；重启即失，重跑幂等）。
	jobsMu sync.Mutex
	jobs   map[string]*MigrateJob
}

func New(cfg *config.Config, db *gorm.DB) *Resolver {
	return &Resolver{
		cfg:           cfg,
		db:            db,
		cache:         map[uint64]cachedDriver{},
		migrateActive: map[uint64]struct{}{},
		jobs:          map[string]*MigrateJob{},
	}
}

// policyFP 计算策略 driver+config 的稳定指纹:任一字段变化即不同,用于缓存失效。
func policyFP(p *model.StoragePolicy) string {
	keys := make([]string, 0, len(p.Config))
	for k := range p.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(p.Driver)
	b.WriteByte('|')
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(p.Config[k])
		b.WriteByte(';')
	}
	return b.String()
}

// Driver 返回策略对应驱动（local/s3/webdav/ftp）。按 (policyID, 配置指纹) 缓存——策略配置更新后
// 指纹变化,自动重建驱动,不会无限沿用旧 endpoint/凭据（codex 终审）。
func (r *Resolver) Driver(p *model.StoragePolicy) (storage.Driver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fp := policyFP(p)
	if c, ok := r.cache[p.ID]; ok && c.fp == fp {
		return c.d, nil
	}
	var d storage.Driver
	var err error
	switch p.Driver {
	case "local":
		d, err = local.New(storage.LocalRoot(r.cfg.DataDir, p.Config["root"]))
	case "s3":
		d, err = s3.New(p.Config)
	case "webdav":
		d, err = webdav.New(p.Config)
	case "ftp":
		d, err = ftp.New(p.Config)
	default:
		return nil, fmt.Errorf("storagesvc: 暂不支持的驱动 %q", p.Driver)
	}
	if err != nil {
		return nil, err
	}
	r.cache[p.ID] = cachedDriver{d: d, fp: fp}
	return d, nil
}

// CapsFor returns static driver capabilities for a policy.
func (r *Resolver) CapsFor(p *model.StoragePolicy) (storage.Caps, error) {
	if d, err := r.Driver(p); err == nil {
		if c, ok := d.(storage.CapabilityProvider); ok {
			return c.Capabilities(), nil
		}
	}
	return storage.CapsForDriver(p.Driver)
}

// EffectiveFor returns policy-level effective storage capabilities.
func (r *Resolver) EffectiveFor(p *model.StoragePolicy) (storage.Effective, error) {
	return storage.EffectiveFor(p.Driver, p.Config, p.CDNDomain)
}

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// RenderPath 见 path_template.go（时间 + 随机占位符；不含 surface/prefix）。

// CDNEligibleObjectKey 判断对象键是否允许走「未鉴权 CDN/公网 302」(S4 纵深)。
// fail-closed：显式 private/ 前缀（及 private 下缩略图）一律禁止；
// 其余（public/ 或 S1 前遗留无前缀路径）允许由调用方配合 visibility 再拦一层。
func CDNEligibleObjectKey(key string) bool {
	k := strings.TrimLeft(key, "/")
	if k == "" {
		return false
	}
	// SurfacePrefix(private) == "private/"
	if strings.HasPrefix(k, SurfacePrefix(model.SurfacePrivate)) {
		return false
	}
	return true
}

// ObjectURL 返回公开图 302 回源目标:CDNDomain(对象存储 CDN 前缀)+ 编码后的
// prefix+key。CDNDomain 空返空串(不可 302,调用方走流式)。
// S4：private surface 键永不拼 CDN URL（即使调用方误传），防桶匿名可读时 302 泄密。
// 对象键含特殊字符(空格/#/?/非ASCII)时按 path segment 编码,保留 / 层级,防 Location
// 路径语义被改致重定向到错地址(codex 终审)。
func (r *Resolver) ObjectURL(p *model.StoragePolicy, key string) string {
	if p.CDNDomain == "" || !CDNEligibleObjectKey(key) {
		return ""
	}
	return strings.TrimRight(p.CDNDomain, "/") + "/" + encodeObjectPath(p.Config["prefix"]+key)
}

// encodeObjectPath 按 / 分段 url.PathEscape,保留层级分隔——CDN/浏览器解码后还原
// 为存储对象键。安全键(字母数字/./-)为无操作。
func encodeObjectPath(objectKey string) string {
	segs := strings.Split(objectKey, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// LinkBase 复制/嵌入链基址,恒 BaseURL(app /i 路由,门禁生效)。CDNDomain 改由
// ObjectURL 用作公开图 302 目标(裁决 5:CDNDomain 语义 = 对象存储 CDN 前缀)。
func (r *Resolver) LinkBase(p *model.StoragePolicy) string {
	return strings.TrimRight(r.cfg.BaseURL, "/")
}

// ThumbGen 缩略图生成世代。算法/最长边/编码策略变更时递增；新图写 g{N} 前缀，
// 读取 dual-probe 旧无版本路径，避免 immutable 长缓存在「同 key 键布局变更」后脏读(C2)。
const ThumbGen = "1"

// NormSurface 读侧规整：空串 ≈ public（与 migrateSurface / 重挂扫描一致）；
// 未知值 fail-closed 为 private，避免误探公开遗留键。
func NormSurface(surface string) string {
	switch surface {
	case "", model.SurfacePublic:
		return model.SurfacePublic
	default:
		return model.SurfacePrivate
	}
}

// ThumbKey 当前世代 JPEG 缩略图键（按 surface 前缀 + 内容寻址）。
func ThumbKey(surface, hash string) string {
	return SurfacePrefix(NormSurface(surface)) + ".thumbs/g" + ThumbGen + "/" + hash + ".jpg"
}

// WidthThumbKey 白名单边长变体 JPEG 键（与默认 thumb 隔离，不污染 content-hash 秒传）。
// 例：public/.thumbs/w400/g1/{hash}.jpg
func WidthThumbKey(surface, hash string, width int) string {
	return SurfacePrefix(NormSurface(surface)) + ".thumbs/w" + strconv.Itoa(width) + "/g" + ThumbGen + "/" + hash + ".jpg"
}

// ThumbKeyWebP 当前世代 WebP 缩略图键(vips 构建)。
func ThumbKeyWebP(surface, hash string) string {
	return SurfacePrefix(NormSurface(surface)) + ".thumbs/g" + ThumbGen + "/" + hash + ".webp"
}

// ThumbKeyCandidates 打开缩略图时的探测顺序：surface 前缀现行世代 webp/jpg。
// public surface 额外探测无前缀遗留路径——S1 之前的公开缩略图落在扁平 .thumbs/,
// 需向后兼容。private surface 不探遗留:私密图在 S1 前不存在,且遗留路径是公开可读
// 位置,私密不应回退到那里(防跨 surface 生命周期耦合)。
func ThumbKeyCandidates(surface, hash string) []string {
	surface = NormSurface(surface)
	c := []string{
		ThumbKeyWebP(surface, hash),
		ThumbKey(surface, hash),
	}
	if surface == model.SurfacePublic {
		c = append(c,
			".thumbs/g"+ThumbGen+"/"+hash+".webp", // 遗留:S1 前扁平现行世代
			".thumbs/g"+ThumbGen+"/"+hash+".jpg",
			".thumbs/"+hash+".webp", // 遗留:pre-ThumbGen
			".thumbs/"+hash+".jpg",
		)
	}
	return c
}

// SurfacePrefix 返回 surface 对应的对象键前缀。public→"public/"（匿名可读,i.img.li 直服）；
// 其余（含 private 与未知值）→"private/"（fail-closed:未知 surface 落非匿名可读位置,绝不误暴露）。
func SurfacePrefix(surface string) string {
	if surface == model.SurfacePublic {
		return "public/"
	}
	return "private/"
}
