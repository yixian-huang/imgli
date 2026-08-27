# Integrations

Upload to imgli / [img.li](https://img.li) using the same HTTP contract:

| Item | Value |
|------|--------|
| URL | `{BASE_URL}/api/v1/upload` |
| Method | `POST` |
| Body | `multipart/form-data`, file field **`file`** |
| Auth | Header `Authorization: Bearer <API_TOKEN>` |
| URL path in JSON | **`data.links.url`** |

Create a token: **Settings → API Token** (scope `upload` or `full`). Plaintext is shown once.

| Guide | Client |
|-------|--------|
| [picgo.md](../picgo.md) | PicGo / PicGo-Core / Typora / VS Code |
| [sharex.md](sharex.md) | [ShareX](https://getsharex.com/) (Windows) |
| [upic.md](upic.md) | [uPic](https://github.com/gee1k/uPic) / PicList-style custom uploaders |
| README **CLI** | `imgli upload` (single file) · `imgli import-dir` (bulk directory) |

### Optional form fields (upload)

| Field | Notes |
|-------|--------|
| `expires_in` | Seconds; `0` / omit = permanent **unless** the user group forbids it (v0.9+) |
| `max_views` | `0` / omit = unlimited **unless** group `max_max_views` > 0 |
| `visibility` | `public` \| `private` |

Group limits (token account): `GET /api/v1/user/quota`. Guest:
`GET /api/v1/config` → `guest`. Full policy guide:
[user-groups-lifecycle.md](../user-groups-lifecycle.md).

Error codes for over-cap options: `expires_over_group`, `max_views_over_group` (HTTP 400).

### CLI

```bash
export IMGLI_BASE_URL=https://your-host
export IMGLI_TOKEN='your-api-token'

imgli upload shot.png
imgli upload -verbose shot.png              # print group expiry/views limits (stderr)
imgli upload -expires-in 86400 shot.png
imgli import-dir ./photos                   # recursive by default
imgli import-dir -dry-run ./photos
imgli import-dir -visibility private ./in
```

`import-dir` flags: `-recursive` (default true), `-continue` (default true),
`-base-url`, `-token`. Reuses `POST /api/v1/upload`.

### Related HTTP surfaces (v0.3+)

| Surface | Notes |
|---------|--------|
| `GET /t/{key}.jpg?w=120\|200\|240\|400\|480\|800\|960\|1600` | Whitelisted thumbnail widths |
| `POST /api/v1/s/{key}/unlock` | Password-protected share unlock |
| `GET /api/v1/a/{id}` · `/a/{id}/images` | Public album visitor API |
| Outbound webhooks | Admin `GET/PUT /api/v1/admin/webhooks` |
| OIDC SSO | Admin `GET/PUT /api/v1/admin/oidc`; user start `/api/v1/auth/oidc/start` |

Product docs: [docs.imgli.com](https://docs.imgli.com) (when published from `docs-imgli/`).

Sample ShareX custom uploader: [imgli.sxcu.example](imgli.sxcu.example).

Replace `https://img.li` with your self-hosted `IMGLI_BASE_URL` everywhere.
