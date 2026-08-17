import { useEffect, useState } from 'react'
import { CheckCircle, XCircle } from 'lucide-react'
import { checkDetailedHealth } from '../api/client'
import type { HealthStatus } from '../api/client'
import PageHeader from '../components/PageHeader'

export default function DiagnosticsPage() {
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [healthError, setHealthError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  function reload() {
    setLoading(true)
    setHealthError(null)
    checkDetailedHealth()
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
    checkDetailedHealth()
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
    if (status === 'ok' || status === 'online' || status === 'ready') return 'var(--success)'
    if (status === 'disabled' || status === 'unconfigured') return 'var(--muted)'
    if (status === 'warning') return '#F59E0B'
    return '#FF4444'
  }

  function healthRows(current: HealthStatus) {
    const mcpStatus = !current.mcp.enabled
      ? 'disabled'
      : !current.mcp.token_configured
        ? 'error'
        : current.mcp.running
          ? 'ok'
          : 'warning'
    const mcpValue = !current.mcp.enabled
      ? 'disabled'
      : !current.mcp.token_configured
        ? 'token missing'
        : current.mcp.running
          ? 'enabled / running'
          : 'enabled / not running'
    const aiStatus = !current.ai.configured ? 'unconfigured' : current.ai.testable ? 'ok' : 'error'
    const aiValue = !current.ai.configured
      ? 'unconfigured'
      : current.ai.testable
        ? `${current.ai.provider} / configured (no probe)`
        : `${current.ai.provider} / not testable`
    const nodeStatus = current.nodes.offline > 0 ? 'warning' : 'ok'
    const nodeValue = `${current.nodes.online} online / ${current.nodes.total} total · ${current.nodes.stale} stale`
    const configValue = (summary: HealthStatus['notifications'] | HealthStatus['webhooks']) => (
      summary.configured ? `${summary.enabled} enabled / ${summary.total} configured` : 'unconfigured'
    )

    return [
      { label: 'SERVER', value: `${current.version} / ${current.commit}`, status: 'ok' },
      { label: 'DATABASE', value: current.database, status: current.database },
      { label: 'OIDC', value: current.oidc_enabled ? current.oidc : 'disabled', status: current.oidc_enabled ? current.oidc : 'disabled' },
      { label: 'MCP', value: mcpValue, status: mcpStatus },
      { label: 'AI PROVIDER', value: aiValue, status: aiStatus },
      { label: 'NODES', value: nodeValue, status: nodeStatus },
      { label: 'NOTIFICATIONS', value: configValue(current.notifications), status: current.notifications.configured ? 'ok' : 'unconfigured' },
      { label: 'WEBHOOKS', value: configValue(current.webhooks), status: current.webhooks.configured ? 'ok' : 'unconfigured' },
    ]
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
            {healthRows(health).map(row => {
              const ok = row.status === 'ok'
              return (
                <div key={row.label} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: '12px', fontFamily: 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace' }}>
                  {ok ? <CheckCircle size={12} style={{ color: 'var(--success)' }} /> : <XCircle size={12} style={{ color: statusColor(row.status) }} />}
                  <span style={{ color: 'var(--muted)', width: 100 }}>{row.label}</span>
                  <span style={{ color: statusColor(row.status) }}>{row.value.toUpperCase()}</span>
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
