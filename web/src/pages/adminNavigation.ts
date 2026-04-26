export type AdminGroup = 'access' | 'integrations' | 'system'
export type Tab =
  | 'invites'
  | 'users'
  | 'oidc'
  | 'audit'
  | 'notifications'
  | 'webhooks'
  | 'agents'
  | 'excludes'
  | 'github'
  | 'ai'
  | 'systemd'
  | 'filewatcher'

export const ADMIN_GROUPS: Record<AdminGroup, { label: string; tabs: Tab[] }> = {
  access: { label: 'ACCESS', tabs: ['users', 'invites', 'oidc', 'audit'] },
  integrations: { label: 'INTEGRATIONS', tabs: ['notifications', 'webhooks', 'agents', 'excludes'] },
  system: { label: 'SYSTEM', tabs: ['ai', 'systemd', 'filewatcher', 'github'] },
}

export const ALL_ADMIN_GROUPS: AdminGroup[] = ['access', 'integrations', 'system']

type AdminTabDirection = 'previous' | 'next'

export function getWrappedAdminTabIndex(index: number, length: number): number {
  return (index + length) % length
}

export function getAdminTabNavigationKey(
  key: string,
  isDesktopSidebar: boolean,
): AdminTabDirection | null {
  if (isDesktopSidebar) {
    if (key === 'ArrowUp') return 'previous'
    if (key === 'ArrowDown') return 'next'
    return null
  }

  if (key === 'ArrowLeft') return 'previous'
  if (key === 'ArrowRight') return 'next'
  return null
}
