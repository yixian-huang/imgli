# imgli public roadmap (mirror)

> **Planning SSOT is internal** (`imgli/roadmap` in the project knowledge base).  
> This file is a **public, de-sensitized mirror** for contributors. Prefer GitHub Issues + Milestones for execution.

## Current focus — v0.6.0 · Ops · Migrate · Trust

Milestone: https://github.com/yixian-huang/imgli/milestone/5 (**open**)  
Baseline: [v0.5.1](https://github.com/yixian-huang/imgli/releases/tag/v0.5.1) · [v0.5.0](https://github.com/yixian-huang/imgli/releases/tag/v0.5.0)

**One-liner:** help self-host operators **move storage safely, clean lifecycle data, and upgrade the binary** — all on the AGPL Community trunk (no white-label / multi-tenant control plane).

| Theme | Highlights | Issues (examples) |
|-------|------------|-------------------|
| Migrate | Admin storage migrate jobs, progress, resume; mutex/safety; filters + size checks | #53–#55, #58, #63 |
| Upgrade | Admin shows version, probes GitHub Releases, one-click binary upgrade (checksums; Docker = redeploy path) | #56, #57 |
| Lifecycle | Inactive/expired cleanup dry-run + batched execute | #59, #61 |
| Trust / community | Community S3 vendor matrix reports | #51 |
| Polish | Migrate/cleanup tests; OIDC operator docs | #60, #62 |

Suggested implementation order: **#53 → #54 → #55 ‖ #56**, then **#57 → #58 → #60 → #59**, with **#51** in parallel. P2 items can interleave.

### Explicit non-goals for v0.6

Team/org spaces, dual-write / object replicas, full transform suite, new IdP protocols (docs only), silent auto-upgrade / phone-home, full white-label, multi-tenant control plane, payment SKUs, video mainline, independent short-link product.

## Latest shipped — v0.5.x · Trust · Onboard · Community

Release: [v0.5.1](https://github.com/yixian-huang/imgli/releases/tag/v0.5.1) (patch) · [v0.5.0](https://github.com/yixian-huang/imgli/releases/tag/v0.5.0)  
Milestone: https://github.com/yixian-huang/imgli/milestone/4 (**closed**, 14/14)

| Theme | Highlights |
|-------|------------|
| Trust | Cloudflare R2 **Verified** in S3 matrix; moderation operator spot-check docs |
| Onboard | First-run Token path; welcome email (SMTP); scenario copy (`?from=` / UTM) |
| Site ops | Favicon URL, title strategy, optional “based on imgli”, source URL, About page |
| Community | Public `ROADMAP.md`, README docs map; other S3 vendors via community reports |

## Shipped earlier

- **v0.4.x** — Storage Caps, FTP compatibility driver  
- **v0.3.0** — Password shares, public albums, width thumbs, import-dir, OIDC, webhooks  
- **v0.2.x** — CLI upload, doctor, EXIF strip, max views, light admin analytics  

## Later candidates (not a committed schedule)

- Single-instance team/org collaboration  
- Fuller transform suite  
- More IdP connector implementations  
- Async object replicas / dual-write (after migrate is solid)  

Community PRs that fit AGPL single-instance self-host are welcome anytime — file an Issue or open a PR against `main`.

## Non-goals (do not open “white-label full” issues expecting Community)

Full BrandLockup replace, multi-tenant control plane, Open Core paywalls, video mainline, independent short-link product. See `COMMERCIAL.md` for dual-license inquiries.

## Contribute

- Bug / feature: GitHub Issues (templates under `.github/`)  
- S3 vendor reports: [`docs/s3-compatibility.md`](docs/s3-compatibility.md) + issue template  
- Security: [`SECURITY.md`](SECURITY.md)
