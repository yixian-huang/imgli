# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Product version comes from **git tags** (`vMAJOR.MINOR.PATCH`). Do not maintain a
separate version in `go.mod` or `web/package.json`.

## [Unreleased]

### Added

- **Storage migrate safety (M2):** process-local mutex per source policy (`ErrMigrateBusy`); disabled target returns `ErrMigrateTargetDisabled`; `MigrateResult.Progress()` / `RedactStoragePath` for admin-safe status (counts + redacted paths, no policy secrets). (#53)
- **Admin storage migrate jobs (M1):** `POST/GET /api/v1/admin/storage/migrate` with batch cursor resume, progress polling, and policies UI wizard (dry-run / delete-source / limit). CLI `storage-migrate` unchanged. (#54)
- **Admin version + update probe (U1):** `GET /admin/system/version` and operator-triggered `POST /admin/system/check-update` (GitHub `releases/latest`); dashboard shows running version. Build injects `internal/version.Version` via ldflags. (#56)

## [0.5.1] - 2026-07-31

Theme: **Patch — dark onboarding + storage-migrate docs**

### Fixed

- **Dark theme:** first-run Token onboarding card uses design tokens (readable on dark backgrounds). (#50)

### Added

- **Docs:** `docs/storage-migrate.md` (CLI cutover) + `docs/design/storage-migrate-sync-draft.md` (multi-policy migrate vs sync roadmap).

## [0.5.0] - 2026-07-31

Theme: **Trust · Onboard · Community**

### Added

- **Site ops:** optional `favicon_url`, `source_url` (AGPL corresponding source), `oss_credit` footer toggle, About page (`about_enabled` / `about_body`), `welcome_email` on register when SMTP configured.
- **Onboarding:** dismissible first-run Token path on upload page; auth scenario copy via `?from=` / `utm_campaign`.
- **Preferences:** `auto_copy_format=share` copies share page URL after upload.
- **Docs:** `docs/moderation-spot-check.md`, public `ROADMAP.md` mirror, README docs map.

### Changed

- **Document title:** custom `site_name` is primary (product brand only when still default img.li).
- **WebDAV:** Open falls back to full GET when HEAD lacks `Content-Length` or is unsupported; Range-ignored servers fall back to full-object buffer on mid-file read; clearer auth/status errors.
- **Docs:** `docs/webdav-compatibility.md` + `scripts/webdav-vendor-matrix.sh` (self-hosted live probe; no SaaS signup required for a matrix row).
- **Docs SSOT:** `docs/documentation-ssot.md` — layered truth (repo engineering vs product `docs-imgli` vs internal KB); CONTRIBUTING pointer.
- **Docs:** Cloudflare R2 matrix row marked **Verified** (live surface + Presign GET, 2026-07-31; post-tag docs commit on main).

## [0.4.1] - 2026-07-30

Theme: **FTP hot-path polish (compat)**

### Changed

- **FTP driver:** in-package control connection pool + remembered TLS mode (no dual-dial per request); still compat-tier, no serve changes.
- **FTP Open streaming:** use `SIZE` + lazy `RETR`/`REST` for better TTFB; full-buffer fallback when `SIZE` unsupported.

### Fixed

- **e2e:** policies admin assert `本地磁盘` with `exact: true` (Caps summary collision).

## [0.4.0] - 2026-07-30

Theme: **Storage Caps · FTP · Detail UX**

### Added

- **Storage Caps:** each storage policy exposes `tier` / `caps` / `effective` / `warnings` on admin API; policy UI shows capability panel and CDN/compat warnings (advisory; serve path unchanged).
- **FTP compatibility driver:** optional `ftp` backend (FTPS preferred, `allow_insecure` for plain FTP); no serve special-cases. Prefer OpenList/proxy — see `docs/storage-ftp.md` and dual-track notes in `docs/s3-compatibility.md`.
- **CDN domain validation:** `cdn_domain` must be http(s) origin/path prefix without userinfo/query/fragment.
- **Doctor:** storage caps / CDN-vs-cap / insecure transport / compat-only checks.
- **Design:** `docs/design/storage-caps-draft.md` + implementation checklist.
- **Detail modal density:** sticky header/actions; primary copy+QR first; access control & stats in collapsible `<details>`; body scroll lock + fixed modal height so pane scrolls (not the page behind). Redesigned access help and layout polish (#34).
- **e2e:** guest-off landing stays on `/` with login-gate; specs register via `/login` instead of expecting hard redirect.
- **Admin slots density:** sub-tabs (CTA / announcement / footer / HTML); zh|en side-by-side; schedule fields folded.
- **Site slots UI polish:** announcement bar soft strip (NOTICE kicker, chip CTA, dismiss control) aligned with Nav/Quota; footer links use equal-width `auto-fit` columns (not left-packed `auto-fill`) + centered meta; no brand billboard.
- **Locale-safe slots:** public config announcement/footer/`register_notice` accept `{zh,en}` maps (legacy string still OK); `pickLocale` in SPA. Regression tests in `SiteSlots.test.tsx` / `locale.test.ts`. Ops checklist: `docs/ops-deploy-checklist.md`.
- **Operator-owned public copy:** settings `help_url`, `upgrade_url`, `register_notice`, `share_branding` (`off|site|links`) on admin + public `/config` for help/upgrade CTAs (defaults empty). **Product brand kept:** SPA meta/title `img.li · 图鲤`, auth copyright 开源 imgli, share footer always shows OSS credit (imgli · 图鲤); `share_branding` only toggles instance name/links (default `site`).
- **Share UX:** detail modal lists share page URL first (public); upload success row copies share page; access password Generate + copy before save; clearer hint that existing  links become gated.
- **Ops helpers:** `scripts/ops-set-public-slots.py` (slots + public copy for img.li), `scripts/ops-patch-imglicom-home.py` (+ JSON); `docs/community/post-drafts-zh.md` for launch posts.

### Fixed

- **Detail modal scroll:** opening 访问控制 / 访问统计 no longer scrolls the page behind; body is fixed-locked and the right pane is a real overflow container.
- **Plaza switch lag:** admin `PutSettings` now uses the process-shared `settings.Service`, so `plaza_enabled` cache invalidates immediately (fixes e2e 404 race and up-to-30s stale enable).
- **CI unit/e2e:** default theme is `system` (store test); Nav quota `getAllByText(/GB/)`; e2e registers via `/login` and guest-off asserts login-gate on `/`.

## [0.3.0] - 2026-07-30

Theme: **Share · Migrate · Integrate**

### Added

- **OIDC login (generic):** admin GET/PUT `/admin/oidc`; `/auth/oidc/start` + callback; auto-provision user by email; `oidc_enabled` on public config.
- **Outbound webhooks:** admin GET/PUT `/admin/webhooks`; async `image.uploaded` / `image.moderated` with optional HMAC `X-Imgli-Signature`.
- **Admin users ops:** filter by signup `channel`, sort by `bandwidth`, CSV export `/admin/export/users.csv`.
- **Open Graph meta** on SPA `/s/{key}` and `/a/{id}` HTML shell for crawlers (passworded shares omit image).
- **Theme `system`**: cycle light → dark → system in the UI toggle.
- **CLI `imgli import-dir`:** bulk-import a local directory via upload API (recursive, dry-run, continue-on-error).
- **Public album visitor page:** `GET /api/v1/a/{id}` + `/a/{id}/images` and SPA `/a/:id` (public+normal images only).
- **Controlled thumbnail width query:** `GET /t/{name}?w=200|400|800` with disk cache keys under `.thumbs/w{N}/` (isolated from content-hash); invalid `w` → 400.
- **Access password for images:** optional password gate on `/i`/`/t` and share page; argon2 hash at rest; unlock via `POST /api/v1/s/{key}/unlock` cookie or `X-Image-Password`; no public CDN when set; detail UI to set/clear.

- **Admin ops dashboard (light analytics):** signup trend + coarse channels
  (`direct|invite|utm|referer`); traffic 7d/30d; referer Top with window toggle,
  suspect flag, host → top images; bandwidth period sum + top users; origin-only
  metering footnote.
- **Signup attribution (register-time only):** optional `utm_*` / `referer_host`
  on `POST /auth/register`; SPA passes URL UTM + `document.referrer` host.
- **Referer image rollup:** `referer_image_stats` + `GET /api/v1/admin/referers/images`.
- **Stats retention:** rolling 90-day purge of access/referer tables.
- **Ops docs:** `deploy/ops/admin-stats-metering.md` (CDN under-count boundary).
- **`imgli doctor`:** WARN when enabled policies set `cdn_domain` (dashboard ≠ CDN bill).

## [0.2.0] - 2026-07-29

Theme: **Workflow & Trust** — CLI/integrations, share landing, privacy
(EXIF strip, max views), ops (`doctor`, compose/backup docs), README demos.

### Fixed

- Plaza feed keyset cursor for sort `new` used nanosecond timestamps that did
  not match SQLite/GORM second-level comparisons, so the next page could repeat
  the previous boundary row.

### Added

- **CLI `imgli upload`:** multipart upload to `/api/v1/upload` via
  `IMGLI_BASE_URL` / `IMGLI_TOKEN` (or flags); file or stdin; output
  `url|markdown|json` (`internal/cliupload`).
- **Integrations docs:** ShareX custom uploader + sample `.sxcu`, uPic/PicList
  custom host mapping (`docs/integrations/`).
- **Settings API Token snippets:** curl / PicGo / ShareX / `imgli upload` CLI
  cards using public `base_url`; plain token only while create-once banner is
  open (`GET /api/v1/config` exposes `base_url`).
- **Public share page:** `GET /api/v1/s/{key}` + SPA `/s/{key}` for
  public+normal images (preview, dimensions, copy URL/Markdown); private /
  pending / rejected / expired → 404.
- **Upload success polish:** large primary URL + one-click copy, multi-format
  chips, and “open share page” for public uploads.
- **Strip EXIF/GPS on upload (default on):** site `processing.strip_exif`
  (nil/missing = on); re-encode JPEG/PNG before scale/watermark; admin toggle;
  content-hash after strip.
- **Max views / burn-after-read:** `max_views` + `views_served` on images;
  non-owner `/i` atomic claim; exhausted → 410; limited images skip public CDN
  302; upload/detail UI presets (1/3/10). Multi-instance needs shared DB.
- **`imgli doctor`:** self-host diagnostics for data dir, DB, base_url,
  trust_proxy, listen, and local storage policy probes (`internal/doctor`).
- **Ops docs:** production Compose example, Caddy/Traefik snippets, backup &
  restore guide (`deploy/compose.prod.example.yml`, `docs/backup.md`).
- **README demos:** product screenshots (upload / share / admin) and honest
  comparison table vs Lsky Pro / Chevereto (`docs/screenshots/`).
- **Monthly bandwidth hard cap (v1):** user-group `bandwidth_quota_month` (Free/default
  seed **5 GiB/month**, Asia/Shanghai calendar month); meter on successful `/i`/`/t`
  gate release by object size; block upload + 429 when exceeded; usage on
  `GET /user/quota`; admin group field; Nav/upload meters. See product decisions
  for scope (no CDN true-hit metering; hotlink still off by default).
- **Guest landing UX:** unauthenticated `/` stays on upload page with sign-in CTA
  when guest upload is off (no hard redirect to login only).
- **Auth `next` return:** login/register honors safe `?next=` (open-redirect safe)
  so users return to upload or the page they attempted.
- GitHub issue templates (bug / feature / S3 vendor report) and PR template.
- Docs: `docs/s3-compatibility.md` matrix stub and
  `docs/security-hardening.md` (private object storage / proxy checklist).
- **S4 slice:** refuse unauthenticated CDN URLs for `private/` object keys
  (`CDNEligibleObjectKey` + serve visibility/surface checks); operator probe
  `scripts/probe-private-object-anon.sh`.
- **Site slots (settings):** announcement bar, footer link groups, and
  admin-only HTML inject (`announcement` / `footer` / `html_inject` settings),
  exposed on public `/api/v1/config` and rendered in the SPA shell.

### Changed

- **License:** project default changed from MIT to **AGPL-3.0-only** to reduce
  closed SaaS / white-label freeloading of network-served modifications, while
  offering optional **commercial licenses** (see `COMMERCIAL.md`). Tags
  `v0.1.0` and `v0.1.1` remain MIT snapshots.
- CI/release workflows: `actions/checkout@v5`, `setup-go@v6`, `setup-node@v5`.

## [0.1.1] - 2026-07-29

### Added

- One-liner binary install script (`scripts/install.sh`) and Quick Start docs
  for Linux/macOS release assets.
- Automated GitHub Releases via GoReleaser (multi-platform binaries + checksums)
  and multi-arch Docker images on `ghcr.io/yixian-huang/imgli`.

## [0.1.0] - 2026-07-28

### Added

- Initial public release: single-binary image hosting with embedded React UI.
- Storage backends: local disk, S3-compatible, WebDAV; CDN `302` offload and
  presigned private serving.
- Pluggable moderation (NSFW + OCR sidecar), review queue, group policies.
- Accounts: groups/quotas, guest upload, invites, SMTP, albums, public gallery,
  recycle bin, image expiry.
- Upload API + API tokens; PicGo/Typora/VS Code guide.
- Bilingual UI (中文/English), PWA, dark mode, text watermark, admin audit logs.
- Docker Compose quick start and GitHub Actions CI (Go matrix, web, e2e smoke).

[Unreleased]: https://github.com/yixian-huang/imgli/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/yixian-huang/imgli/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/yixian-huang/imgli/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/yixian-huang/imgli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/yixian-huang/imgli/releases/tag/v0.1.0
