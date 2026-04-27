import { useEffect, useState } from 'react'
import { ArrowRight, Boxes, ExternalLink, ShieldAlert } from 'lucide-react'

import { DEMO_INTRO_DISMISSED_KEY, isDemoIntroDismissed } from '../demoMode'

const fontFamily = 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace'

export default function DemoIntroModal() {
  const [open, setOpen] = useState(false)

  useEffect(() => {
    const dismissed = isDemoIntroDismissed(window.localStorage.getItem(DEMO_INTRO_DISMISSED_KEY))
    setOpen(!dismissed)
  }, [])

  if (!open) return null

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 500,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 20,
        background: 'rgba(0, 0, 0, 0.72)',
      }}
    >
      <div
        style={{
          width: 'min(680px, 100%)',
          background: '#0B0B0B',
          border: '1px solid #2A2A2A',
          boxShadow: '0 18px 64px rgba(0, 0, 0, 0.48)',
          fontFamily,
          color: '#E8E8E8',
        }}
      >
        <div style={{ padding: '16px 18px', borderBottom: '1px solid #1E1E1E', display: 'flex', alignItems: 'center', gap: 10 }}>
          <Boxes size={16} />
          <span style={{ fontSize: 12, letterSpacing: '0.18em' }}>BLACKBOX DEMO</span>
        </div>

        <div style={{ padding: 18, display: 'grid', gap: 14 }}>
          <div style={{ color: '#B3B3B3', fontSize: 12, lineHeight: 1.7 }}>
            Blackbox is an incident timeline for homelabs and self-hosted fleets. It pulls together container churn,
            config changes, and external webhook signals so an outage reads like a coherent event chain instead of raw noise.
          </div>

          <div style={{ display: 'grid', gap: 10 }}>
            <div style={{ display: 'flex', gap: 10, alignItems: 'flex-start', border: '1px solid #1E1E1E', padding: '10px 12px' }}>
              <ShieldAlert size={14} style={{ flexShrink: 0, marginTop: 2, color: '#FF6B6B' }} />
              <div style={{ fontSize: 11, color: '#B3B3B3', lineHeight: 1.7 }}>
                This demo uses seeded data that stays recent over time. All writes are disabled, so forms and admin mutations return
                demo-mode errors by design.
              </div>
            </div>

            <a
              href="https://github.com/maxjb-xyz/blackbox"
              target="_blank"
              rel="noreferrer"
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 8,
                width: 'fit-content',
                border: '1px solid #1E1E1E',
                padding: '9px 12px',
                color: '#E8E8E8',
                textDecoration: 'none',
                fontSize: 11,
                letterSpacing: '0.08em',
              }}
            >
              <ExternalLink size={14} />
              VIEW SOURCE
            </a>
          </div>
        </div>

        <div style={{ padding: '0 18px 18px', display: 'flex', justifyContent: 'flex-end' }}>
          <button
            type="button"
            onClick={() => {
              window.localStorage.setItem(DEMO_INTRO_DISMISSED_KEY, 'true')
              setOpen(false)
            }}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 8,
              background: 'transparent',
              border: '1px solid #E8E8E8',
              color: '#E8E8E8',
              padding: '10px 14px',
              fontFamily,
              fontSize: 11,
              letterSpacing: '0.14em',
              cursor: 'pointer',
            }}
          >
            EXPLORE
            <ArrowRight size={14} />
          </button>
        </div>
      </div>
    </div>
  )
}
