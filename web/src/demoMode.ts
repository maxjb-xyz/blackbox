import type { SessionUser } from './api/client'

export const DEMO_INTRO_DISMISSED_KEY = 'blackbox_demo_intro_dismissed'

export const DEMO_SESSION_USER: SessionUser = {
  user_id: 'demo-admin',
  username: 'operator',
  is_admin: true,
  email: 'operator@blackboxd.dev',
  oidc_linked: false,
}

export function isDemoModeEnabled(flag: string | undefined): boolean {
  return flag === 'true'
}

export function isDemoIntroDismissed(value: string | null): boolean {
  return value === 'true'
}
