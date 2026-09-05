# v0.9.17 · HEIC 入库 + `/t?w=` 白名单扩展

**Status:** draft for review  
**Version:** `v0.9.17` (patch; not a new minor milestone)  
**Date:** 2026-08-27  

Approved in conversation: HEIC ingest as the user-visible change; transform limited to expanding `/t?w=` (option A). No serve-time `/i` format rewrite. No S3 vendor matrix in this release.

## 1. Goal

1. iPhone camera-roll **HEIC / HEIF** uploads succeed on the official Docker / vips build, and produce a browser-embeddable original.
2. `/t/{key}?w=` accepts a slightly larger **fixed** width set. Existing 200 / 400 / 800 keep working.

One-sentence: **decode HEIC at upload, store JPEG (or WebP if processing says so); let thumbs request a few more widths.**

## 2. Non-goals

- Store or serve HEIC bytes (`/i/{key}.heic` does not exist).
- On-demand `/i/{key}.webp` or `/i?w=` derived originals (CDN 302 stays original-object only).
- AVIF encode, crop, filters, arbitrary `w`.
- Auto-append `heic`/`heif` to **existing** user-group `allowed_exts`.
- HDR / 10-bit preservation, HEIC image sequences, Live Photo `.mov`.
- Welcome-mail / site copy pack / OSS-COS matrix.
- Proprietary cloud SDKs. imgli still speaks local / S3 API / WebDAV / FTP-compat; odd backends stay behind OpenList.

PicGo / `data.links.url` **shape unchanged** (only the stored extension becomes `jpg` or `webp`).

## 3. HEIC ingest

### 3.1 Placement

Insert **before content-hash** in `upload.Service.Save` (same moment as today's `burn`). Instant reuse and quota then key off **stored** JPEG/WebP bytes, not the camera file.

```
probe → allowlist(original heic|heif)
     → if HEIC family: transcode to JPEG (or fail)
     → existing burn (scale / watermark / strip EXIF / optional WebP)
     → hash → instant / put
```

### 3.2 Detection

Do **not** trust the filename alone. Sniff ISO-BMFF `ftyp` brands, including at least:

`heic`, `heix`, `hevc`, `hevx`, `heim`, `heis`, `hevm`, `hevs`, `mif1`, `msf1`, `heif`

- Filename `.heic` / `.HEIC` / `.heif` / `.HEIF` maps to allowlist ext `heic` or `heif` (lowercased).
- Magic-only (no/unknown suffix) uses allowlist ext `heic`.
- `heic` and `heif` are **not** aliases: a group that lists only `heic` rejects `.heif`. New seeds include **both**.

### 3.3 Transcode (vips + HEIF loader)

When `HeicDecodeAvailable()` is true:

1. Decode with libvips.
2. `autorot` so EXIF orientation is in pixels (otherwise `/i` is sideways in browsers).
3. Encode JPEG at `processing.EffectiveJPEGQuality()` (0 → 90).
4. Write back the temp file. `Probe` again. `images.ext` after a successful keep-path upload is `jpg`.
5. Then call existing `burn` with ext `jpg` / `jpeg`. If `output_format=webp` and WebP encode is available, stored ext becomes `webp` as today.

Apply the same pixel cap as other uploads (`MaxDimension` / decode budget). Oversize → existing `ErrDimensionOver`, not a HEIC-specific code.

### 3.4 `HeicDecodeAvailable`

**Runtime**, not merely `-tags vips`.

| Build | Loader present | Result |
|-------|----------------|--------|
| Pure Go (GitHub Release archives, `make build`) | n/a | `false` |
| `-tags vips` without a HEIF loader | no | `false` |
| Official Docker after this spec | yes | `true` |

Mirror `WebPEncodeAvailable`: expose on admin health / processing capabilities (`heic_decode`). Frontend and doctor can show “needs Docker / libheif”, not a silent `invalid image`.

Probe of a HEIC file on `false` must return a **distinct** error (`ErrHeicUnavailable`), not `ErrUnsupported` / `ErrInvalidImage`.

### 3.5 Errors

| Situation | HTTP | `code` | Message (zh; en in i18n) |
|-----------|------|--------|---------------------------|
| HEIC/HEIF sniffed, decoder unavailable | 415 | `heic_unsupported` | 当前构建无法解码 HEIC，请使用官方 Docker 镜像或 `make build-vips`（需 libheif） |
| Group `allowed_exts` lacks `heic`/`heif` as matched | 415 | `ext_not_allowed` | unchanged |
| Corrupt payload / vips load fail after sniff | 400 | `invalid_request` | 不是有效的图片文件 |
| Live Photo video / non-image | 415 or 400 | `ext_not_allowed` or invalid | not an image path |

`heic_unsupported` is **not** retryable in the upload queue.

### 3.6 Names, links, reuse

- `images.name` may remain `IMG_1234.HEIC`.
- `images.ext` / object key extension / `links.url` use `jpg` or `webp` (never `heic`).
- Default `/t` and `?w=` run on the stored raster; serve path never opens HEIC.
- Same HEIC re-uploaded with the same processing settings is deterministic enough for content-hash instant reuse (fixed JPEG Q + autorot).

### 3.7 Groups and CLI

- **New** default + guest group seeds: append `heic`, `heif` to `["png","jpg","jpeg","gif","webp"]`.
- **Existing** rows: no migration rewrite of `allowed_exts` in v0.9.17. **v0.9.18** one-shot schema v9 appends `heic`/`heif` only when `allowed_exts` is exactly the pre-0.9.17 five (`png,jpg,jpeg,gif,webp`). Custom lists stay untouched.
- `imgli import-dir` suffix map: add `.heic`, `.heif`. Decode still happens on the server; CLI does not transcode locally.
- Web upload: `accept="image/*"` stays; client-side ext gate follows the session/guest `allowed_exts` list (so iPhone files are choosable once the group allows them).

### 3.8 Docker

Runtime image must load HEIF. Do not assume Alpine `vips` already does. Add the package that provides the loader (`libheif` and/or `vips-heif`, whichever Alpine 3.20 actually ships).

Release gate: a committed **tiny** HEIC fixture uploads on the published Dockerfile and yields JPEG/WebP. If the loader is missing, this version is not done.

## 4. `/t?w=` whitelist

Keep the existing generator (`streamWidthThumb`, `.thumbs/w{N}/g{ThumbGen}/`, singleflight, source-byte and pixel caps). **Only** change the allowed set.

**Allowed widths (union, sorted):** `120, 200, 240, 400, 480, 800, 960, 1600`

- 200 / 400 / 800 remain valid; existing disk keys stay hits.
- Unknown or non-integer `w` → 400 as today.
- Error text is generated from the set (zh and JSON), not the hard-coded “须为 200、400 或 800”.
- Semantics unchanged: long-edge downscale, never upscale; output JPEG for `?w=` (vips default thumbs remain WebP without `w`).

Docs to update in the same PR: README.md, README.zh-CN.md, `docs/integrations/README.md` (and any `w=200|400|800` mention in integration guides).

Frontend library format chips stay `PNG/JPG/GIF/WEBP`. No HEIC chip: stored files are not HEIC.

## 5. Modules

| Area | Touch |
|------|--------|
| `internal/imaging` | HEIC sniff; runtime `HeicDecodeAvailable`; vips decode+autorot+JPEG; pure-Go stubs |
| `internal/service/upload` | Save order; `ErrHeicUnavailable`; seed consumers unchanged |
| `internal/handler` | map error; health/capabilities `heic_decode`; `allowedThumbWidths` |
| `internal/model` | default group JSON seed only |
| `web` | i18n `heic_unsupported`; processing/system capability readout |
| `cmd/imgli` | `import-dir` suffixes |
| `Dockerfile` | HEIF loader package |
| docs / CHANGELOG | whitelist + HEIC note |

No schema version bump. No `files` / `images` column changes.

## 6. Tests

**Pure Go (CI default):**

- `ftyp` fixture (not a full photo) → Probe/Save → `ErrHeicUnavailable` / HTTP 415 `heic_unsupported`.
- JPEG/PNG uploads unchanged.
- `GET /t/{key}?w=` 200 for each new width; 400 for `w=123`, `w=abc`, `w=0`.
- Error body lists the full whitelist.

**Vips + HEIF (build tag + loader; skip if `!HeicDecodeAvailable()`):**

- Minimal valid HEIC → 200 upload; `ext` is `jpg` or `webp`; `links.url` does not end in `.heic`; `GET /i` `image/jpeg` or `image/webp`; default `/t` 200; `?w=1600` 200.
- Group without `heic`/`heif` → `ext_not_allowed` even on a vips build.

**E2E / Docker smoke:**

- Image built from this repo’s Dockerfile can decode the fixture (release job or `ops-smoke`). Failure = missing loader, not a flaky test to skip.

## 7. Acceptance

| ID | Case | Expect |
|----|------|--------|
| H1 | Docker, default group, iPhone HEIC | Upload 200; original is jpeg/webp; share + hotlink work |
| H2 | GitHub pure-Go binary, same file | 415 `heic_unsupported`; queue shows non-retryable copy |
| H3 | Existing group, stock exts only | 415 `ext_not_allowed` until admin adds `heic`/`heif` |
| H4 | `output_format=webp` (vips) | Stored `.webp`; HEIC not retained |
| H5 | `?w=120` and `?w=1600` | 200; `?w=200` still 200; `?w=300` 400 |
| H6 | Public + CDN | `/i` still 302 to original object; `?w=` still origin/cache thumbs |
| H7 | import-dir `.HEIC` | Sent; server transcodes; same errors as web |

## 8. Changelog sketch (Unreleased)

- **Added:** HEIC/HEIF upload on vips builds with a HEIF loader; converted to JPEG (then existing processing). New groups allow `heic`/`heif`.
- **Changed:** `/t?w=` whitelist is 120, 200, 240, 400, 480, 800, 960, 1600.

## 9. OpenList / S3 (recorded, out of tree)

imgli does not add vendor SDKs this version. Operators with FTP or netdisk-private APIs keep using OpenList → WebDAV/S3. OSS/COS **S3-compatible endpoints** already use the standard S3 driver; community matrix `#51` is optional evidence, not a 0.9.17 deliverable.
