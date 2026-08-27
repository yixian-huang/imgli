//go:build vips

package imaging

import (
	"bytes"
	"errors"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHeicDecodeAvailableIsRuntime(t *testing.T) {
	_ = HeicDecodeAvailable() // must not panic
}

func TestDecodeHEIFToJPEGRoundtrip(t *testing.T) {
	if !HeicDecodeAvailable() {
		t.Skip("no HEIF loader in this libvips")
	}
	src := makeTinyHEIF(t)
	out, meta, err := DecodeHEIFToJPEG(src, 90)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Ext != "jpg" || meta.MIME != "image/jpeg" {
		t.Errorf("meta=%+v", meta)
	}
	if meta.Width < 1 || meta.Height < 1 {
		t.Errorf("dims %+v", meta)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatal("not jpeg:", err)
	}
	if SniffHEIF(out) {
		t.Fatal("output still sniffs as HEIF")
	}
}

func TestDecodeHEIFToJPEGRejectsOversizeFromProbe(t *testing.T) {
	if !HeicDecodeAvailable() {
		t.Skip("no HEIF loader in this libvips")
	}
	src := makeTinyHEIF(t)
	old := MaxSidePixels
	MaxSidePixels = 1
	t.Cleanup(func() { MaxSidePixels = old })
	var transcoded int
	heifTranscodeHook = func() { transcoded++ }
	t.Cleanup(func() { heifTranscodeHook = nil })
	_, _, err := DecodeHEIFToJPEG(src, 90)
	if !errors.Is(err, ErrDimensionOver) {
		t.Fatalf("err=%v, want ErrDimensionOver", err)
	}
	if transcoded != 0 {
		t.Fatal("heif_to_jpeg must not run after header oversize")
	}
}

func TestVipsProbeHEIF(t *testing.T) {
	if !HeicDecodeAvailable() {
		_, err := NewVips().Probe(bytes.NewReader(ftypBox("heic")))
		if !errors.Is(err, ErrHeicUnavailable) {
			t.Fatalf("err=%v, want ErrHeicUnavailable", err)
		}
		return
	}
	src := makeTinyHEIF(t)
	m, err := NewVips().Probe(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.Width < 1 || m.Height < 1 {
		t.Errorf("meta=%+v", m)
	}
	if m.Ext != "heic" {
		t.Errorf("probe ext=%q, want heic (allowlist applied in upload)", m.Ext)
	}
}

func makeTinyHEIF(t *testing.T) []byte {
	t.Helper()
	png := alphaPNG(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.png")
	out := filepath.Join(dir, "out.heic")
	if err := os.WriteFile(in, png, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("vips", "copy", in, out)
	if err := cmd.Run(); err != nil {
		t.Skip("cannot encode HEIF via vips copy:", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !SniffHEIF(b) {
		t.Skip("vips copy did not produce HEIF")
	}
	return b
}
