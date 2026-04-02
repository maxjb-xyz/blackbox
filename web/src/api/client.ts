export interface SetupStatus {
  bootstrapped: boolean
}

export interface HealthStatus {
  database: 'ok' | 'error'
  oidc: 'ok' | 'unavailable' | 'disabled'
  oidc_enabled: boolean
}

export async function checkSetupStatus(): Promise<SetupStatus> {
  const res = await fetch('/api/setup/status')
  if (!res.ok) throw new Error('Failed to check setup status')
  return res.json()
}

export async function checkHealth(): Promise<HealthStatus> {
  const res = await fetch('/api/setup/health')
  if (!res.ok) throw new Error('Failed to check health')
  return res.json()
}

export async function bootstrap(username: string, password: string): Promise<string> {
  const res = await fetch('/api/auth/bootstrap', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error ?? 'Bootstrap failed')
  }
  const data = await res.json()
  return data.token as string
}

export async function login(username: string, password: string): Promise<string> {
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error ?? 'Login failed')
  }
  const data = await res.json()
  return data.token as string
}
