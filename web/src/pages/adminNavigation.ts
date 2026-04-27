export type AdminGroup = 'access' | 'sources' | 'integrations' | 'system'
export type Tab =
  | 'invites'
  | 'users'
  | 'oidc'
  | 'audit'
  | 'notifications'
  | 'webhooks'
  | 'agents'
  | 'github'
  | 'ai'

export const ADMIN_GROUPS: Record<AdminGroup, { label: string; tabs: Tab[] }> = {
  access: { label: 'ACCESS', tabs: ['users', 'invites', 'oidc', 'audit'] },
  sources: { label: 'DATA SOURCES', tabs: [] }, // dynamic sidebar
  integrations: { label: 'INTEGRATIONS', tabs: ['notifications', 'webhooks', 'agents'] },
  system: { label: 'SYSTEM', tabs: ['ai', 'github'] },
}

export const ALL_ADMIN_GROUPS: AdminGroup[] = ['access', 'sources', 'integrations', 'system']
export const ADMIN_SIDEBAR_BREAKPOINT = 961
export const ADMIN_SIDEBAR_BREAKPOINT_QUERY = `(min-width: ${ADMIN_SIDEBAR_BREAKPOINT}px)`

type AdminTabDirection = 'previous' | 'next'

export function getWrappedAdminTabIndex(index: number, length: number): number {
  if (length <= 0) {
    throw new RangeError('length must be greater than 0')
  }
  return ((index % length) + length) % length
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
