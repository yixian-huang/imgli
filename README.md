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
  PostgreSQL supported. No CGO required.
- **Storage backends** — local disk, **S3-compatible** (verified against
  MinIO/RustFS; vendor test toolkit included), WebDAV, optional **FTP
  compatibility** tier (prefer OpenList/proxy; see [docs/storage-ftp.md](docs/storage-ftp.md)).
  Per-policy CDN domain
  with `302` offloading, presigned-URL direct serving for private images.
- **Content safety** — pluggable moderation pipeline: NSFW detection endpoint
  + OCR keyword screening (self-hosted sidecar in `deploy/ocr-paddle/`),
  review queue, per-group policies.
- **Accounts & sharing** — user groups with quotas/rate limits, guest upload,
  invite codes, SMTP email (verification/reset/reject notices), albums,
  public gallery (`/a/{id}`), recycle bin, image expiry, access passwords,
  max views; Open Graph previews on share/album pages.
- **Integrations** — clean upload API with API tokens; `imgli upload` /
  `imgli import-dir` CLI; optional **OIDC** SSO and outbound **webhooks**;
  PicGo/Typora/VS Code ([guide](docs/picgo.md)),
  [ShareX](docs/integrations/sharex.md) / [uPic](docs/integrations/upic.md)
  ([index](docs/integrations/README.md)).
- **Transforms** — controlled thumbnails via `/t/{key}?w=200|400|800`.
- **Ops (v0.6)** — admin **cross-policy storage migrate** (progress / resume /
  size check; [docs/storage-migrate.md](docs/storage-migrate.md)); **version
  display + GitHub update probe + one-click binary upgrade** (Docker = image
  redeploy); **lifecycle cleanup** dry-run + confirm for expired images and
  aged trash.
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
curl -fsSL https://raw.githubusercontent.com/yixian-huang/imgli/main/scripts/install.sh | sh -s -- v0.6.0
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

Pin a release with `ghcr.io/yixian-huang/imgli:v0.6.0` (see
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
./imgli version     # git tag via ldflags, e.g. v0.6.0
./imgli serve       # → http://localhost:8686
```

## Configuration

Ops-level config only — everything product-facing (site name, registration
mode, groups, storage policies, SMTP, moderation) lives in the admin panel.

Precedence: defaults → YAML file (`imgli serve -config imgli.yaml`, see
[`deploy/imgli.example.yaml`](deploy/imgli.example.yaml)) → environment:

| Env | Default | Meaning |
|---|---|---|
| `IMGLI_LISTEN` | `:8686` | Listen address |
| `IMGLI_BASE_URL` | `http://localhost:8686` | Public base URL used in generated links |
| `IMGLI_DATA_DIR` | `./data` | Local storage + SQLite directory |
| `IMGLI_DATABASE_DRIVER` | `sqlite` | `sqlite` \| `postgres` |
| `IMGLI_DATABASE_DSN` | `<data_dir>/imgli.db` | DSN when using postgres |
| `IMGLI_TRUST_PROXY` | `false` | Trust `X-Forwarded-For` (behind a trusted reverse proxy only) |
| `IMGLI_FETCH_ALLOW` | *(empty)* | Extra hosts/CIDRs allowed for URL-fetch upload |

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

imgli upload -format markdown shot.png
imgli upload -format json -visibility private shot.png
cat shot.png | imgli upload -name shot.png -
```

Flags: `-base-url`, `-token`, `-format url|markdown|json`, `-visibility`,
`-expires-in`, `-name` (stdin filename).

### Doctor (self-host checks)

```bash
imgli doctor
imgli doctor -config /path/to/imgli.yaml
# exit 1 if any FAIL; WARN alone exits 0
```

Checks data directory writability, database connectivity, `base_url` shape,
`trust_proxy` guidance, listen address, and enabled **local** storage policies
(write/read/delete probe). See also [docs/security-hardening.md](docs/security-hardening.md).

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
- **Storage migrate (ops):** [docs/storage-migrate.md](docs/storage-migrate.md) — CLI + Admin job
- **Cleanup vs CDN:** [docs/ops-cleanup-cdn-boundary.md](docs/ops-cleanup-cdn-boundary.md)
- **OIDC troubleshooting:** [docs/oidc-operator.md](docs/oidc-operator.md)
- Migration into imgli: `imgli import-dir` (folder → upload API)
- Moderation operator path: [docs/moderation-spot-check.md](docs/moderation-spot-check.md)
- Public roadmap mirror: [ROADMAP.md](ROADMAP.md) (execution = GitHub Issues)
- Product site / demo: [imgli.com](https://imgli.com) · [img.li](https://img.li)
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
