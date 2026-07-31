# OIDC operator troubleshooting

imgli supports a **generic OIDC** login (Admin `GET/PUT /api/v1/admin/oidc`).
This is not a multi-IdP product; one OIDC client per instance.

## Checklist

1. **Issuer discovery** — Issuer URL must expose `/.well-known/openid-configuration`.
2. **Redirect URI** — Register `{base_url}/api/v1/auth/oidc/callback` (exact match).
3. **Client type** — Confidential client with client secret; enable Authorization Code.
4. **Scopes** — At least `openid email profile` (email required for auto-provision).
5. **Email claim** — User must have a verified email claim; missing email fails provision.
6. **Clock skew** — NTP on the imgli host; large skew breaks `id_token` validation.

## Common IdPs

| IdP | Notes |
|-----|--------|
| Authentik / Keycloak | Create OIDC app; set redirect URI; copy client id/secret and issuer |
| Google Workspace | OAuth client (Web); authorized redirect URI as above; issuer `https://accounts.google.com` |
| Authelia | OIDC client with matching redirect; ensure email is in claims |

## Failure modes

| Symptom | Likely cause |
|---------|----------------|
| Start 404 / disabled | Admin OIDC not enabled or public config `oidc_enabled` false |
| Callback 400 invalid state | Cookie/session domain or multi-instance without sticky sessions |
| Login succeeds but no account | Missing email claim |
| Intermittent token errors | Clock skew or issuer URL mismatch (trailing slash) |

## Disable

Admin OIDC settings: turn off enabled flag. Local password login remains available
unless you separately disable registration/login.

## Non-goals

New protocols (SAML, LDAP) and per-tenant IdP packs are out of scope for Community.
