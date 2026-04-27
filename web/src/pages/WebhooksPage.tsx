import { useState } from 'react'
import { CheckCircle, Copy } from 'lucide-react'
import { useSession } from '../session'
import PageHeader from '../components/PageHeader'

function CopyRow({ label, path }: { label: string; path: string }) {
  const [copied, setCopied] = useState(false)
  const [copyError, setCopyError] = useState<string | null>(null)
  const url = `${window.location.origin}${path}`

  function handleCopy() {
    setCopyError(null)
    if (!navigator?.clipboard) {
      setCopyError('clipboard unavailable')
      return
    }
    navigator.clipboard.writeText(url)
      .then(() => { setCopied(true); setTimeout(() => setCopied(false), 2000) })
      .catch(err => { console.error('copy webhook url:', err); setCopyError('copy failed') })
  }

  return (
    <div style={{ padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.05em', width: 120, flexShrink: 0 }}>
          {label}
        </span>
        <span style={{ color: 'var(--text)', fontSize: '12px', flex: 1 }}>{url}</span>
        <button
          onClick={handleCopy}
          style={{
            background: 'none',
            border: '1px solid var(--border)',
            color: copied ? 'var(--accent)' : 'var(--muted)',
            padding: '4px 8px',
            fontFamily: 'inherit',
            fontSize: '11px',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          {copied ? <CheckCircle size={12} /> : <Copy size={12} />}
          {copied ? 'COPIED' : 'COPY'}
        </button>
      </div>
      {copyError && <div style={{ color: 'var(--danger)', fontSize: '11px', marginTop: 6 }}>{copyError}</div>}
    </div>
  )
}

export default function WebhooksPage() {
  const { user } = useSession()
  const isAdmin = user?.is_admin === true

  return (
    <div>
      <PageHeader title="WEBHOOKS / ENDPOINTS" />
      <div style={{ padding: '24px', maxWidth: 960, margin: '0 auto' }}>
        <p style={{ color: 'var(--muted)', fontSize: '12px', marginBottom: 16 }}>
          {isAdmin ? (
            <>
              Configure your webhook providers to POST to these endpoints. Set the{' '}
              <code style={{ color: 'var(--accent)' }}>X-Webhook-Secret</code> header to the secret configured on the matching
              source in <code style={{ color: 'var(--accent)' }}>Admin &gt; Data Sources</code>.
            </>
          ) : (
            <>Configure your webhook providers to POST to these endpoints. Contact your administrator to get the webhook secret.</>
          )}
        </p>
        <CopyRow label="UPTIME KUMA" path="/api/webhooks/uptime" />
        <CopyRow label="WATCHTOWER" path="/api/webhooks/watchtower" />
        {isAdmin && (
          <div style={{ marginTop: 24, color: 'var(--muted)', fontSize: '11px' }}>
            Manage per-source webhook secrets from <code style={{ color: 'var(--accent)' }}>Admin &gt; Data Sources</code>.
          </div>
        )}
      </div>
    </div>
  )
}
