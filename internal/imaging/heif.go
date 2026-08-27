package imaging

import (
	"bytes"
	"errors"
	"io"
	"path"
	"strings"
)

// ErrHeicUnavailable 魔数已认出 HEIF，但本进程没有可用的 HEIF 解码器。
var ErrHeicUnavailable = errors.New("imaging: HEIC requires libvips+libheif")

var heifBrands = map[string]struct{}{
	"heic": {}, "heix": {}, "hevc": {}, "hevx": {},
	"heim": {}, "heis": {}, "hevm": {}, "hevs": {},
	"mif1": {}, "msf1": {}, "heif": {},
}

// SniffHEIF 识别 ISO-BMFF ftyp 品牌。文件名不可靠（iCloud 导出、无后缀）。
func SniffHEIF(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	if string(b[4:8]) != "ftyp" {
		return false
	}
	brand := string(b[8:12])
	if _, ok := heifBrands[brand]; ok {
		return true
	}
	// compatible brands start at offset 16, 4 bytes each
	for i := 16; i+4 <= len(b); i += 4 {
		if _, ok := heifBrands[string(b[i:i+4])]; ok {
			return true
		}
	}
	return false
}

// HEIFAllowExt 组白名单用的原始后缀：.heif → heif，其余（含无后缀）→ heic。
func HEIFAllowExt(filename string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(filename), "."))
	if ext == "heif" {
		return "heif"
	}
	return "heic"
}

func readProbePrefix(r io.Reader) ([]byte, io.Reader, error) {
	buf := make([]byte, 64)
	n, err := io.ReadFull(r, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		buf = buf[:n]
		err = nil
	}
	if err != nil {
		return nil, nil, err
	}
	return buf, io.MultiReader(bytes.NewReader(buf), r), nil
}
