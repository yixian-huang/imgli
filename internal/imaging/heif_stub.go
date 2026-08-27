//go:build !vips

package imaging

// HeicDecodeAvailable 纯 Go 构建无 HEIF 解码器。
func HeicDecodeAvailable() bool { return false }

// DecodeHEIFToJPEG 纯 Go 构建不可用。
func DecodeHEIFToJPEG(_ []byte, _ int) ([]byte, Meta, error) {
	return nil, Meta{}, ErrHeicUnavailable
}
