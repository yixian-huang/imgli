<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/svg/lockup-dark.svg">
    <img src="docs/brand/svg/lockup-light.svg" alt="imgli" width="360">
  </picture>
</p>

<p align="center"><b>自托管图床——一跃成链。</b></p>

<p align="center">
  <a href="LICENSE"><img alt="License: AGPL-3.0-only" src="https://img.shields.io/badge/license-AGPL--3.0--only-blue.svg"></a>
  <a href="https://github.com/yixian-huang/imgli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/yixian-huang/imgli"></a>
  <a href=".github/workflows/ci.yml"><img alt="CI" src="https://github.com/yixian-huang/imgli/actions/workflows/ci.yml/badge.svg"></a>
</p>

<p align="center"><a href="README.md">English</a> · 简体中文</p>

**imgli** 是 Go 编写的单二进制图床,内嵌 React 前端。截图上传,直接得链接。
公共实例:[img.li](https://img.li)。

## 演示

<p align="center">
  <img src="docs/screenshots/01-upload.png" alt="上传页：拖拽、粘贴、URL 抓取" width="820">
</p>
<p align="center"><sub>上传：拖拽 / 截图粘贴 / URL 抓取，可选过期与访问次数</sub></p>

<p align="center">
  <img src="docs/screenshots/03-share.png" alt="公开分享页与复制按钮" width="820">
</p>
<p align="center"><sub>分享页：预览 + 复制 URL / Markdown / 分享链接</sub></p>

<p align="center">
  <img src="docs/screenshots/05-admin.png" alt="管理后台与审核入口" width="820">
</p>
<p align="center"><sub>管理后台：用户、存储策略、审核队列、审计日志</sub></p>

更多截图（图库、API Token、详情）：[`docs/screenshots/`](docs/screenshots/)。

## 对比（诚实快照）

| | **imgli** | **兰空 Lsky Pro** | **Chevereto** |
|---|-----------|-------------------|---------------|
| 运行时 | Go 单二进制（可选 OCR 旁路） | PHP + Web 服务器 + 数据库 | PHP + Web 服务器 + 数据库 |
| 默认协议 | **AGPL-3.0-only**（另有商业双许可） | Apache-2.0（社区版） | 闭源付费为主 |
| 存储 | 本地、**S3 兼容**、WebDAV；CDN `302`；私有预签名 | 本地 / 云（生态各异） | 付费版驱动较全 |
| 内容安全 | 可插拔 NSFW + OCR 词表、审核队列、用户组 | 插件 / 外部工具 | 随版本/商业版变化 |
| 交付 | `curl \| sh`、Docker、Compose、`imgli doctor` | 经典 PHP 部署 | 经典 PHP 部署 |
| 对接 | API Token、CLI、PicGo / ShareX / uPic | 插件与客户端生态成熟 | 客户端生态成熟 |

更在意 **像小 Go 服务一样运维**、以及现代存储/隐私能力时选 imgli；需要其 PHP
生态或已有商业支持时选 Lsky / Chevereto。功能会变，迁移前请对照上游文档。

## 特性

- **单二进制**:前端 `go:embed` 内嵌;默认 SQLite、支持 PostgreSQL。本地/CI 默认可无 CGO；
  **官方 Docker 镜像默认带 libvips**（WebP 缩略图与可选原图转 WebP）。纯 Go 二进制仍可用
  `make build`；本机要 vips 时用 `make build-vips`。
- **多存储**:本地盘、**S3 兼容**(MinIO/RustFS 已真机验证,附厂商验证工具包)、WebDAV
  (OpenList/网盘出口;矩阵见 [docs/webdav-compatibility.md](docs/webdav-compatibility.md));
  可选 **FTP 兼容层**(功能受限;优先 OpenList/外置代理,见 [docs/storage-ftp.md](docs/storage-ftp.md));
  策略级 CDN 域 `302` 卸带宽,私密图预签名直连。
- **内容安全**:可插拔机审链——NSFW 检测端点 + OCR 词表筛查
  (自托管旁路服务见 `deploy/ocr-paddle/`),审核队列,按用户组配策略。
- **账号与分享**:用户组配额/限速、**组级有效期与访问次数 / 保留与强制存活**
  （[docs/user-groups-lifecycle.md](docs/user-groups-lifecycle.md)）、游客上传、邀请码、SMTP 邮件
  (验证/重置/拒审通知)、相册、公开相册访客页 `/a/{id}`、回收站、图片过期、
  访问口令、阅后即焚/次数上限；分享页与公开相册自动 Open Graph 预览。
- **生态对接**:干净的上传 API + API Token;`imgli upload` / `imgli import-dir` CLI；
  可选 **OIDC** SSO 与出站 **Webhook**；PicGo/Typora/VS Code
  ([指南](docs/picgo.md))、[ShareX](docs/integrations/sharex.md) /
  [uPic](docs/integrations/upic.md)（[索引](docs/integrations/README.md)）。
- **HEIC / HEIF**：官方 Docker 镜像解码 HEIC/HEIF 上传并存储为 JPEG（随后走现有处理管线，含可选原图 WebP）。GitHub 纯 Go 归档返回 `heic_unsupported`。
- **变换**:受控缩略 `/t/{key}?w=120|200|240|400|480|800|960|1600`。
- **运维（v0.6+）**:管理后台 **跨策略存储搬迁**（进度/续跑/size 校验；见
  [docs/storage-migrate.md](docs/storage-migrate.md)）；**版本展示 + 探测更新 +
  一键二进制升级**（Docker 请换镜像）；**生命周期清理**（dry-run + 确认执行）。
- **运维控制台（v0.7–v0.9）**:管理后台 **系统 / 运维**——doctor 健康检查、升级 preflight、
  反代 CSRF 提示；**v0.8** 图片软删/彻底删除与存储定位；**v0.9** 组级保留/强制存活清理
  kinds、存量有效期钳制、批量硬删。文档：
  [用户组生命周期](docs/user-groups-lifecycle.md) ·
  [清理与 CDN](docs/ops-cleanup-cdn-boundary.md) ·
  [反代 FAQ](docs/security-hardening.md#faq-reverse-proxy-loginregister-cross-site-rejected)。
- **补丁打磨（v0.9.2）**:同用户秒传复用（图库不重复、不二次扣配额）；`site_name` 进顶栏字标
  （鲤鱼标保留）；存储策略 live/trash 图片数与对象占用。验收：
  [docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md](docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md)；
  站点定制 IA：[docs/design/site-customization-ia.md](docs/design/site-customization-ia.md)。
- **v0.9.3**：管理端用户运营表（流量/最近访问/表头排序/图标二次确认）；前端 **Tailwind CSS v4**
  全站样式重写 + 共享 admin chrome。
- **v0.9.4**：管理端回收站恢复；用户表合并列+展开；图片处理 JPEG 质量 / 原图 WebP（vips）；
  Docker 默认 libvips；系统页图像能力展示。
- **v0.9.5**：暗色模式修复（Tailwind `@theme inline`）；**站点外观**：强调色、整站背景图、
  遮罩与毛玻璃不透明度（边框随透明度变淡）；管理台圆角/标题栏布局打磨。
  [CHANGELOG](CHANGELOG.md) · [站点定制 IA](docs/design/site-customization-ia.md)。
- **v0.9.6**：自托管健壮性——SQLite 低内存/绑定挂载 OOM 缓解、Docker entrypoint 修正
  绑定卷属主、libvips 默认并发上限。
- **v0.9.12**：私密相册不再漏到广场 / 匿名 `/i` / `/s`；公开源站缓存（`{data_dir}/.serve-cache`，可关）。
- **v0.9.13**：上传拖放一次只入队一次；拖出页面或在投放区内移动不再卡住/闪烁 overlay。
- **v0.9.16**：后台格子不再对前台还能看见的公开图显示 NOT FOUND（`/t` 走 CDN 回源、空 surface 按 public 探测）；管理员可预览他人私密/待审缩略图。
- **v0.9.15**：MinIO/S3 上传后第一次 `/t` 不再 JSON 500；缺缩略图会从原图补；站内图片失败会再拉一次。
- **v0.9.14**：飞书/Lark SMTP（LOGIN、用表单测发信、错误可操作）以及欢迎/验证/重置/换绑/拒绝邮件的中英文案覆盖。
- **细节**:中英双语界面、PWA、浅色/深色/**跟随系统**主题、文字水印(内嵌中文字体子集)、
  带审计日志与轻量运营统计的管理后台。

## 快速开始

### 二进制一键安装

Linux / macOS — 自动下载当前系统对应的
[GitHub Release](https://github.com/yixian-huang/imgli/releases) 产物到 `~/.local/bin`
（可用 `PREFIX=...` 改目录）：

```bash
curl -fsSL https://raw.githubusercontent.com/yixian-huang/imgli/main/scripts/install.sh | sh
imgli serve
# → http://localhost:8686（第一个注册用户即管理员）
```

固定版本或安装路径：

```bash
curl -fsSL https://raw.githubusercontent.com/yixian-huang/imgli/main/scripts/install.sh | sh -s -- v0.9.16
PREFIX=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/yixian-huang/imgli/main/scripts/install.sh | sh
```

Windows：从 [Releases](https://github.com/yixian-huang/imgli/releases) 下载 `Windows_*.zip`，
或在 Git Bash 下运行上述脚本。

### Docker（预构建镜像）

```bash
docker run --rm -p 8686:8686 -v imgli-data:/data \
  -e IMGLI_BASE_URL=http://localhost:8686 \
  ghcr.io/yixian-huang/imgli:latest
# → http://localhost:8686（第一个注册用户即管理员）
```

固定版本用 `ghcr.io/yixian-huang/imgli:v0.9.16`（见
[Releases](https://github.com/yixian-huang/imgli/releases)）。

### Docker Compose

```bash
git clone https://github.com/yixian-huang/imgli && cd imgli
docker compose up -d
# → http://localhost:8686 (第一个注册用户即管理员)
```

生产向示例（卷、健康检查、回环绑定、反代说明）：
[`deploy/compose.prod.example.yml`](deploy/compose.prod.example.yml)。
TLS 反代片段：[`deploy/Caddyfile.example`](deploy/Caddyfile.example)、
[`deploy/traefik.labels.example.yml`](deploy/traefik.labels.example.yml)。
备份与恢复：[`docs/backup.md`](docs/backup.md)。

### 源码构建

```bash
make build          # 需要 Go ≥ 1.26、Node ≥ 24
./imgli version     # ldflags 注入的 git tag，如 v0.9.16
./imgli serve       # → http://localhost:8686
```

## 配置

这里只有运维层配置——产品层(站点名、注册模式、用户组、存储策略、SMTP、机审)
全部在管理后台设置。

优先级:默认值 → YAML(`imgli serve -config imgli.yaml`,
见 [`deploy/imgli.example.yaml`](deploy/imgli.example.yaml))→ 环境变量:

| 环境变量 | 是否必需 | 默认 | 含义 |
|---|---|---|---|
| `IMGLI_BASE_URL` | **公网部署必需** | `http://localhost:8686` | 浏览器访问源（`https://你的域名`）；外链/邮件/Cookie/CSRF。本机试玩可保持默认 |
| `IMGLI_TRUST_PROXY` | **有反代时必需 true** | `false` | 信任 `X-Forwarded-For`（仅可信反代后） |
| `IMGLI_LISTEN` | 可选 | `:8686` | 监听地址 |
| `IMGLI_DATA_DIR` | 可选 | `./data` | 本地存储与默认 SQLite 目录（镜像内常为 `/data`） |
| `IMGLI_DATABASE_DRIVER` | 可选 | `sqlite` | `sqlite` \| `postgres` |
| `IMGLI_DATABASE_DSN` | **Postgres 时必需** | SQLite 自动路径 | Postgres DSN；SQLite 请留空 |
| `IMGLI_FETCH_ALLOW` | 可选 | *(空)* | URL 抓取上传额外放行的 host/CIDR |
| `IMGLI_RATE_LIMIT_MULT` | 可选 | `1` | 限速倍率；生产保持 1 |
| `IMGLI_SERVE_CACHE_DISABLED` | 可选 | `false` | 关闭公开 `/t` 与流式 `/i` 的本地代理缓存 |
| `IMGLI_SERVE_CACHE_MAX_BYTES` | 可选 | `0`（512MiB） | `{data_dir}/.serve-cache` 上限 |

### 反代后无法注册/登录（「跨站请求被拒绝」）

用 1Panel / Nginx / Caddy 等反代到域名后，登录注册 403 提示「跨站请求被拒绝」，
而 `http://IP:端口` 正常——通常是 **`IMGLI_BASE_URL` 仍是 localhost 或 IP 端口**，
与浏览器里的 `https://你的域名` 不一致。TLS 在反代终止时，应用侧无法仅靠
`Host` 判定 HTTPS 同源，**必须**把 base 设成用户实际访问的公网源，例如：

```bash
IMGLI_BASE_URL=https://img.example.com
IMGLI_TRUST_PROXY=true
```

原因与检查清单见
[docs/security-hardening.md](docs/security-hardening.md#faq-reverse-proxy-loginregister-cross-site-rejected)。

## 上传 API

```bash
curl -X POST https://your-host/api/v1/upload \
  -H "Authorization: Bearer <API_TOKEN>" \
  -F file=@shot.png -F visibility=public
# → data.links.{url,markdown,html,bbcode,thumbnail_url}
```

Token 在 **设置 → API Token** 创建。客户端指南：
[PicGo](docs/picgo.md) · [ShareX](docs/integrations/sharex.md) ·
[uPic](docs/integrations/upic.md) · [integrations 索引](docs/integrations/README.md)。

### CLI 上传

已构建二进制（或 `go run ./cmd/imgli`）后，可上传本地文件或 stdin：

```bash
export IMGLI_BASE_URL=https://your-host
export IMGLI_TOKEN='粘贴一次性显示的 token'
imgli upload shot.png
# → https://your-host/i/….png

imgli upload -verbose shot.png              # stderr 打印组有效期/次数限制
imgli upload -format markdown shot.png
imgli upload -format json -visibility private shot.png
imgli upload -expires-in 86400 shot.png
cat shot.png | imgli upload -name shot.png -
```

参数：`-base-url`、`-token`、`-format url|markdown|json`、`-visibility`、
`-expires-in`、`-verbose`、`-name`（stdin 时的文件名）。组策略见
[docs/user-groups-lifecycle.md](docs/user-groups-lifecycle.md)。

### doctor（自检）

```bash
imgli doctor
imgli doctor -config /path/to/imgli.yaml
# 有 FAIL 时 exit 1；仅有 WARN 时 exit 0
```

检查 data 目录可写、数据库连通、`base_url` 形态、`trust_proxy` 提示、监听地址，
以及对已启用的 **local** 存储策略做写/读/删探针。**v0.7+** 管理后台 **系统 / 运维**
页提供同源健康报告（`GET /admin/system/health`）。亦见
[docs/security-hardening.md](docs/security-hardening.md)。

## 开发

```bash
make test        # go vet + go test(sqlite;设 IMGLI_TEST_PG_DSN 跑 postgres)
make test-web    # vitest
cd web && npm run e2e   # Playwright,会先构建二进制
```

参与贡献见 [CONTRIBUTING.md](CONTRIBUTING.md)（含版本号与发版流程）；
变更记录 [CHANGELOG.md](CHANGELOG.md)；安全 [SECURITY.md](SECURITY.md) ·
[加固清单](docs/security-hardening.md)；S3 兼容矩阵
[docs/s3-compatibility.md](docs/s3-compatibility.md)。


## 文档索引（自托管）

- 存储矩阵：[S3](docs/s3-compatibility.md) · [WebDAV](docs/webdav-compatibility.md) · [FTP 双轨](docs/storage-ftp.md)
- **S3 策略字段怎么填（CDN / 前缀 / 路径风格 / 路径模板）**：[docs/s3-compatibility.md#storage-policy-fields-s3](docs/s3-compatibility.md#storage-policy-fields-s3)
- **跨策略搬迁**：[docs/storage-migrate.md](docs/storage-migrate.md)（CLI + Admin 任务）
- **用户组生命周期（v0.9）**：[docs/user-groups-lifecycle.md](docs/user-groups-lifecycle.md)（有效期/次数上限、保留与强制存活、存量钳制）
- **站点定制（L0 / L2 外观）**：[docs/design/site-customization-ia.md](docs/design/site-customization-ia.md)（站名、强调色、背景图、毛玻璃；v0.9.5）
- **v0.9.2 验收**：[docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md](docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md)
- **清理与 CDN 边界**：[docs/ops-cleanup-cdn-boundary.md](docs/ops-cleanup-cdn-boundary.md)
- **发版 / CI / 部署**：[docs/ops-release.md](docs/ops-release.md)（tag 门禁、脚本、冒烟）
- **生产部署检查清单**：[docs/ops-deploy-checklist.md](docs/ops-deploy-checklist.md)
- **反代 / CSRF**：[docs/security-hardening.md](docs/security-hardening.md#faq-reverse-proxy-loginregister-cross-site-rejected)（v0.7+ 亦可在后台「系统 / 运维」自检）
- **OIDC 运维排错**：[docs/oidc-operator.md](docs/oidc-operator.md)
- 集成：[docs/integrations/README.md](docs/integrations/README.md) · CLI `imgli upload -verbose`
- 迁入 imgli：`imgli import-dir`（本地目录 → 上传 API）
- 机审抽检路径：[docs/moderation-spot-check.md](docs/moderation-spot-check.md)
- 文档 SSOT 地图：[docs/documentation-ssot.md](docs/documentation-ssot.md)
- 公开 Roadmap 镜像：[ROADMAP.md](ROADMAP.md)（执行面 = GitHub Issues）
- 产品文档：[docs.imgli.com](https://docs.imgli.com) · 产品站 / 演示：[imgli.com](https://imgli.com) · [img.li](https://img.li)
- 截图：[docs/screenshots/](docs/screenshots/)

## 许可

**[AGPL-3.0-only](LICENSE)** © 2026 Yixian Huang。

- 可自托管与修改；若修改后通过网络提供服务，需按 AGPL 提供对应源码。
- 需要闭源商用、多租户 SaaS 或无法接受 AGPL 义务时，见
  **[COMMERCIAL.md](COMMERCIAL.md)** 付费许可说明。
- 历史标签 **v0.1.0** / **v0.1.1** 仍为 **MIT**。

内嵌 Noto Sans SC 子集：[SIL OFL](internal/imaging/fonts/OFL.txt)。  
第三方说明：[NOTICE](NOTICE)。
