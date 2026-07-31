# Cleanup vs CDN and quota boundaries

Operator cleanup (Admin `POST /admin/cleanup/*` or hourly server jobs) acts on
the **origin** database and storage drivers.

## What cleanup does

- Deletes `images` rows and enqueues physical object deletes on the configured
  storage policy.
- Frees **origin** storage quota accounting for affected users.

## What cleanup does not do

| System | Effect of cleanup |
|--------|-------------------|
| **CDN** (Cloudflare, vendor CDN, `cdn_domain`) | Cached public objects may still be served until TTL/purge. **imgli does not auto-purge CDN.** Use your CDN console or API after bulk cleanup of public keys. |
| **CDN billing** | Unrelated to origin delete counts; dashboard traffic may under-count CDN hits (see `deploy/ops/admin-stats-metering.md`). |
| **Presigned private URLs** | Already short-lived; no CDN offload for private by design. |

## Doctor / ops tips

- After large public cutovers or mass deletes, plan a CDN purge for high-traffic
  prefixes if visitors still see old content.
- Prefer dry-run (`/admin/cleanup/preview`) before `/admin/cleanup/run`.
