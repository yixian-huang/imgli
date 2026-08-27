// Package imaging 图像探测与缩略图。纯 Go 实现（零 cgo）；
// Phase 3 引 libvips 时以同一 Processor 接口经 build tag 替换。
package imaging

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"os"
	"strconv"
	"sync"

	xdraw "golang.org/x/image/draw"

	_ "image/gif" // 注册解码器；GIF 经 image.Decode 得首帧
	_ "image/png"

	_ "golang.org/x/image/webp" // 仅解码——纯 Go 无成熟有损 WebP 编码器
)

// ErrUnsupported is returned when image format is not recognized.
var ErrUnsupported = errors.New("imaging: unsupported image format")

// ErrTooLarge 源图像素或压缩体积超过缩略图安全上限（防 pure-Go 全图解码 OOM）。
// 调用方应跳过缩略图或回退占位，勿当致命上传错误。
var ErrTooLarge = errors.New("imaging: image too large for safe decode")

// 缩略图解码安全上限（可被环境变量覆盖，见 init）。
// 16MP ≈ RGBA 64MiB 峰值量级；再叠并发会顶穿小机 MemoryHigh。
// 压缩体 24MiB：生产库内有 17–25MiB 原图，仍允许常见截图，拒绝巨型 GIF/PNG 炸弹。
var (
	MaxThumbSourceBytes = 24 << 20 // 24 MiB
	MaxDecodePixels     = 16_000_000
)

// thumbSlots 限制并发全图解码/缩放，避免管理端列表批量 /t 击穿内存。
var thumbSlots chan struct{}

func init() {
	if v := os.Getenv("IMGLI_THUMB_MAX_PIXELS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1_000_000 && n <= 100_000_000 {
			MaxDecodePixels = n
		}
	}
	if v := os.Getenv("IMGLI_THUMB_MAX_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1<<20 && n <= 256<<20 {
			MaxThumbSourceBytes = n
		}
	}
	n := 1
	if v := os.Getenv("IMGLI_THUMB_CONCURRENCY"); v != "" {
		if c, err := strconv.Atoi(v); err == nil && c >= 1 && c <= 8 {
			n = c
		}
	}
	thumbSlots = make(chan struct{}, n)
}

// acquireThumbSlot 占用一个缩略图解码名额；释放函数必须调用。
func acquireThumbSlot() (release func()) {
	thumbSlots <- struct{}{}
	var once sync.Once
	return func() { once.Do(func() { <-thumbSlots }) }
}

// readThumbSource 读取缩略图源并强制体积上限。
func readThumbSource(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(MaxThumbSourceBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrUnsupported
	}
	if len(data) > MaxThumbSourceBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}

// checkDecodeBudget 在全量 Decode 前用头信息拒绝超大像素（防解压炸弹）。
func checkDecodeBudget(data []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ErrUnsupported
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ErrUnsupported
	}
	// int64 防 width*height 溢出
	if int64(cfg.Width)*int64(cfg.Height) > int64(MaxDecodePixels) {
		return ErrTooLarge
	}
	return nil
}

// Meta contains image metadata: dimensions, MIME type, and file extension.
type Meta struct {
	Width, Height int
	MIME, Ext     string
}

// Processor detects image metadata and generates JPEG thumbnails.
// Both Probe and Thumbnail consume their input readers (partially or fully);
// callers needing multiple passes must supply independent readers, e.g. buffer once then bytes.NewReader per call.
type Processor interface {
	// Probe reads only the image header bytes and returns metadata.
	// After Probe returns, r is at an indeterminate offset and must not be reused.
	// 仅读取头部字节即返回；调用后 r 处于不确定偏移，不可复用。
	Probe(r io.Reader) (Meta, error)

	// Thumbnail fully consumes r, returning a JPEG-encoded thumbnail.
	// 完整消耗 r。
	Thumbnail(r io.Reader, maxEdge int) ([]byte, error)

	// ThumbExt 本处理器缩略图输出格式的扩展名(纯 Go "jpg"/vips "webp")——
	// 上传落盘键与 /t 双探测按此对齐(D-②)。
	ThumbExt() string
}

type goProcessor struct{}

// NewGo returns a pure-Go image Processor using the standard library.
func NewGo() Processor { return goProcessor{} }

var formatMeta = map[string]struct{ mime, ext string }{
	"jpeg": {"image/jpeg", "jpg"},
	"png":  {"image/png", "png"},
	"gif":  {"image/gif", "gif"},
	"webp": {"image/webp", "webp"},
}

func (goProcessor) ThumbExt() string { return "jpg" }

func (goProcessor) Probe(r io.Reader) (Meta, error) {
	prefix, rest, err := readProbePrefix(r)
	if err != nil {
		return Meta{}, ErrUnsupported
	}
	if SniffHEIF(prefix) {
		return Meta{}, ErrHeicUnavailable
	}
	cfg, format, err := image.DecodeConfig(rest)
	if err != nil {
		return Meta{}, ErrUnsupported
	}
	fm, ok := formatMeta[format]
	if !ok {
		return Meta{}, ErrUnsupported
	}
	return Meta{Width: cfg.Width, Height: cfg.Height, MIME: fm.mime, Ext: fm.ext}, nil
}

func (goProcessor) Thumbnail(r io.Reader, maxEdge int) ([]byte, error) {
	data, err := readThumbSource(r)
	if err != nil {
		return nil, err
	}
	if err := checkDecodeBudget(data); err != nil {
		return nil, err
	}
	release := acquireThumbSlot()
	defer release()

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	if w > maxEdge || h > maxEdge { // 只缩不放
		if w >= h {
			h = h * maxEdge / w
			w = maxEdge
		} else {
			w = w * maxEdge / h
			h = maxEdge
		}
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
	}
	// 白底画布：平铺 alpha（JPEG 无透明通道）
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, xdraw.Over, nil)
	src = nil // 尽早丢大图引用，助 GC

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Avatar 中心方裁并缩放到 edge×edge 的 JPEG 头像(白底平铺 alpha)。
// 非图片或单边超过 8192(头像场景防解压炸弹,阈值远低于上传管线)返回 ErrUnsupported。
func Avatar(data []byte, edge int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width > 8192 || cfg.Height > 8192 {
		return nil, ErrUnsupported
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}
	sb := src.Bounds()
	side := sb.Dx()
	if sb.Dy() < side {
		side = sb.Dy()
	}
	x0 := sb.Min.X + (sb.Dx()-side)/2
	y0 := sb.Min.Y + (sb.Dy()-side)/2
	crop := image.Rect(x0, y0, x0+side, y0+side)

	dst := image.NewRGBA(image.Rect(0, 0, edge, edge))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, xdraw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
