import { useState } from 'react'
import { CheckCircle, Copy } from 'lucide-react'

function CopyRow({ label, path }: { label: string; path: string }) {
  const [copied, setCopied] = useState(false)
  const url = `${window.location.origin}${path}`

  function handleCopy() {
    navigator.clipboard.writeText(url).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '8px 0', borderBottom: '1px solid var(--border)' }}>
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
  )
}

export default function WebhooksPage() {
  return (
    <div style={{ padding: '0' }}>
      <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--border)', background: 'var(--surface)' }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>WEBHOOKS / ENDPOINTS</span>
      </div>
      <div style={{ padding: '16px' }}>
        <p style={{ color: 'var(--muted)', fontSize: '12px', marginBottom: 16 }}>
          Configure your webhook providers to POST to these endpoints. Set{' '}
          <code style={{ color: 'var(--accent)' }}>X-Webhook-Secret</code> header to the value of{' '}
          <code style={{ color: 'var(--accent)' }}>WEBHOOK_SECRET</code>.
        </p>
        <CopyRow label="UPTIME KUMA" path="/api/webhooks/uptime" />
        <CopyRow label="WATCHTOWER" path="/api/webhooks/watchtower" />
      </div>
    </div>
  )
}
