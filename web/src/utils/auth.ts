function decodeBase64URL(value: string) {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
  return atob(padded)
}

interface TokenClaims {
  username?: unknown
  is_admin?: unknown
}

function getTokenClaims(): TokenClaims | null {
  try {
    const token = localStorage.getItem('token')
    if (!token) return null

    const [, payload] = token.split('.')
    if (!payload) return null

    return JSON.parse(decodeBase64URL(payload)) as TokenClaims
  } catch {
    return null
  }
}

export function getTokenUsername(fallback = '') {
  const parsed = getTokenClaims()
  return typeof parsed?.username === 'string' && parsed.username ? parsed.username : fallback
}

export function getTokenIsAdmin() {
  return getTokenClaims()?.is_admin === true
}
