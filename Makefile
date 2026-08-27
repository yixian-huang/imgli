.PHONY: build build-go build-vips web web-ci test test-web test-vips font-subset run release-snapshot docker-build pre-tag smoke-public

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Local: install deps then build embeddable frontend.
web:
	cd web && npm install --no-fund --no-audit && npm run build

# CI / GoReleaser: deps already installed via npm ci — only build.
web-ci:
	cd web && npm run build

# 本地默认：纯 Go（CGO 关、无系统 libvips；CI 同）。发行 Docker 镜像默认带 vips（见 Dockerfile）。
build-go:
	go build -ldflags "-X github.com/yixian-huang/imgli/internal/version.Version=$(VERSION)" -o imgli ./cmd/imgli

# libvips 构建:WebP 缩略图 + 原图转 WebP + HEIC 解码。HEIC 需要 libheif（不是仅 WebP）；
# 需本机 pkg-config vips + cgo；交叉编译请在目标机本机构建。
build-vips: web
	CGO_ENABLED=1 go build -tags vips -ldflags "-X github.com/yixian-huang/imgli/internal/version.Version=$(VERSION)-vips" -o imgli ./cmd/imgli

# Prefer web-ci when node_modules already present (CI).
build: web build-go

pre-tag:
	@test -n "$(TAG)" || (echo "usage: make pre-tag TAG=v0.9.6" >&2; exit 2)
	./scripts/pre-tag-check.sh $(TAG)

smoke-public:
	./scripts/ops-smoke-public.sh $(or $(BASE_URL),https://img.li)


# 与生产一致的发行镜像（含 libvips）。
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t imgli:local .

test:
	go vet ./...
	go test ./... -count=1

# libvips 手测/自营站点门禁(CI 默认纯 Go；本机需 libvips 开发包)。
test-vips:
	CGO_ENABLED=1 go test -tags vips ./internal/imaging/ ./internal/service/upload/ -count=1

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
