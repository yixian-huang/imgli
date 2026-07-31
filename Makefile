.PHONY: build build-go build-vips web test test-web test-vips font-subset run release-snapshot

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

web:
	cd web && npm install --no-fund --no-audit && npm run build

build-go:
	go build -ldflags "-X github.com/yixian-huang/imgli/internal/version.Version=$(VERSION)" -o imgli ./cmd/imgli

# libvips 构建:缩略图 WebP(需本机 pkg-config vips + cgo；交叉编译请在目标机本机构建)。
build-vips: web
	CGO_ENABLED=1 go build -tags vips -ldflags "-X github.com/yixian-huang/imgli/internal/version.Version=$(VERSION)-vips" -o imgli ./cmd/imgli

build: web build-go

test:
	go vet ./...
	go test ./... -count=1

# libvips 手测/自营站点门禁(CI 默认纯 Go；本机需 libvips 开发包)。
test-vips:
	CGO_ENABLED=1 go test -tags vips ./internal/imaging/ -count=1

test-web:
	cd web && npm test

# 再生水印字体子集(需 fonttools + 完整 NotoSansSC 源文件)。
font-subset:
	./scripts/subset-watermark-font.sh

run: build
	./imgli serve

# 本地试跑 GoReleaser（需已安装 goreleaser；不发布、不打 tag）。
release-snapshot:
	goreleaser release --snapshot --clean --skip=publish
