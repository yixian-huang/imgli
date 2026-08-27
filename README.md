<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/svg/lockup-dark.svg">
    <img src="docs/brand/svg/lockup-light.svg" alt="imgli" width="360">
  </picture>
</p>

<p align="center"><b>Self-hosted image hosting — one leap to a link.</b></p>
<p align="center">
  <a href="LICENSE"><img alt="License: AGPL-3.0-only" src="https://img.shields.io/badge/license-AGPL--3.0--only-blue.svg"></a>
  <a href="https://github.com/yixian-huang/imgli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/yixian-huang/imgli"></a>
  <a href=".github/workflows/ci.yml"><img alt="CI" src="https://github.com/yixian-huang/imgli/actions/workflows/ci.yml/badge.svg"></a>
</p>

<p align="center">English · <a href="README.zh-CN.md">简体中文</a></p>

**imgli** is a single-binary image hosting service written in Go, with an
embedded React frontend. Upload a screenshot, get a link. Runs the public
instance at [img.li](https://img.li).

## Demos

<p align="center">
  <img src="docs/screenshots/01-upload.png" alt="Upload page — drag, paste, URL fetch" width="820">
</p>
<p align="center"><sub>Upload: drag-and-drop, screenshot paste, URL fetch, expiry / view limits</sub></p>

<p align="center">
  <img src="docs/screenshots/03-share.png" alt="Public share page with copy actions" width="820">
</p>
<p align="center"><sub>Share page: preview + copy URL / Markdown / share link</sub></p>

<p align="center">
  <img src="docs/screenshots/05-admin.png" alt="Admin dashboard with review queue" width="820">
</p>
<p align="center"><sub>Admin: users, storage policies, review queue, audit logs</sub></p>

More captures (library, API tokens, detail): [`docs/screenshots/`](docs/screenshots/).

## Compare (honest snapshot)

| | **imgli** | **Lsky Pro** | **Chevereto** |
|---|-----------|--------------|---------------|
| Runtime | Single Go binary (+ optional OCR sidecar) | PHP + web server + DB | PHP + web server + DB |
| Default license | **AGPL-3.0-only** (commercial dual-license available) | Apache-2.0 (community) | Proprietary (paid editions) |
| Storage | Local, **S3-compatible**, WebDAV; CDN `302` offload; private presign | Local / cloud drivers (ecosystem varies) | Broad drivers in paid product |
| Content safety | Pluggable NSFW endpoint + OCR keywords, review queue, groups | Plugins / external tools | Features depend on edition |
| Ship shape | `curl \| sh`, Docker, Compose, `imgli doctor` | Classic PHP deploy | Classic PHP deploy |
| Integrations | API tokens, CLI upload, PicGo / ShareX / uPic | Mature plugin/client ecosystem | Mature client ecosystem |

Choose imgli when you want **self-host ops like a small Go service** with modern
storage/privacy controls. Choose Lsky/Chevereto when you need their specific
PHP ecosystem or commercial support already in place. Feature sets change —
verify upstream docs before migrating.

## Features

- **Single binary** — frontend embedded via `go:embed`; SQLite by default,
  PostgreSQL supported. Local/CI builds are pure Go (no CGO). **Official Docker
  images ship with libvips** (WebP thumbnails + optional original→WebP). Use
  `make build` for pure Go or `make build-vips` when developing with vips.
- **Storage backends** — local disk, **S3-compatible** (verified against
  MinIO/RustFS; vendor test toolkit included), WebDAV, optional **FTP
  compatibility** tier (prefer OpenList/proxy; see [docs/storage-ftp.md](docs/storage-ftp.md)).
  Per-policy CDN domain
  with `302` offloading, presigned-URL direct serving for private images.
- **Content safety** — pluggable moderation pipeline: NSFW detection endpoint
  + OCR keyword screening (self-hosted sidecar in `deploy/ocr-paddle/`),
  review queue, per-group policies.
- **Accounts & sharing** — user groups with quotas/rate limits, group expiry/max-views & retention ([docs/user-groups-lifecycle.md](docs/user-groups-lifecycle.md)), guest upload,
  invite codes, SMTP email (verification/reset/reject notices), albums,
  public gallery (`/a/{id}`), recycle bin, image expiry, access passwords,
  max views; Open Graph previews on share/album pages.
- **Integrations** — clean upload API with API tokens; `imgli upload` /
  `imgli import-dir` CLI; optional **OIDC** SSO and outbound **webhooks**;
  PicGo/Typora/VS Code ([guide](docs/picgo.md)),
  [ShareX](docs/integrations/sharex.md) / [uPic](docs/integrations/upic.md)
  ([index](docs/integrations/README.md)).
- **HEIC / HEIF** — official Docker images decode HEIC/HEIF uploads and store JPEG (then the existing processing pipeline, including optional original WebP). GitHub pure-Go archives reject with `heic_unsupported`.
- **Transforms** — controlled thumbnails via `/t/{key}?w=120|200|240|400|480|800|960|1600`.
- **Ops (v0.6+)** — admin **cross-policy storage migrate** (progress / resume /
  size check; [docs/storage-migrate.md](docs/storage-migrate.md)); **version
  display + GitHub update probe + one-click binary upgrade** (Docker = image
  redeploy); **lifecycle cleanup** dry-run + confirm.
- **Ops console (v0.7–v0.9)** — admin **System / Ops**: doctor health
  (`GET /admin/system/health`), upgrade preflight, reverse-proxy CSRF hints;
  **v0.8** image soft/hard delete + storage locate; **v0.9** group lifecycle
  cleanup kinds (`expired` / `trash` / `group_retention` / `group_force_age`),
  stock clamp, batch purge. Guides:
  [user-groups-lifecycle](docs/user-groups-lifecycle.md) ·
  [cleanup vs CDN](docs/ops-cleanup-cdn-boundary.md) ·
  [security-hardening](docs/security-hardening.md#faq-reverse-proxy-loginregister-cross-site-rejected).
- **Patch polish (v0.9.2)** — same-user instant upload reuse (no library
  duplicates / double quota); `site_name` on nav wordmark (carp mark kept);
  storage policy live/trash image counts + object bytes. Acceptance:
  [docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md](docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md);
  customization IA: [docs/design/site-customization-ia.md](docs/design/site-customization-ia.md).
- **v0.9.3** — admin users ops (bandwidth / last-seen / header sort / icon
  confirms); **Tailwind CSS v4** UI rewrite with shared admin chrome.
- **v0.9.4** — admin trash restore; denser users table + row expand; processing
  JPEG quality / original WebP (vips); Docker ships libvips by default; System
  page imaging capabilities.
- **v0.9.5** — dark-mode fix after Tailwind (`@theme inline`); **site appearance**:
  accent color, full-page background, scrim + frosted glass (borders soften with
  opacity); admin radii/layout polish. [CHANGELOG](CHANGELOG.md) ·
  [customization IA](docs/design/site-customization-ia.md).
- **v0.9.6** — self-host robustness: SQLite low-RAM/bind-mount OOM mitigations,
  Docker entrypoint ownership fix, libvips concurrency cap.
- **v0.9.12** — private albums no longer leak to plaza / anonymous `/i` / `/s`;
  optional origin serve cache under `{data_dir}/.serve-cache`.
- **v0.9.13** — upload drag-and-drop: one drop queues once; leaving the page
  (or moving inside the drop zone) no longer sticks or flickers the overlay.
- **v0.9.16** — admin grid no longer shows `NOT FOUND` for public photos that
  still load in the owner's library (`/t` CDN fallback + empty surface); admins
  can preview other users' private/pending thumbs.
- **v0.9.15** — MinIO/S3 first `/t` after upload no longer JSON-500s; missing
  thumbs generate on demand; SPA images retry once.
- **v0.9.14** — Lark/Feishu SMTP (LOGIN, form test send, clearer errors) and
  customizable zh/en mail copy for welcome / verify / reset / change-email / reject.
- **Polish** — bilingual UI (中文/English), PWA, light/dark/**system** theme,
  text watermark (embedded CJK font subset), admin dashboard with audit logs
  and light ops analytics.

## Quick start

### Binary (one-liner)

Linux / macOS — downloads the latest [GitHub Release](https://github.com/yixian-huang/imgli/releases)
asset for your OS/arch into `~/.local/bin` (override with `PREFIX=...`):

```bash
curl -fsSL https://raw.githubusercontent.com/yixian-huang/imgli/main/scripts/install.sh | sh
imgli serve
# → http://localhost:8686  (first registered user becomes admin)
```

Pin a version or install location:

```bash
curl -fsSL https://raw.githubusercontent.com/yixian-huang/imgli/main/scripts/install.sh | sh -s -- v0.9.16
PREFIX=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/yixian-huang/imgli/main/scripts/install.sh | sh
```

Windows: download the `Windows_*.zip` from
[Releases](https://github.com/yixian-huang/imgli/releases), or use the script under Git Bash.

### Docker (prebuilt)

```bash
docker run --rm -p 8686:8686 -v imgli-data:/data \
  -e IMGLI_BASE_URL=http://localhost:8686 \
  ghcr.io/yixian-huang/imgli:latest
# → http://localhost:8686  (first registered user becomes admin)
```

Pin a release with `ghcr.io/yixian-huang/imgli:v0.9.16` (see
[Releases](https://github.com/yixian-huang/imgli/releases)).

### Docker Compose

```bash
git clone https://github.com/yixian-huang/imgli && cd imgli
docker compose up -d
# → http://localhost:8686  (first registered user becomes admin)
```

Production-oriented example (volumes, healthcheck, loopback bind, proxy notes):
[`deploy/compose.prod.example.yml`](deploy/compose.prod.example.yml). TLS reverse
proxy snippets: [`deploy/Caddyfile.example`](deploy/Caddyfile.example),
[`deploy/traefik.labels.example.yml`](deploy/traefik.labels.example.yml).
Backup / restore: [`docs/backup.md`](docs/backup.md).

### From source

```bash
make build          # needs Go ≥ 1.26 and Node ≥ 24
./imgli version     # git tag via ldflags, e.g. v0.9.16
./imgli serve       # → http://localhost:8686
```

## Configuration

Ops-level config only — everything product-facing (site name, registration
mode, groups, storage policies, SMTP, moderation) lives in the admin panel.

Precedence: defaults → YAML file (`imgli serve -config imgli.yaml`, see
[`deploy/imgli.example.yaml`](deploy/imgli.example.yaml)) → environment:

| Env | Required? | Default | Meaning |
|---|---|---|---|
| `IMGLI_BASE_URL` | **Yes on public deploy** | `http://localhost:8686` | Exact browser origin (`https://your.domain`) for links, cookies, CSRF; leave default for local tryout |
| `IMGLI_TRUST_PROXY` | **Yes behind reverse proxy** | `false` | Trust `X-Forwarded-For` only when a trusted proxy sits in front |
| `IMGLI_LISTEN` | Optional | `:8686` | Listen address |
| `IMGLI_DATA_DIR` | Optional | `./data` | Local storage + default SQLite file dir (Docker image usually `/data` + volume) |
| `IMGLI_DATABASE_DRIVER` | Optional | `sqlite` | `sqlite` \| `postgres` |
| `IMGLI_DATABASE_DSN` | **Required for Postgres** | auto SQLite path | Postgres DSN; leave empty for default SQLite under `data_dir` |
| `IMGLI_FETCH_ALLOW` | Optional | *(empty)* | Extra hosts/CIDRs for URL-fetch upload (default denies private nets) |
| `IMGLI_RATE_LIMIT_MULT` | Optional | `1` | Rate-limit multiplier; keep `1` in production |
| `IMGLI_SERVE_CACHE_DISABLED` | Optional | `false` | Disable local proxy cache for public `/t` and streamed `/i` |
| `IMGLI_SERVE_CACHE_MAX_BYTES` | Optional | `0` (512MiB) | Cap for `{data_dir}/.serve-cache` |

### Reverse proxy: login/register rejected as cross-site

If registration/login returns 403 with `跨站请求被拒绝` (cross-site request
rejected) after Nginx / Caddy / Traefik / 1Panel reverse proxy, but
`http://IP:port` works, set `IMGLI_BASE_URL` to the **exact public origin**
users open (not the default localhost or the backend IP:port):

```bash
IMGLI_BASE_URL=https://img.example.com
IMGLI_TRUST_PROXY=true
```

Details:
[docs/security-hardening.md](docs/security-hardening.md#faq-reverse-proxy-loginregister-cross-site-rejected).

## Upload API

```bash
curl -X POST https://your-host/api/v1/upload \
  -H "Authorization: Bearer <API_TOKEN>" \
  -F file=@shot.png -F visibility=public
# → data.links.{url,markdown,html,bbcode,thumbnail_url}
```

Create tokens under **Settings → API Token**. Client guides:
[PicGo](docs/picgo.md) · [ShareX](docs/integrations/sharex.md) ·
[uPic](docs/integrations/upic.md) · [integrations index](docs/integrations/README.md).

### CLI upload

With a built binary (or `go run ./cmd/imgli`), upload a file or stdin:

```bash
export IMGLI_BASE_URL=https://your-host
export IMGLI_TOKEN='paste-token-once'
imgli upload shot.png
# → https://your-host/i/….png

imgli upload -verbose shot.png              # stderr: group expiry / max-views limits
imgli upload -format markdown shot.png
imgli upload -format json -visibility private shot.png
imgli upload -expires-in 86400 shot.png
cat shot.png | imgli upload -name shot.png -
```

Flags: `-base-url`, `-token`, `-format url|markdown|json`, `-visibility`,
`-expires-in`, `-verbose`, `-name` (stdin filename). Group policy:
[docs/user-groups-lifecycle.md](docs/user-groups-lifecycle.md).

### Doctor (self-host checks)

```bash
imgli doctor
imgli doctor -config /path/to/imgli.yaml
# exit 1 if any FAIL; WARN alone exits 0
```

Checks data directory writability, database connectivity, `base_url` shape,
`trust_proxy` guidance, listen address, and enabled **local** storage policies
(write/read/delete probe). **v0.7+** also surfaces the same doctor report in
admin **System / Ops** (`GET /admin/system/health`). See also
[docs/security-hardening.md](docs/security-hardening.md).

## Development

```bash
make test        # go vet + go test (sqlite; set IMGLI_TEST_PG_DSN for postgres)
make test-web    # vitest
cd web && npm run e2e   # Playwright, builds the binary first
```

See [CONTRIBUTING.md](CONTRIBUTING.md) (includes versioning / release steps).
Changelog: [CHANGELOG.md](CHANGELOG.md). Security: [SECURITY.md](SECURITY.md) ·
[hardening](docs/security-hardening.md). S3 matrix:
[docs/s3-compatibility.md](docs/s3-compatibility.md).


## Docs map (self-hosters)

- Storage matrices: [S3](docs/s3-compatibility.md) · [WebDAV](docs/webdav-compatibility.md) · [FTP dual-track](docs/storage-ftp.md)
- **How to fill S3 policy fields (CDN / prefix / path style / path template):** [docs/s3-compatibility.md#storage-policy-fields-s3](docs/s3-compatibility.md#storage-policy-fields-s3)
- **Storage migrate (ops):** [docs/storage-migrate.md](docs/storage-migrate.md) — CLI + Admin job
- **User-group lifecycle (v0.9):** [docs/user-groups-lifecycle.md](docs/user-groups-lifecycle.md) — expiry/max-views, retention, force age, stock clamp
- **Site customization (L0 / L2 appearance):** [docs/design/site-customization-ia.md](docs/design/site-customization-ia.md) — `site_name`, accent, background, glass (v0.9.5)
- **v0.9.2 acceptance:** [docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md](docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md)
- **Cleanup vs CDN:** [docs/ops-cleanup-cdn-boundary.md](docs/ops-cleanup-cdn-boundary.md) — cleanup kinds + CDN boundary
- **Release / CI / deploy:** [docs/ops-release.md](docs/ops-release.md) — tag gate, scripts, smoke
- **Deploy checklist (SPA outage guard):** [docs/ops-deploy-checklist.md](docs/ops-deploy-checklist.md)
- **Reverse proxy / CSRF:** [docs/security-hardening.md](docs/security-hardening.md#faq-reverse-proxy-loginregister-cross-site-rejected) (admin System/Ops health in v0.7+)
- **OIDC troubleshooting:** [docs/oidc-operator.md](docs/oidc-operator.md)
- Integrations: [docs/integrations/README.md](docs/integrations/README.md) · CLI `imgli upload -verbose`
- Migration into imgli: `imgli import-dir` (folder → upload API)
- Moderation operator path: [docs/moderation-spot-check.md](docs/moderation-spot-check.md)
- Docs SSOT map: [docs/documentation-ssot.md](docs/documentation-ssot.md)
- Public roadmap mirror: [ROADMAP.md](ROADMAP.md) (execution = GitHub Issues)
- Product docs: [docs.imgli.com](https://docs.imgli.com) · site / demo: [imgli.com](https://imgli.com) · [img.li](https://img.li)
- Screenshots: [docs/screenshots/](docs/screenshots/)

## License

**[AGPL-3.0-only](LICENSE)** © 2026 Yixian Huang.

- Self-host and modify freely under the AGPL (network use of modified versions
  requires offering corresponding source).
- Need proprietary / multi-tenant use without AGPL obligations? See
  **[COMMERCIAL.md](COMMERCIAL.md)** for paid licensing.
- Tags **v0.1.0** / **v0.1.1** remain available under **MIT** (historical).

Embedded Noto Sans SC subset: [SIL OFL](internal/imaging/fonts/OFL.txt).
Third-party notices: [NOTICE](NOTICE).
