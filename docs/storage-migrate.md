# Cross-policy storage migration

Move object rows between storage policies **after** both policies work for
upload/serve. Prefer a dedicated test pass with dry-run and a small limit.

## Prerequisites

- Target policy **enabled** and credentials valid (`imgli doctor` / admin probe).
- New uploads should already target the destination (group `allowed_policy_ids` /
  user default) so the source is not still growing.
- Same object key (`files.path`) is reused; S3 path templates must remain
  compatible with existing keys.
- Only **admins** may start full-site migrate (CLI on the host or Admin UI).

## Admin UI (recommended for operators)

1. Open **Admin → Storage policies**.
2. Use the **Cross-policy migrate** panel:
   - Select **from** / **to** policies (must differ; target must be enabled).
   - Prefer **Dry-run** first.
   - Optional **max rows** (0 = unlimited) and **delete source** after success.
3. Click **Start migrate** and watch status / progress
   (`scanned` / `ok` / `skipped` / `failed`). Sample paths are redacted.

API (session cookie, admin role):

```http
POST /api/v1/admin/storage/migrate
Content-Type: application/json

{
  "from_policy_id": 1,
  "to_policy_id": 2,
  "dry_run": true,
  "delete_source": false,
  "limit": 50
}
```

```http
GET /api/v1/admin/storage/migrate/{job_id}
```

| Field | Notes |
|-------|--------|
| Job is process-local | Restart loses in-memory job status; re-run is safe: already-retargeted rows no longer match `from` |
| One job per source | Concurrent migrate on the same `from` returns conflict / busy |
| Secrets | Progress never includes storage credentials |

## CLI

```bash
# Preview
imgli storage-migrate -config /path/to/imgli.yaml -from 1 -to 2 -dry-run

# Small batch
imgli storage-migrate -config /path/to/imgli.yaml -from 1 -to 2 -limit 50

# Full cutover (optional delete source objects after DB update)
imgli storage-migrate -config /path/to/imgli.yaml -from 1 -to 2 -delete-source
```

| Flag | Meaning |
|------|---------|
| `-from` / `-to` | `storage_policies.id` |
| `-dry-run` | Count only; no Put / no DB update |
| `-limit N` | Process at most N `files` rows |
| `-delete-source` | After successful retarget, delete object (+ thumbs) on source |

Thumbs under `.thumbs/…` are copied best-effort; missing thumbs do not fail the row.

## Acceptance sketch (operators)

1. Dry-run: `scanned > 0`, DB and objects unchanged.
2. `limit=10`: exactly 10 rows retargeted when enough candidates exist; objects readable on `to`.
3. Full + delete-source: source empty of migrated keys (or only skipped missing); serve via `to`.
4. Interrupt / re-run: no double-delete chaos; rows already on `to` are not selected again.
5. Target disabled: start refused.
6. Private / password / expiry gates still apply after migrate (metadata unchanged).

## What this is not

- Continuous **dual-write** or replica sync (see design draft).
- Import from a foreign tree → use `imgli import-dir` instead.
- CDN cache purge (do that in your CDN console after public keys move).

## Design (multi-policy / sync roadmap)

See [design/storage-migrate-sync-draft.md](design/storage-migrate-sync-draft.md).
