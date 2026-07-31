# imgli public roadmap (mirror)

> **Planning SSOT is internal** (`imgli/roadmap` in the project knowledge base).  
> This file is a **public, de-sensitized mirror** for contributors. Prefer GitHub Issues + Milestones for execution.

## Latest shipped — v0.6.0 · Ops · Migrate · Trust

Release: https://github.com/yixian-huang/imgli/releases/tag/v0.6.0  
Milestone: https://github.com/yixian-huang/imgli/milestone/5

| Theme | Highlights |
|-------|------------|
| Migrate | Admin storage migrate jobs, progress/resume, filters + size verify, safety mutex |
| Upgrade | Admin version display, GitHub update probe, one-click binary upgrade (checksums; Docker = redeploy) |
| Lifecycle | Cleanup preview/run for expired images and aged trash |
| Docs | Admin migrate runbook, cleanup vs CDN, OIDC operator troubleshooting |

## Shipped earlier

- **v0.5.x** — Trust · Onboard · Community (R2 Verified, first-run Token, site ops, public ROADMAP)  
- **v0.4.x** — Storage Caps, FTP compatibility driver  
- **v0.3.0** — Password shares, public albums, width thumbs, import-dir, OIDC, webhooks  
- **v0.2.x** — CLI upload, doctor, EXIF strip, max views, light admin analytics  

## Next (not a committed schedule)

- **Community:** more S3-compatible vendors in the matrix (#51) — vendor report template + `scripts/s3-vendor-matrix.sh`  
- **Later candidates (internal SSOT):** single-instance team/org, fuller transform suite, more IdP connectors, async replicas / dual-write  

No open product milestone is required for community PRs — file an Issue or open a PR against `main`.

## Non-goals (do not open “white-label full” issues expecting Community)

Full BrandLockup replace, multi-tenant control plane, Open Core paywalls, video mainline, independent short-link product. See `COMMERCIAL.md` for dual-license inquiries.

## Contribute

- Bug / feature: GitHub Issues (templates under `.github/`)  
- S3 vendor reports: [`docs/s3-compatibility.md`](docs/s3-compatibility.md) + issue template  
- Security: [`SECURITY.md`](SECURITY.md)
