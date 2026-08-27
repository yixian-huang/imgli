# imgli public roadmap (mirror)

> **Planning SSOT is internal** (`imgli/roadmap` in the project knowledge base).  
> This file is a **public, de-sensitized mirror** for contributors. Prefer GitHub Issues + Milestones for execution.

## Latest shipped — v0.9.17 · HEIC ingest · more `/t` widths

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.17

HEIC/HEIF uploads transcode to JPEG (then existing processing) on vips + libheif.
Pure-Go binaries return `415 heic_unsupported`. `/t?w=` accepts 120, 200, 240,
400, 480, 800, 960, 1600. See
[CHANGELOG](CHANGELOG.md#0917---2026-08-27).

## Previous — v0.9.16 · Admin thumbs NOT FOUND

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.16

Admin `/t` no longer draws `NOT FOUND` for public photos that still load in the
owner's library (CDN fallback + empty surface as public). Logged-in admins can
preview other users' private/pending thumbs. See
[CHANGELOG](CHANGELOG.md#0916---2026-08-20).

## Previous — v0.9.15 · MinIO/S3 first-load thumbs

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.15

First `/t` after upload on MinIO/S3 no longer returns JSON 500 (HEAD fallback to
GET; missing default thumb generated from the original). Image 5xx is SVG +
`no-store`. SPA surfaces retry once. See
[CHANGELOG](CHANGELOG.md#0915---2026-08-19).

## Previous — v0.9.14 · Lark/Feishu SMTP + mail copy

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.14

Lark/Feishu public-mailbox SMTP can be saved and tested (LOGIN auth, form-based
test send, explicit password-reenter). Operators can override zh/en subject and
body for the five transactional mails without editing HTML. See
[CHANGELOG](CHANGELOG.md#0914---2026-08-19).

## Previous — v0.9.13 · Upload drag-and-drop

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.13

One drop onto the upload area queues and uploads once; dragging out of the
page without dropping clears the overlay; moving inside the drop zone no
longer flickers the highlight. See [CHANGELOG](CHANGELOG.md#0913---2026-08-16).

## Previous — v0.9.12 · Private album privacy + origin serve cache

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.12

Private albums no longer leak to plaza / anonymous `/i` / `/s`; password images
stay off discovery; public covers must be public. Optional origin cache under
`{data_dir}/.serve-cache` (disable with `IMGLI_SERVE_CACHE_DISABLED`). First
boot runs schema v8 + surface rehome. See [CHANGELOG](CHANGELOG.md#0912---2026-08-15).

## Previous — v0.9.6 · Self-host robustness (SQLite / Docker / low-RAM)

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.6

Safer SQLite defaults (`mmap_size=0`, cache, temp_store, WAL→DELETE fallback);
Docker entrypoint fixes bind-mount ownership (uid 1000); libvips concurrency cap.
See [CHANGELOG](CHANGELOG.md#096---2026-08-06).

## Previous — v0.9.5 · Dark-mode tokens · site appearance

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.5

Dark-mode colors after Tailwind (`@theme inline`); admin **Appearance** tab —
accent, full-page background, scrim, frosted glass; layout/radius polish.
Post-tag ops: release scripts + CI split (see [ops-release](docs/ops-release.md)).
See [CHANGELOG](CHANGELOG.md#095---2026-08-06) and
[site customization IA](docs/design/site-customization-ia.md).

## Previous — v0.9.4 · Admin restore · processing WebP · Docker vips

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.4

Admin trash restore (single + batch); denser users table with expand details;
JPEG quality + original WebP (vips) processing; Docker image defaults to
libvips; System page imaging capability readout. See
[CHANGELOG](CHANGELOG.md#094---2026-08-05).

## Previous — v0.9.3 · Admin users ops · Tailwind UI

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.3

Admin users bandwidth/last-seen/sortable headers/icon confirms; frontend
migrated to Tailwind v4 with shared admin chrome. See
[CHANGELOG](CHANGELOG.md#093---2026-08-04).

## Previous — v0.9.2 · Instant reuse · site name · stats honesty

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.2

Same-user instant reuse (no library dup / double quota), site_name on nav
wordmark, storage policy live/trash object stats. See
[CHANGELOG](CHANGELOG.md#092---2026-08-04) and acceptance
[docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md](docs/superpowers/plans/2026-08-04-v0.9.2-acceptance.md).

## Previous — v0.9.1 · Admin groups UX polish

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.1

Groups save toasts, lifecycle badges (max vs force separate), collapsible form,
expiry UI in days. See [CHANGELOG](CHANGELOG.md#091---2026-08-03).

## Previous — v0.9.0 · Group lifecycle ops · stock clamp

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.9.0

User-group expiry/max-views caps, retention & force-max-age, stock lifecycle
preview/apply, admin batch trash/purge, cleanup kinds, docs/CLI. See
[CHANGELOG](CHANGELOG.md#090---2026-08-03) and
[docs/user-groups-lifecycle.md](docs/user-groups-lifecycle.md).

## Previous — v0.8.0 · Admin image ops · delete clarity

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.8.0

Admin storage locate (policy/driver/path), permanent purge vs trash, guest
auto-purge, clearer delete UX. See [CHANGELOG](CHANGELOG.md#080---2026-08-02).

## Previous — v0.7.x · Ops console · WebDAV · upgrade

- **v0.7.4** — WebDAV mount discovery on failed probe  
- **v0.7.3** — OpenList WebDAV read via 302  
- **v0.7.2** — One-click upgrade + admin shell UX  
- **v0.7.1** — Storage probe reliability  
- **v0.7.0** — Ops Console · Health · Deploy  

## Shipped earlier

- **v0.6.0** — Ops · Migrate · Trust (storage migrate jobs, version upgrade, lifecycle cleanup)  
- **v0.5.x** — Trust · Onboard · Community (R2 Verified, first-run Token, site ops, public ROADMAP)  
- **v0.4.x** — Storage Caps, FTP compatibility driver  
- **v0.3.0** — Password shares, public albums, width thumbs, import-dir, OIDC, webhooks  
- **v0.2.x** — CLI upload, doctor, EXIF strip, max views, light admin analytics  

## Next (not a committed schedule)

- **Community:** more S3-compatible vendors in the matrix (#51)  
- **Later candidates (internal SSOT):** single-instance team/org, fuller transform suite, more IdP connectors, async replicas / dual-write  

No open product milestone is required for community PRs — file an Issue or open a PR against `main`.

## Non-goals (do not open “white-label full” issues expecting Community)

Full BrandLockup replace, multi-tenant control plane, Open Core paywalls, video mainline, independent short-link product. See `COMMERCIAL.md` for dual-license inquiries.

## Contribute

- Bug / feature: GitHub Issues (templates under `.github/`)  
- S3 vendor reports: [`docs/s3-compatibility.md`](docs/s3-compatibility.md) + issue template  
- Security: [`SECURITY.md`](SECURITY.md)
