import { useEffect, useState } from 'react'
import { CheckCircle, Copy, Eye, EyeOff } from 'lucide-react'
import { useSession } from '../session'
import { fetchAdminConfig } from '../api/client'
import PageHeader from '../components/PageHeader'

function CopyRow({ label, path }: { label: string; path: string }) {
  const [copied, setCopied] = useState(false)
  const [copyError, setCopyError] = useState<string | null>(null)
  const url = `${window.location.origin}${path}`

  function handleCopy() {
    setCopyError(null)
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

function SecretRow({ secret }: { secret: string }) {
  const [revealed, setRevealed] = useState(false)
  const [copied, setCopied] = useState(false)

  function handleCopy() {
    navigator.clipboard.writeText(secret)
      .then(() => { setCopied(true); setTimeout(() => setCopied(false), 2000) })
      .catch(() => {})
  }

  return (
    <div style={{ padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.05em', width: 120, flexShrink: 0 }}>
          WEBHOOK SECRET
        </span>
        <span style={{ color: 'var(--text)', fontSize: '12px', flex: 1, fontFamily: 'inherit', letterSpacing: revealed ? 'normal' : '0.2em' }}>
          {revealed ? secret : '************'}
        </span>
        <button
          onClick={() => setRevealed(p => !p)}
          style={{
            background: 'none',
            border: '1px solid var(--border)',
            color: 'var(--muted)',
            padding: '4px 8px',
            fontFamily: 'inherit',
            fontSize: '11px',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          {revealed ? <EyeOff size={12} /> : <Eye size={12} />}
          {revealed ? 'HIDE' : 'SHOW'}
        </button>
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
    </div>
  )
}

export default function WebhooksPage() {
  const { user } = useSession()
  const isAdmin = user?.is_admin === true
  const [webhookSecret, setWebhookSecret] = useState<string | null>(null)

  useEffect(() => {
    if (!isAdmin) return
    fetchAdminConfig()
      .then(cfg => setWebhookSecret(cfg.webhook_secret))
      .catch(() => {})
  }, [isAdmin])

  return (
    <div>
      <PageHeader title="WEBHOOKS / ENDPOINTS" />
      <div style={{ padding: '24px', maxWidth: 960, margin: '0 auto' }}>
        <p style={{ color: 'var(--muted)', fontSize: '12px', marginBottom: 16 }}>
          Configure your webhook providers to POST to these endpoints. Set{' '}
          <code style={{ color: 'var(--accent)' }}>X-Webhook-Secret</code> header to the value of{' '}
          <code style={{ color: 'var(--accent)' }}>WEBHOOK_SECRET</code>.
        </p>
        <CopyRow label="UPTIME KUMA" path="/api/webhooks/uptime" />
        <CopyRow label="WATCHTOWER" path="/api/webhooks/watchtower" />
        {isAdmin && webhookSecret !== null && (
          <div style={{ marginTop: 24 }}>
            <div style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em', marginBottom: 8 }}>
              SECRET
            </div>
            <SecretRow secret={webhookSecret} />
          </div>
        )}
      </div>
    </div>
  )
}
