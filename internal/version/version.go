// Package version 进程产品版本（由 -ldflags -X …Version= 注入）。
package version

// Version 为 git tag 形态（如 v0.5.1）；开发默认 "dev"。
var Version = "dev"

// DefaultReleaseRepo 探测更新时默认使用的 GitHub 仓库。
const DefaultReleaseRepo = "yixian-huang/imgli"
