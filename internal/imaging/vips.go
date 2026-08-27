//go:build vips

package imaging

/*
#cgo pkg-config: vips
#include <vips/vips.h>
#include <stdlib.h>

// thumb_webp:vips_thumbnail_buffer(shrink-on-load)+白底 flatten+webpsave Q=80。
// 变参 C API 在 cgo 中不可直接调,经此静态辅助包一层。
// 成功返回 0,输出 *out 由 g_malloc 分配,调用方 g_free;*out_len 字节数。
// 失败返回 -1(调用方读 vips_error_buffer)。
static int thumb_webp(void *in, size_t in_len, int max_edge, void **out, size_t *out_len) {
	VipsImage *thumb = NULL;
	VipsImage *flat = NULL;
	VipsImage *src = NULL;
	VipsArrayDouble *bg = NULL;
	int ret = -1;

	if (max_edge < 1) {
		max_edge = 1;
	}
	if (vips_thumbnail_buffer(in, in_len, &thumb, max_edge,
			"height", max_edge,
			"size", VIPS_SIZE_DOWN,
			NULL)) {
		return -1;
	}
	src = thumb;
	if (vips_image_hasalpha(thumb)) {
		// 单值背景 libvips 会广播到全部通道:同时兼容灰度+alpha(去 alpha 后 1 通道)
		// 与 RGB+alpha(3 通道),白底不因通道数不匹配而 flatten 失败(codex 终审)。
		bg = vips_array_double_newv(1, 255.0);
		if (vips_flatten(thumb, &flat, "background", bg, NULL)) {
			goto done;
		}
		g_object_unref(thumb);
		thumb = NULL;
		src = flat;
	}
	if (vips_webpsave_buffer(src, out, out_len, "Q", 80, NULL)) {
		goto done;
	}
	ret = 0;
done:
	if (bg) {
		vips_area_unref(VIPS_AREA(bg));
	}
	if (src) {
		g_object_unref(src);
	} else if (thumb) {
		g_object_unref(thumb);
	}
	return ret;
}
*/
import "C"

import (
	"io"
	"sync"
	"unsafe"
)

type vipsProcessor struct{}

var (
	vipsOnce    sync.Once
	vipsInitErr error
)

func ensureVips() error {
	vipsOnce.Do(func() {
		name := C.CString("imgli")
		if C.vips_init(name) != 0 {
			vipsInitErr = ErrUnsupported
		}
		C.free(unsafe.Pointer(name))
		if vipsInitErr != nil {
			return
		}
		// 低默认并发 + 有界缓存，减轻 Docker/ARM 上大图转码 OOM。
		C.vips_concurrency_set(C.int(VipsConcurrency()))
		C.vips_cache_set_max(64)
		C.vips_cache_set_max_mem(64 * 1024 * 1024) // 64MiB
	})
	return vipsInitErr
}

// NewVips 返回 libvips Processor。进程级 vips_init 经 sync.Once。
// Probe 对 HEIF 走 vips（见 heif_vips.go）；其它格式沿用纯 Go。Thumbnail 用
// vips_thumbnail_buffer(shrink-on-load)+ vips_webpsave_buffer(Q=80)。
func NewVips() Processor { return vipsProcessor{} }

func (vipsProcessor) ThumbExt() string { return "webp" }

func (vipsProcessor) Thumbnail(r io.Reader, maxEdge int) ([]byte, error) {
	if err := ensureVips(); err != nil {
		return nil, err
	}
	data, err := readThumbSource(r)
	if err != nil {
		return nil, err
	}
	// vips 有 shrink-on-load，像素上限可放宽，但仍拒绝明显解压炸弹与超大文件。
	if err := checkDecodeBudget(data); err != nil {
		return nil, err
	}
	if maxEdge < 1 {
		maxEdge = 1
	}
	release := acquireThumbSlot()
	defer release()

	var out unsafe.Pointer
	var outLen C.size_t
	// C.CBytes 分配 C 堆副本,thumbnail 内部拷贝/映射后可立刻 free。
	in := C.CBytes(data)
	defer C.free(in)

	if C.thumb_webp(in, C.size_t(len(data)), C.int(maxEdge), &out, &outLen) != 0 {
		C.vips_error_clear()
		return nil, ErrUnsupported
	}
	if out == nil || outLen == 0 {
		return nil, ErrUnsupported
	}
	buf := C.GoBytes(out, C.int(outLen))
	C.g_free(C.gpointer(out))
	return buf, nil
}
