# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Product version comes from **git tags** (`vMAJOR.MINOR.PATCH`). Do not maintain a
separate version in `go.mod` or `web/package.json`.

## [Unreleased]

### Added

- **Durable storage migrate jobs:** Admin cross-policy migrate persists in the database. Restart recovers `pending`/`running` from `cursor_after_id`. Failed jobs can **Resume**; running jobs can **Cancel** (no rollback of already copied objects). Same source policy still busy-locks. CLI `storage-migrate` stays a synchronous foreground command.
- **Stock-group HEIC/HEIF (schema v9):** one-shot append of `heic`/`heif` when a group's `allowed_exts` is exactly the pre-0.9.17 default five (`png,jpg,jpeg,gif,webp`). Custom lists are unchanged. Seed still does not rewrite existing groups. `imgli doctor` WARNs when this build can decode HEIC but a group omits those extensions.
- **DCO on pull requests:** every human commit must include `Signed-off-by`. dependabot / github-actions are skipped.

## [0.9.17] - 2026-08-27

Theme: **HEIC ingest · more `/t` widths**.

### Added

- **HEIC / HEIF upload (vips + libheif):** decoded and stored as JPEG (then the existing processing pipeline, including optional original WebP). New default/guest groups allow `heic` and `heif`. Pure-Go binaries return `415 heic_unsupported`. Existing groups are unchanged.

### Changed

- **`/t?w=` whitelist:** `120`, `200`, `240`, `400`, `480`, `800`, `960`, `1600`. 200/400/800 keep working.

## [0.9.16] - 2026-08-20

Theme: **Admin thumbs NOT FOUND**.

### Fixed

- **Admin grid showed `NOT FOUND` for photos still visible in the owner's library:** public `/i` often 302s to CDN, but the admin grid always loads `/t`, which only opened origin keys. Empty `files.surface` also probed `private/.thumbs`. `/t` now treats empty surface as public and, on origin miss, fetches the same CDN object `/i` would redirect to, then generates the thumb.
- **Admin could not preview other users' private / pending / password / expired images:** `/i` and `/t` only treated the owner as privileged, so the admin grid rendered `PRIVATE IMAGE` / `IMAGE REMOVED`. Logged-in admins can now preview those originals and thumbs; anonymous access is unchanged.

## [0.9.15] - 2026-08-19

Theme: **MinIO/S3 first-load thumbnails**.

### Fixed

- **Broken library thumbs after upload on MinIO/S3:** the first `GET /t/{key}.jpg` could return JSON 500 (HEAD 403/503 or missing Content-Length right after PUT). `<img>` then showed the browser broken-image icon until a full refresh. `Open` now falls back to GET; a missing default thumb is generated from the original (same 400px edge as upload).
- **`/t` `/i` 5xx for browsers:** image requests get an SVG placeholder with `Cache-Control: no-store` instead of a JSON envelope, so reverse proxies should not cache the error as a JPEG.

### Changed

- Library, admin, share, albums, plaza, and avatar `<img>` tags retry once with `?r=1` after a load error.
- A successful upload invalidates the library and admin image lists (no full page reload required).

## [0.9.14] - 2026-08-19

Theme: **Lark/Feishu SMTP + customizable mail copy**.

### Added

- **Mail copy overrides:** Admin → Email SMTP can edit zh/en subject and body for welcome, verify, reset, change-email, and reject notices. Layout, button, and copyable link stay built-in. Empty fields keep the stock text. Placeholders: `{{site_name}}` `{{link}}` `{{image_name}}` `{{image_key}}`. Preview and “send this template” use the current form (no need to save first). Button color follows `theme_accent`.
- **SMTP AUTH LOGIN:** when the server advertises LOGIN but not PLAIN (common on Lark/Feishu and some 163/QQ endpoints), imgli authenticates with LOGIN instead of only PLAIN.

### Fixed

- **Could not save SMTP after changing username:** a masked password plus a new host/username was rejected as a generic “port/encryption/from invalid” error. The form now clears the mask, asks to re-enter the password, and the API says so explicitly. Host/username/from are trimmed.
- **Test send ignored the form:** “Send test email” used only the last saved config, so filling a Lark username then testing produced `530 5.7.1 Authentication required`. Tests now send the form values; empty From falls back to username.
- **Opaque SMTP errors:** 530/535, TLS/port mismatch, timeout, DNS, and sender-rejected failures are mapped to actionable Chinese (and English toasts still surface the server text when useful). Validation errors for port, encryption, From, and “none + username” are split.

### Changed

- Choosing SSL on the default port 587 switches to 465 (and STARTTLS on 465 switches to 587). Custom ports are left alone; 465+STARTTLS / 587+SSL show an inline warning.
- SMTP field hints cover Lark/Feishu public mailboxes (full address + IMAP/SMTP password, not the login password).

## [0.9.13] - 2026-08-16

Theme: **Upload drag-and-drop correctness**.

### Fixed

- **Duplicate drag-and-drop uploads:** dropping an image onto the upload area no longer creates duplicate queue entries or upload requests.
- **Stuck / flickering drag overlay:** dragging out of the upload page without dropping now clears the prompt; moving among elements inside the drop zone no longer clears the drop-zone highlight.

## [0.9.12] - 2026-08-15

Theme: **Private album privacy + origin serve cache**.

### Added

- **Origin serve cache:** public `/t` and streamed `/i` responses are cached under `{data_dir}/.serve-cache` (default 512MiB) so repeat hotlinks do not re-fetch S3 every time. Disable with `serve_cache_disabled` / `IMGLI_SERVE_CACHE_DISABLED`.

### Fixed

- **Private albums leaking into plaza:** plaza and public profile feeds now require the parent album to be public (in addition to `list_in_plaza`). Uploading a public image into a private album no longer lists it on the square.
- **Private album images stayed public on `/i`:** upload, move-in, and setting an album private now force image visibility to `private` (anonymous direct links 401). Existing leaked rows are repaired on migrate (schema v8). Bulk-set-public is rejected on private albums. PATCH / batch cannot flip an image back to public while it remains in a private album.
- **Private visibility without object rehome:** album bulk-set-private and schema v8 previously only updated the visibility column, leaving objects on `public/`. Bulk updates now go through `imagesvc` (copy + rehome). Startup also repairs live rows whose `files.surface` still mismatches `images.visibility`.
- **Password-gated images on plaza:** images with an access password are excluded from plaza and public profile feeds (`/t` would 401).
- **Share page ignored parent album:** `/s` and share OG now 404 when the image sits in a private album, even if the row is still marked public.
- **Public album covers leaked private keys:** plaza album cards and `/a` OG only use publicly displayable covers; setting a private or password-gated cover on a public album is rejected.
- **Public instance stats counted private images:** homepage `live_image_count` now counts only unexpired public images without an access password.

## [0.9.11] - 2026-08-12

Theme: **Public homepage stats + album gallery click preference**.

### Added

- **Public instance stats (optional, default off):** setting `public_stats` controls whether the guest homepage shows coarse local metrics (uptime days from a configurable `since` date, live image count, optional user count / used bytes). Exposed on `GET /api/v1/config` as a computed snapshot (60s process cache). Self-host stays unchanged until an operator enables it in Admin → 站点插槽.
- **Album share click behavior:** per-album `click_to_immersive` (default on). When off, gallery thumbnails open the image share page instead of immersive; visitors can still enter immersive via toolbar / hero. Configure next to default view in album settings → share.

### Changed

- **Guest landing layout:** clearer hierarchy for announcement, instance stats strip, primary upload CTA, and secondary login when guest upload is off.

## [0.9.10] - 2026-08-10

Theme: **Stability patch — thumbnail memory, watermark/dedup, HTML inject scripts**.

### Fixed

- **Thumbnail OOM / cgroup thrash:** pure-Go `/t` and upload thumbs now cap source bytes (24MiB) and decode pixels (16MP), limit concurrent full-frame decodes (`IMGLI_THUMB_CONCURRENCY`, default 1), and skip oversized `?w=` generation instead of decoding multi-hundred-MB bitmaps. Ops: `MemoryHigh`/`GOMEMLIMIT` guidance in unit example; `health-check.sh` monitors cgroup memory + optional auto-restart.
- **Text watermark incomplete / tofu (口口):** shrink font iteratively so padded outline fits the canvas (avoids clipping last glyphs); pin baseline inside the layer on tiny canvases so ink still lands (hash changes / no false instant); admin save rejects watermark text with glyphs missing from the embedded Noto Sans SC subset.
- **Same image re-upload with expiry:** content-hash reuse compared absolute `ExpiresAt` by exact second; `now+expires_in` (and group default expiry) drifted across uploads so the same photo created multiple library rows. Reuse now tolerates ≤2 minutes skew.
- **Custom HTML inject scripts never ran:** SPA mounted inject markup via `innerHTML`/`cloneNode`, so analytics `<script>` entered the DOM but did not execute or fetch. Scripts are re-created with `document.createElement('script')` so inline and external tags run.

## [0.9.9] - 2026-08-08

Theme: **Storage policy operator UX — path template, CDN / path-style guidance**.

### Added

- **Storage path template tokens:** `{H}{M}{S}{ms}`, `{rand}`/`{rand:N}`, `{hex:N}`/`{HEX:N}`, `{digits:N}` with length floors; create/update rejects templates without a random segment.
- **Admin storage policy UX:** field hints (endpoint / region / CDN / prefix / path style / template), live example object key preview, path-template presets (default / flat / to-second / digits), S3 vendor example chips (OSS / COS / R2 / MinIO).
- **`imgli doctor`:** WARN `path_style_vendor` when an enabled S3 policy uses path-style on typical public-cloud endpoints.
- **Test connection:** path-style + public-cloud endpoint appends a virtual-host remediation hint on Put probe failure; failure text also pinned under the button.

### Changed

- **S3 prefix:** non-empty `prefix` is normalized with a trailing `/` on save; admin form reloads server-normalized config after create/update.
- **CDN domain errors:** bare hostnames (common OSS endpoint paste) return an explicit “must be http(s)” message; detailed validation messages prefer surfacing in toasts over generic `invalid_request`.
- **Path-style advisory:** S3 policies with `path_style=true` on typical public-cloud endpoints emit `path_style_vendor` warning (does not block save).
- **Docs:** [s3-compatibility.md](docs/s3-compatibility.md) storage policy fields section; README / README.zh-CN link; surface-prefix cross-link in [security-hardening.md](docs/security-hardening.md).

## [0.9.8] - 2026-08-08

Theme: **Album management — settings modal, batch rename pipeline, share stats & plaza listing**.

### Added

- **Album owner tools:** description, manual cover, visitor access password (gates `/a` only), `list_in_plaza`, album PV stats (30-day chart), bulk set all images public/private, reorder via `album_pos`, settings modal (share / content / stats tabs).
- **Public discovery of albums:** `GET /plaza/albums`, `GET /u/{username}/albums`; Explore and public profile **Albums** tabs.
- **Batch rename pipeline:** optional find/replace (multi-keyword, case-insensitive, clean separators) then optional template (`{name}` `{original}` `{n}`/`{n:03}` `{yyyy}{mm}{dd}` `{ext}` `{album}`); start index; skip unchanged; in-batch conflict detection; full preview (only-changed filter).
- **Album password unlock:** `POST /a/{id}/unlock` + cookie; public images list requires unlock when password set.

### Changed

- **Album detail UI:** settings/stats moved into modal so the page stays image-first (select all, order, cover, batch bar).
- **Plaza eligibility:** images in albums with `list_in_plaza=false` are excluded from plaza feed; uncategorized public images unchanged.
- **Public album image order:** respects `album_pos` (then id).

### Fixed

- **BatchBar Hooks order:** move `useMemo` above early return when selection is empty.

## [0.9.7] - 2026-08-08

Theme: **Public album share dual-mode · solid page background · plaza opt-in clarity**.

### Added

- **Public album share (`/a/:id`):** magazine hero, masonry gallery, cinematic immersive viewer (fullscreen, palette letterbox, filmstrip, zoom/swipe), owner `default_view` (`gallery`|`immersive`), deep links `?view=immersive&i=N`, infinite scroll + preload.
- **Album default view:** owners set default visitor mode on album detail; visitors follow URL first, then owner default (no localStorage preference).
- **Solid page background:** admin Appearance `theme_bg_color` (#RGB/#RRGGBB) → soft gradient wash under optional photo background + scrim.
- **Plaza / public dual opt-in guidance:** upload, profile, and explore copy clarify that plaza listing needs both public image visibility and public profile.
- **Release install smoke:** `scripts/ops-smoke-install.sh` plus release jobs `smoke-binary` / `smoke-docker` — pull published Release binary or Docker image, start fresh, check SPA/healthz, register first user, upload, GET object; Docker covers named volume and bind mount.

### Changed

- **Visitor album link / copy feedback:** public album copy uses shared toast path; share/album footers unified via `ShareBrandFooter`.
- **Segmented control:** selected chip text/background contrast hard-locked via CSS variables (`data-segmented-active`) so labels stay readable on accent.
- **Public album structure:** extract hooks (`usePublicAlbum`, `useAlbumViewMode`), hero/masonry components, immersive icons/fullscreen helper, and shared `albumLinks` types.

### Fixed

- Public album visitor links that previously failed silently when copy/open needed feedback.
- Nested invalid interactive markup on album share surfaces (button-in-anchor style issues).

## [0.9.6] - 2026-08-06

Theme: **Self-host robustness — SQLite / Docker bind mounts / low-RAM**.

### Fixed

- **SQLite OOM / low-RAM:** default connection sets `mmap_size=0`, modest `cache_size`, `temp_store=FILE`; runtime pragmas also applied when a custom DSN omits them.
- **SQLite open failures:** probe `data_dir` writability with Docker uid-1000 permission hints; WAL open failure falls back to `journal_mode=DELETE`.
- **Docker bind mounts:**
- **e2e:smoke:** `guest.spec` bootstraps first admin when run as the first smoke file (before `admin`/`main` register `boss`). entrypoint runs as root only to `chown` data dir / SQLite files to `imgli` (1000), then `su-exec` drops privileges (fixes root-owned host paths).

### Changed

- **libvips:** default concurrency capped at 2 (override with `VIPS_CONCURRENCY`); process cache bounded (~64 entries / 64MiB). Image `VIPS_CONCURRENCY=2` in Dockerfile.
- **Compose / docs:** named volume vs bind mount notes; product config docs clarify required env vars.
- **CI/CD (main):** concurrency cancel; e2e split; release job split; ops scripts (`pre-tag-check`, `ops-deploy-baili`, `ops-smoke-public`).

## [0.9.5] - 2026-08-06

Theme: **Dark-mode token fix · site appearance (accent, background, glass)**.

### Fixed

- **Dark mode after Tailwind migration:** semantic colors use `@theme inline` so `body[data-theme=dark]` flips `text-ink` / `bg-surface` / buttons at runtime (was frozen to light values on `:root`).
- **Page title padding:** `PageHeader` keeps internal `px`/`py` without widening the strip past content (layout contract + unit test).

### Added

- **Site appearance (L2):** admin **Appearance** tab — `theme_accent` (#RGB/#RRGGBB → primary buttons), `theme_bg_image_url` (full-page background), `theme_bg_dim` (0–1 scrim), `theme_glass` (0–1 frosted panel opacity). Exposed on public `GET /api/v1/config`; applied via CSS variables on `body`.

### Changed

- **With background image:** frosted glass panels (`backdrop-filter`), muted text contrast boost, borders fade with `theme_glass` (softer when opacity is low).
- **Admin UI:** unify panel radii to `rounded-sm`; admin title strip bleeds under chrome with matching gutters; app main gutters slightly wider (`px-8`).

## [0.9.4] - 2026-08-05

Theme: **Admin trash restore · users table density · processing WebP · Docker vips**.

### Added

- **Admin trash restore:** `POST /api/v1/admin/images/{key}/restore`; batch `action: restore`; list hover, detail, and batch bar restore controls (audit `image_admin_restore`).
- **Image processing — JPEG quality:** `processing.jpeg_quality` (0 → default 90; else 1–100) for re-encode on strip/scale/watermark (keep path).
- **Image processing — original WebP:** `output_format` (`keep`|`webp`), `webp_quality` (0 → 80), `webp_skip_if_larger` (default on). Single-decode / final-encode pipeline; GIF/WebP inputs unchanged. Enabling WebP requires a build with libvips encode.
- **Processing capabilities:** settings expose `processing_capabilities.webp_encode`; System health exposes `imaging_backend`, `webp_encode`, `thumb_ext`.
- **Docker default libvips:** release `Dockerfile` builds with `-tags vips` and ships runtime `vips` (WebP thumbnails + optional original→WebP).

### Changed

- **Admin users table:** denser layout — merged usage column (storage + bandwidth), expandable row for image count / registered / last seen; sort select for date fields; lower min table width.
- **Admin sticky headers:** PageHeader solid isolation + shadow; main `scroll-pt`; settings tabs sticky polish (title stays pinned).
- **Docs / install pins:** README, ROADMAP, Goreleaser notes clarify pure-Go GitHub archives vs Docker+vips; compose examples note vips image.

### Fixed

- Admin images trash scope previously only offered permanent delete; restore path is now available end-to-end.

## [0.9.3] - 2026-08-04

Theme: **Admin users ops · full Tailwind UI**.

### Added

- **Admin users table:** monthly bandwidth + period, last-seen (latest Web session), registration time; sortable column headers (`sort=bandwidth|storage|created|last_seen`); icon actions with two-click confirm for ban/unban/reset password.
- **Tailwind CSS v4** frontend stack (`tailwindcss` + `@tailwindcss/vite`, `clsx` / `tailwind-merge`); design tokens mapped via `@theme` in `web/src/styles/tokens.css`.
- **Shared admin chrome:** `adminChrome.tsx` (filters, search, select, table/sort headers, status pills, icon actions); admin settings class map `settingsUi.ts`.

### Changed

- **UI rewrite:** all product and admin pages/shells/primitives use Tailwind utilities; CSS Modules removed from `web/src`.
- Users list layout: aligned columns, footer page stats, CSV export unchanged.

## [0.9.2] - 2026-08-04

Theme: **Instant reuse · site name surface · stats honesty**.

### Fixed

- **Same-user instant upload:** re-uploading the same content with the same options returns the existing live image (`reused: true`) — no duplicate library row, no double quota charge. Different options still create a new link on the shared file; cross-user and guest uploads still get their own keys.
- **Site name in chrome:** configured `site_name` drives the nav/auth/guest/discover/admin/share wordmark (carp mark kept). Document title already used it; settings now explain the surface.
- **Storage policy counts:** list shows object count + used bytes; detail adds live/trash image split and a note that objects stay until hard purge. Link to admin images filtered by policy.

### Changed

- Upload copy: instant vs reused hints distinguish “new link, content deduped” vs “library hit, original link”.
- Quota tooltip and dashboard labels clarify storage includes trash; image counts are live-only.
- Docs: acceptance cases and site-customization IA (`docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md`, `docs/design/site-customization-ia.md`).

## [0.9.1] - 2026-08-03

Theme: **Admin groups UX polish**.

### Fixed

- **Groups save feedback:** toast on create/save/no-op (no silent save).
- **Lifecycle list badges:** `max_expires_in` and `force_max_age_days` shown separately (no longer min-collapsed into a single `≤7d` when force is shorter).

### Changed

- **Groups form layout:** collapsible sections (quota, rate, formats, lifecycle, policies, stock).
- **Expiry fields in UI:** default/max expiry entered in **days** (API still seconds).

## [0.9.0] - 2026-08-03

Theme: **Group lifecycle ops · admin stock clamp · cleanup observability**.

### Added

- **Admin images batch:** `POST /admin/images/batch` `{keys, action: trash|purge}` (max 100); list multi-select + batch bar; per-card permanent delete on hover.
- **User-group lifecycle / upload options:** `default_expires_in`, `max_expires_in`, `default_max_views`, `max_max_views`, `retention_days`, `force_max_age_days` on groups; enforced at upload + image PATCH; exposed on `/user/quota` and guest `/config`; admin Groups UI; hourly soft-delete by retention and hard purge by force max age. Guest seed defaults: 1d default / 7d max / 7d force age.
- **Image detail access presets:** library detail modal filters expiry / max-views Segmented options by the same group caps as the upload page; hides “remove expiry” when permanent is forbidden; out-of-policy banner + apply group max; dynamic cap presets.
- **Group stock lifecycle:** `POST /admin/groups/{id}/lifecycle/preview|apply` clamps permanent/over-cap live images to now+cap; Groups UI preview/apply + list badges + stock-only warning.
- **Cleanup kinds:** `group_retention`, `group_force_age` in admin cleanup preview/run (System page includes them by default).
- **Error codes:** `expires_over_group`, `max_views_over_group` with i18n mapping.
- **Review queue:** purge-all-on-page (permanent delete batch).
- **Docs:** [docs/user-groups-lifecycle.md](docs/user-groups-lifecycle.md); PicGo/ShareX/CDN cleanup notes; CLI `imgli upload -verbose` prints group limits.

## [0.8.0] - 2026-08-02

Theme: **Admin image ops · delete clarity** — storage locate, trash vs permanent delete, guest purge.

### Added

- **Admin image storage locate:** list/detail expose `policy_id`, `policy_name`, `policy_driver`, `surface`, and object `path` (copy in detail) so operators can find WebDAV/S3/local objects.
- **Admin permanent delete:** `DELETE /api/v1/admin/images/{key}?permanent=1` hard-deletes DB rows and enqueues `delete_file` for storage cleanup; response includes `physical_queued` / `object_retained` (instant-upload shared refs).
- **Admin trash scope:** `GET /api/v1/admin/images?deleted=live|trash|all` (default live); UI filter + trash badge.
- **Guest upload delete:** guests have no owner trash — admin default delete is permanent purge.
- **Audit:** `image_admin_purge` with owner/policy/path and physical-delete flags.

### Fixed

- **Library delete two-click UX:** card/list quick-delete arms with a visible **确认 / OK** label (was easy to miss as “no reaction”).
- **Trash cache after soft-delete:** user delete / batch delete invalidates the trash query so the recycle bin updates immediately.
- **AdminPurge race:** soft-delete-then-restore race no longer returns a silent success without purging.

### Changed

- User-facing copy treats soft-delete as **move to trash** (card, batch bar, toasts); detail already used “add to trash”.
- Admin list hover: clearer trash vs permanent labels; success toasts for soft vs hard delete (including shared-object retained / queue failure).
- Whitelist armed button shows a short confirm label.

## [0.7.4] - 2026-08-01

Theme: **WebDAV mount discovery on failed probe**.

### Added

- **WebDAV test-connection mount discovery (P0/P1):** when the write probe fails, imgli PROPFIND Depth:1 lists child collections and write-probes each (max 8), then appends copy-paste endpoint suggestions (e.g. OpenList virtual `/dav` → `…/dav/<mount>`). If discovery finds nothing, a short OpenList virtual-root hint is still shown.

## [0.7.3] - 2026-08-01

Theme: **OpenList WebDAV read via 302**.

### Fixed

- **WebDAV Open/Exists against OpenList (and similar netdisk proxies):** write could succeed while "Test connection" failed on read-back because the peer returns **302** to a presigned object URL, and **HEAD on that URL often 403**. imgli now treats HEAD 302 as "exists / use buffered GET", follows **GET** redirects only (strips Basic auth), and leaves PUT/DELETE unfollowed. Verified against a live OpenList mount that fronts China Mobile EOS.

### Changed

- Clearer PUT 404 wording when the path is missing or the WebDAV root is not writable (e.g. OpenList virtual `/dav` root).

## [0.7.2] - 2026-08-01

Theme: **One-click upgrade + admin shell UX**.

### Fixed

- **One-click binary upgrade under systemd:** preflight checks that the binary directory is writable; clear error when `ProtectSystem=strict` leaves the bin path read-only (was a silent production failure). Successful upgrades **re-exec** the new binary so the version changes without a manual restart.
- **Update check:** fall back from HTTP `HEAD` to `GET` when resolving GitHub `releases/latest` (some networks omit `Location` on HEAD).
- **Doctor:** new `binary_upgrade` check reports whether in-place upgrade can write next to the running executable.

### Changed

- **Admin layout:** fixed viewport shell — top header stays put, left nav stays put, only the main content scrolls; page title + filter row (`PageHeader`) sticky within the content pane.
- **Ops docs:** `deploy/imgli.service.example` documents `ReadWritePaths` for both data dir and binary dir (required for admin upgrade).

## [0.7.1] - 2026-08-01

Theme: **Storage probe reliability** — fix local test-connection path and clearer remote probe errors.

### Fixed

- **Local storage “Test connection”:** probe now resolves `config.root` with the same rules as real uploads and doctor (`storage.LocalRoot` under `data_dir`; absolute roots unchanged). Fixes Docker/non-root false failures like `root 不可写: mkdir uploads: permission denied` when `/data/uploads` is actually writable.
- **WebDAV/S3/FTP probe key:** write probe objects under `imgli-probe/…` so WebDAV exercises parent `MKCOL` (closer to real upload paths; more friendly to OpenList-style servers).
- **Probe error messages:** one readable sentence with path or endpoint plus a short hint for common cases (permission, auth, unreachable, 404); audit stores `error` text; avoid noisy structured payloads.

### Changed

- Admin `UseDataDir` wires `cfg.DataDir` into policy test probes so local relative roots match production layout.

## [0.7.0] - 2026-08-01

Theme: **Ops Console · Health · Deploy** — admin-visible self-host diagnostics, reverse-proxy clarity, unified three-step setup UI.

### Added

- **Admin system health (H1/H2):** `GET /api/v1/admin/system/health` returns doctor checks (same as CLI `imgli doctor`) plus read-only runtime summary (`base_url`, `trust_proxy`, listen, install shape, request Host / forwarded headers). (#74, #75, #80)
- **Admin System / Ops page:** health table, browser vs `base_url` mismatch banner (reverse-proxy CSRF), first-run checklist, version upgrade with preflight notes, lifecycle cleanup UI, links to migrate/backup. Nav **系统 / 运维**. (#74–#78, #80)

### Changed

- **Onboarding UI:** shared `StepGuide` for upload first-run and Settings → API Token “three-step setup” (console design language: mono kicker, numbered steps, shared buttons). (#81)
- **Docs:** reverse-proxy CSRF FAQ in README / security-hardening and product FAQ (docs.imgli.com); ROADMAP points at v0.7.0 milestone.

## [0.6.0] - 2026-07-31

Theme: **Ops · Migrate · Trust** — admin storage migrate jobs, lifecycle cleanup, version probe/upgrade.

### Added

- **Storage migrate safety (M2):** process-local mutex per source policy (`ErrMigrateBusy`); disabled target returns `ErrMigrateTargetDisabled`; `MigrateResult.Progress()` / `RedactStoragePath` for admin-safe status (counts + redacted paths, no policy secrets). (#53)
- **Admin storage migrate jobs (M1):** `POST/GET /api/v1/admin/storage/migrate` with batch cursor resume, progress polling, and policies UI wizard (dry-run / delete-source / limit). CLI `storage-migrate` unchanged. (#54)
- **Docs:** `docs/storage-migrate.md` documents Admin migrate path, API, and operator acceptance sketch. (#55)
- **Admin version + update probe (U1):** `GET /admin/system/version` and operator-triggered `POST /admin/system/check-update` (GitHub `releases/latest`); dashboard shows running version. Build injects `internal/version.Version` via ldflags. (#56)
- **Admin one-click binary upgrade (U2):** `POST /admin/system/upgrade` with `confirm=true`, checksum-verified GitHub Release asset, in-place binary replace + restart guidance; Docker/container installs refuse binary replace. (#57)
- **Storage migrate filters + size verify (M3):** optional `user_id` / created time window; post-Put size check blocks silent policy flip on mismatch. (#58)
- **Admin lifecycle cleanup (L1):** `POST /admin/cleanup/preview` and `POST /admin/cleanup/run` (confirm required) for expired images and old trash; dry-run samples image keys. (#59)
- **Docs (P2):** cleanup vs CDN boundary, OIDC operator troubleshooting, migrate estimate note. (#61, #62, #63)

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

[Unreleased]: https://github.com/yixian-huang/imgli/compare/v0.9.17...HEAD
[0.9.17]: https://github.com/yixian-huang/imgli/compare/v0.9.16...v0.9.17
[0.9.16]: https://github.com/yixian-huang/imgli/compare/v0.9.15...v0.9.16
[0.9.15]: https://github.com/yixian-huang/imgli/compare/v0.9.14...v0.9.15
[0.9.14]: https://github.com/yixian-huang/imgli/compare/v0.9.13...v0.9.14
[0.9.13]: https://github.com/yixian-huang/imgli/compare/v0.9.12...v0.9.13
[0.9.12]: https://github.com/yixian-huang/imgli/compare/v0.9.11...v0.9.12
[0.9.11]: https://github.com/yixian-huang/imgli/compare/v0.9.10...v0.9.11
[0.9.10]: https://github.com/yixian-huang/imgli/compare/v0.9.9...v0.9.10
[0.9.9]: https://github.com/yixian-huang/imgli/compare/v0.9.8...v0.9.9
[0.9.8]: https://github.com/yixian-huang/imgli/compare/v0.9.7...v0.9.8
[0.9.7]: https://github.com/yixian-huang/imgli/compare/v0.9.6...v0.9.7
[0.9.6]: https://github.com/yixian-huang/imgli/compare/v0.9.5...v0.9.6
[0.9.5]: https://github.com/yixian-huang/imgli/compare/v0.9.4...v0.9.5
[0.9.4]: https://github.com/yixian-huang/imgli/compare/v0.9.3...v0.9.4
[0.9.3]: https://github.com/yixian-huang/imgli/compare/v0.9.2...v0.9.3
[0.9.2]: https://github.com/yixian-huang/imgli/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/yixian-huang/imgli/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/yixian-huang/imgli/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/yixian-huang/imgli/compare/v0.7.4...v0.8.0
[0.7.4]: https://github.com/yixian-huang/imgli/compare/v0.7.3...v0.7.4
[0.7.3]: https://github.com/yixian-huang/imgli/compare/v0.7.2...v0.7.3
[0.7.2]: https://github.com/yixian-huang/imgli/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/yixian-huang/imgli/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/yixian-huang/imgli/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/yixian-huang/imgli/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/yixian-huang/imgli/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/yixian-huang/imgli/compare/v0.4.1...v0.5.0
[0.3.0]: https://github.com/yixian-huang/imgli/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/yixian-huang/imgli/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/yixian-huang/imgli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/yixian-huang/imgli/releases/tag/v0.1.0
