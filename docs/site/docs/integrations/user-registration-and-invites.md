---
title: User Registration And Invites
---

# User Registration And Invites

Blackbox supports admin-created invite codes for onboarding new users.

## Admin Workflow

Admins can create invite codes from the admin UI and copy the full invite link
for sharing.

## Registration URL

Invited users register with a URL like:

```text
/register?code=<invite-code>
```

## Operational Notes

- Unused invite codes can be revoked from the admin UI.
- Invite behavior also interacts with OIDC provider policy when SSO is enabled.
- This gives operators a middle ground between fully open registration and
  purely manual account creation.
