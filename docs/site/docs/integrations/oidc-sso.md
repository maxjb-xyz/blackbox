---
title: OIDC And SSO
---

# OIDC And SSO

Blackbox supports multiple OIDC providers configured from
**Admin > Access > OIDC**.

## Per-Provider Fields

- Name
- Issuer URL
- Client ID
- Client Secret
- Redirect URL

## Callback Format

```text
https://<your-domain>/api/auth/oidc/<provider-id>/callback
```

## Access Policies

- `open`
- `existing accounts only`
- `invite required`

## Result

Configured providers appear on the login page alongside local authentication.

For the broader auth model, including local sign-in and session behavior, see
[Authentication](../operations/authentication.md).
