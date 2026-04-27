---
title: Authentication
---

# Authentication

Blackbox supports local authentication by default and optional OIDC-based SSO
for operators who want centralized identity management.

## Local Authentication

- The first launch flow bootstraps the initial admin account.
- Local users sign in with a username and password.
- Passwords are stored using Argon2id hashing rather than plaintext or
  reversible encryption.

## Session Model

- Blackbox issues a signed JWT session cookie after successful authentication.
- `JWT_SECRET` controls the signing key for those session cookies.
- `JWT_TTL` controls the session lifetime and defaults to `24h`.
- Local auth and OIDC both end in the same session model once sign-in succeeds.

## OIDC And SSO

OIDC providers are configured from **Admin > Access > OIDC**.

- Multiple providers can be configured.
- Each provider defines its own callback URL and access policy.
- OIDC logins appear alongside local authentication on the login page.

See [OIDC And SSO](../integrations/oidc-sso.md) for provider-specific setup.

## User Onboarding

Blackbox supports invite-based onboarding for controlled account creation.

- Admins can generate invite codes from the admin UI.
- Invite codes can be used for local registration.
- OIDC providers can also be configured with an `invite required` policy.

See [User Registration And Invites](./user-registration-and-invites.md) for the
operator workflow.

## Related Docs

- [Server Environment](../configuration/server-environment.md)
- [OIDC And SSO](../integrations/oidc-sso.md)
- [User Registration And Invites](./user-registration-and-invites.md)
- [Security Model](./security-model.md)
