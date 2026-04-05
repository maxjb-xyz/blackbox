import { useEffect, useState } from 'react'
import { CheckCircle, XCircle } from 'lucide-react'
import { checkHealth } from '../api/client'
import type { HealthStatus } from '../api/client'
import PageHeader from '../components/PageHeader'

export default function DiagnosticsPage() {
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [healthError, setHealthError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  function reload() {
    setLoading(true)
    setHealthError(null)
    checkHealth()
      .then(status => {
        setHealth(status)
        setHealthError(null)
      })
      .catch(err => {
        setHealthError(err instanceof Error ? err.message : 'Failed to check health')
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    checkHealth()
      .then(status => {
        setHealth(status)
        setHealthError(null)
      })
      .catch(err => {
        setHealthError(err instanceof Error ? err.message : 'Failed to check health')
      })
      .finally(() => setLoading(false))
  }, [])

  function statusColor(status: string) {
    if (status === 'ok' || status === 'online') return 'var(--success)'
    if (status === 'disabled') return 'var(--muted)'
    return '#FF4444'
  }

  return (
    <div style={{ minHeight: '100%', background: '#0B0B0B', fontFamily: 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace' }}>
      <PageHeader
        title="DIAGNOSTICS / SYSTEM HEALTH"
        actions={(
          <button
            onClick={reload}
            style={{
              background: 'none',
              border: '1px solid var(--border)',
              color: 'var(--muted)',
              padding: '4px 10px',
              fontFamily: 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace',
              fontSize: '11px',
              cursor: 'pointer',
              letterSpacing: '0.05em',
            }}
          >
            REFRESH
          </button>
        )}
      />
      <div style={{ padding: '10px 16px', maxWidth: 960, margin: '0 auto', background: '#0B0B0B' }}>
        {healthError && (
          <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>
            {healthError}
          </div>
        )}
        {loading ? (
          <div style={{ color: 'var(--muted)', fontSize: '12px' }}>checking...</div>
        ) : health ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {[
              { label: 'DATABASE', value: health.database },
              ...(health.oidc_enabled ? [{ label: 'OIDC', value: health.oidc }] : []),
            ].map(row => {
              const ok = row.value === 'ok'
              return (
                <div key={row.label} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: '12px', fontFamily: 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace' }}>
                  {ok ? <CheckCircle size={12} style={{ color: 'var(--success)' }} /> : <XCircle size={12} style={{ color: statusColor(row.value) }} />}
                  <span style={{ color: 'var(--muted)', width: 100 }}>{row.label}</span>
                  <span style={{ color: statusColor(row.value) }}>{row.value.toUpperCase()}</span>
                </div>
              )
            })}
          </div>
        ) : (
          <div style={{ color: 'var(--muted)', fontSize: '12px' }}>health data unavailable</div>
        )}
      </div>
    </div>
  )
}
