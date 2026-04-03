function decodeBase64URL(value: string) {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
  return atob(padded)
}

export function getTokenUsername(fallback = '') {
  try {
    const token = localStorage.getItem('token')
    if (!token) return fallback

    const [, payload] = token.split('.')
    if (!payload) return fallback

    const parsed = JSON.parse(decodeBase64URL(payload)) as { user_id?: unknown }
    return typeof parsed.user_id === 'string' && parsed.user_id ? parsed.user_id : fallback
  } catch {
    return fallback
  }
}
