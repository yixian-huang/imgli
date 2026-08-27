package imaging

import (
	"bytes"
	"errors"
	"testing"
)

func ftypBox(brand string) []byte {
	// size(4) + "ftyp" + brand(4) + minor(4)
	b := make([]byte, 16)
	b[3] = 16
	copy(b[4:8], "ftyp")
	copy(b[8:12], brand)
	return b
}

func TestSniffHEIFBrands(t *testing.T) {
	for _, brand := range []string{"heic", "heix", "heif", "mif1", "msf1", "heis"} {
		if !SniffHEIF(ftypBox(brand)) {
			t.Errorf("brand %s: want true", brand)
		}
	}
	if SniffHEIF(ftypBox("isom")) {
		t.Error("isom must not sniff as HEIF")
	}
	if SniffHEIF([]byte("MZ not iso bmff")) {
		t.Error("random bytes")
	}
	if SniffHEIF(nil) || SniffHEIF([]byte("ftyp")) {
		t.Error("short buffer")
	}
}

func TestHEIFAllowExt(t *testing.T) {
	if g := HEIFAllowExt("IMG_1.HEIC"); g != "heic" {
		t.Errorf("HEIC → %q", g)
	}
	if g := HEIFAllowExt("a.heif"); g != "heif" {
		t.Errorf("heif → %q", g)
	}
	if g := HEIFAllowExt("noext"); g != "heic" {
		t.Errorf("none → %q", g)
	}
}

func TestProbeHEIFMagicIsUnavailableOnPureGo(t *testing.T) {
	if HeicDecodeAvailable() {
		t.Skip("vips+heif build")
	}
	_, err := NewGo().Probe(bytes.NewReader(ftypBox("heic")))
	if !errors.Is(err, ErrHeicUnavailable) {
		t.Errorf("err=%v, want ErrHeicUnavailable", err)
	}
}

func TestDecodeHEIFToJPEGUnavailableOnPureGo(t *testing.T) {
	if HeicDecodeAvailable() {
		t.Skip("vips+heif build")
	}
	_, _, err := DecodeHEIFToJPEG(ftypBox("heic"), 90)
	if !errors.Is(err, ErrHeicUnavailable) {
		t.Errorf("err=%v", err)
	}
}
