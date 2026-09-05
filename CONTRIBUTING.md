# Contributing to imgli

Issues and pull requests are welcome — bug reports, S3-compatible vendor test
results, translations, and features alike.

## License of contributions

The project is **[AGPL-3.0-only](LICENSE)** (see also [COMMERCIAL.md](COMMERCIAL.md)
for dual commercial licensing by the copyright holder).

By opening a pull request you agree that:

1. You have the right to submit the contribution.
2. Your contribution is licensed to the project under **AGPL-3.0-only**.
3. You grant the copyright holder (Yixian Huang) a **perpetual, worldwide,
   royalty-free right to relicense your contribution** as part of dual-licensed
   distributions (including proprietary commercial licenses of imgli that
   include your contribution).

Please include a **Developer Certificate of Origin** sign-off on each commit
(CI rejects PRs that omit it; dependabot / github-actions are skipped):

```bash
git commit -s -m "your message"
# adds: Signed-off-by: Your Name <you@example.com>
```

Substantial contributions may later require a short CLA form; we will not merge
external code into the commercial dual-license tree without clear rights.

## Development setup

```bash
make build       # web (npm) + Go binary with embedded frontend
make run         # build + ./imgli serve  (http://localhost:8686)
make test        # go vet + go test
make test-web    # vitest
```

Go ≥ 1.26 and Node ≥ 24 are expected (see `.github/workflows/ci.yml`, which is
the source of truth). **Local/CI default** is pure Go (CGO-free, SQLite
included). **Production Docker images** build with `-tags vips` and ship
libvips (see `Dockerfile`). Use `make build-vips` locally if you have vips dev
headers; `make docker-build` matches the release image.

## Documentation

See **[docs/documentation-ssot.md](docs/documentation-ssot.md)** for where docs live:

- **This repo `docs/`** — engineering SSOT (matrices, scripts, design drafts).
- **Product site (`docs-imgli/` in the maintainers’ KB → docs.imgli.com)** — end-user how-tos; update after user-facing releases (short pages + links to GitHub for long matrices).
- **Internal KB (`imgli/*`, hub)** — roadmap/ops only; not for public full copies.

Behavior change in a PR → update repo docs in the **same PR**. Do not maintain a second full matrix only in the KB.

## Guidelines

- Keep PRs focused; include tests for behavior changes. CI must pass
  (sqlite + postgres matrix, web tests, e2e smoke).
- The codebase comments are written in Chinese; either language is fine in
  contributions, but please match the style of the file you are editing.
- S3 vendor reports: run `scripts/s3-vendor-matrix.sh` and
  `scripts/s3-vendor-e2e.sh` against your vendor (see
  `scripts/imgli-s3-vendors.env.example`) and open an issue with the **S3 vendor
  test report** template — results feed
  [docs/s3-compatibility.md](docs/s3-compatibility.md).
- Security issues: see [SECURITY.md](SECURITY.md) — never via public issues.
- User-facing changes: add a bullet under **`[Unreleased]`** in
  [CHANGELOG.md](CHANGELOG.md). Mark breaking changes clearly (and prefer
  `BREAKING` in the PR title).

## Versioning and releasing

Product version is **only** the git tag (`vMAJOR.MINOR.PATCH`). Build injects it
via `-ldflags` (`make build` / Docker `VERSION` / GoReleaser). Do not duplicate
it in `go.mod` or `web/package.json`.

| Change | Bump |
|--------|------|
| Breaking API/config/storage semantics | MAJOR (or MINOR while still `0.x`) |
| New features | MINOR |
| Bug fixes, security patches | PATCH |

Internal DB `SchemaVersion` is independent of the product tag; call out
migrations in the changelog when operators must act.

### Maintainer release checklist

Full playbook (tag gate scripts, job order, baili deploy, public smoke):
**[docs/ops-release.md](docs/ops-release.md)**.

Short form:

1. PR → CI green (e2e-smoke on PR; full e2e on `main`); merge.
2. Move `[Unreleased]` notes in `CHANGELOG.md` into a dated `## [X.Y.Z]` section;
   refresh the compare links at the bottom; leave an empty `[Unreleased]`.
3. `./scripts/pre-tag-check.sh vX.Y.Z` (CHANGELOG section + clean tree + main CI).
4. Tag and push:

   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```

5. GitHub Actions [release](.github/workflows/release.yml):
   - **goreleaser** — multi-platform binaries → GitHub Release (production path);
   - **docker-amd64** / **docker-multi** — `ghcr.io/yixian-huang/imgli` tags
     `vX.Y.Z`, `X.Y.Z`, `X.Y`, and `latest` (not for pre-releases containing `-`).
     Binary deploy need not wait for multi-arch docker.

6. Smoke: `./scripts/ops-smoke-public.sh https://…` or Actions `smoke-prod`;
   `imgli version` on a release asset should print `vX.Y.Z`.

Local dry-run (requires [GoReleaser](https://goreleaser.com/) installed):

```bash
make release-snapshot
```