//go:build vips

package imaging

/*
#cgo pkg-config: vips
#include <vips/vips.h>
#include <stdlib.h>

static int heif_loader_present(void *in, size_t in_len) {
	const char *loader = vips_foreign_find_load_buffer(in, in_len);
	return loader != NULL;
}

static int heif_probe(void *in, size_t in_len, int *w, int *h) {
	VipsImage *im = vips_image_new_from_buffer(in, in_len, "", NULL);
	if (!im) {
		return -1;
	}
	*w = vips_image_get_width(im);
	*h = vips_image_get_height(im);
	g_object_unref(im);
	return 0;
}

static int heif_to_jpeg(void *in, size_t in_len, int q, void **out, size_t *out_len, int *w, int *h) {
	VipsImage *im = NULL;
	VipsImage *rot = NULL;
	int ret = -1;
	if (q < 1) q = 1;
	if (q > 100) q = 100;
	im = vips_image_new_from_buffer(in, in_len, "", NULL);
	if (!im) {
		return -1;
	}
	if (vips_autorot(im, &rot, NULL)) {
		goto done;
	}
	if (vips_jpegsave_buffer(rot, out, out_len, "Q", q, "strip", TRUE, NULL)) {
		goto done;
	}
	*w = vips_image_get_width(rot);
	*h = vips_image_get_height(rot);
	ret = 0;
done:
	if (rot) g_object_unref(rot);
	if (im) g_object_unref(im);
	return ret;
}
*/
import "C"

import (
	"io"
	"sync"
	"unsafe"
)

var (
	heifAvailOnce sync.Once
	heifAvail     bool
)

// HeicDecodeAvailable vips 构建：运行时探测 HEIF loader（无 libheif 则为 false）。
func HeicDecodeAvailable() bool {
	heifAvailOnce.Do(func() {
		if err := ensureVips(); err != nil {
			return
		}
		probe := []byte{
			0, 0, 0, 16,
			'f', 't', 'y', 'p',
			'h', 'e', 'i', 'c',
			0, 0, 0, 0,
		}
		in := C.CBytes(probe)
		defer C.free(in)
		heifAvail = C.heif_loader_present(in, C.size_t(len(probe))) != 0
		if !heifAvail {
			C.vips_error_clear()
		}
	})
	return heifAvail
}

func DecodeHEIFToJPEG(data []byte, jpegQuality int) ([]byte, Meta, error) {
	if len(data) == 0 || !SniffHEIF(data) {
		return nil, Meta{}, ErrUnsupported
	}
	if !HeicDecodeAvailable() {
		return nil, Meta{}, ErrHeicUnavailable
	}
	if err := ensureVips(); err != nil {
		return nil, Meta{}, ErrHeicUnavailable
	}
	q := clampJPEGQuality(jpegQuality)
	in := C.CBytes(data)
	defer C.free(in)
	var out unsafe.Pointer
	var outLen C.size_t
	var w, h C.int
	if C.heif_to_jpeg(in, C.size_t(len(data)), C.int(q), &out, &outLen, &w, &h) != 0 {
		C.vips_error_clear()
		return nil, Meta{}, ErrUnsupported
	}
	if out == nil || outLen == 0 {
		return nil, Meta{}, ErrUnsupported
	}
	buf := C.GoBytes(out, C.int(outLen))
	C.g_free(C.gpointer(out))
	return buf, Meta{Width: int(w), Height: int(h), MIME: "image/jpeg", Ext: "jpg"}, nil
}

func probeHEIF(data []byte) (Meta, error) {
	if !HeicDecodeAvailable() {
		return Meta{}, ErrHeicUnavailable
	}
	if err := ensureVips(); err != nil {
		return Meta{}, ErrHeicUnavailable
	}
	in := C.CBytes(data)
	defer C.free(in)
	var w, h C.int
	if C.heif_probe(in, C.size_t(len(data)), &w, &h) != 0 {
		C.vips_error_clear()
		return Meta{}, ErrUnsupported
	}
	if int(w) < 1 || int(h) < 1 {
		return Meta{}, ErrUnsupported
	}
	return Meta{Width: int(w), Height: int(h), MIME: "image/heic", Ext: "heic"}, nil
}

func (vipsProcessor) Probe(r io.Reader) (Meta, error) {
	prefix, rest, err := readProbePrefix(r)
	if err != nil {
		return Meta{}, ErrUnsupported
	}
	if SniffHEIF(prefix) {
		// rest already includes prefix (readProbePrefix rewinds via MultiReader).
		data, rerr := io.ReadAll(rest)
		if rerr != nil {
			return Meta{}, ErrUnsupported
		}
		return probeHEIF(data)
	}
	return goProcessor{}.Probe(rest)
}
