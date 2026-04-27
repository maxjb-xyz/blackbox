import { useEffect, useId, useRef, useState } from 'react'
import { ArrowRight, Boxes, ExternalLink, ShieldAlert } from 'lucide-react'

import { DEMO_INTRO_DISMISSED_KEY, isDemoIntroDismissed } from '../demoMode'

export default function DemoIntroModal() {
  const [open, setOpen] = useState(false)
  const dismissBtnRef = useRef<HTMLButtonElement>(null)
  const titleId = useId()
  const descId = useId()

  useEffect(() => {
    const dismissed = isDemoIntroDismissed(window.localStorage.getItem(DEMO_INTRO_DISMISSED_KEY))
    if (!dismissed) setOpen(true)
  }, [])

  useEffect(() => {
    if (!open) return
    dismissBtnRef.current?.focus()
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') dismiss()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open])

  function dismiss() {
    window.localStorage.setItem(DEMO_INTRO_DISMISSED_KEY, 'true')
    setOpen(false)
  }

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-5 bg-black/70"
      onClick={dismiss}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descId}
        className="w-full max-w-[680px] bg-[#0B0B0B] border border-[#2A2A2A] shadow-2xl font-mono text-[#E8E8E8]"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center gap-2.5 px-4 py-3 border-b border-[#1E1E1E]">
          <Boxes size={16} />
          <span id={titleId} className="text-xs tracking-[0.18em]">BLACKBOX DEMO</span>
        </div>

        <div id={descId} className="p-4 grid gap-3.5">
          <p className="text-[#B3B3B3] text-xs leading-relaxed">
            Blackbox is an incident timeline for homelabs and self-hosted fleets. It pulls together container churn,
            config changes, and external webhook signals so an outage reads like a coherent event chain instead of raw noise.
          </p>

          <div className="grid gap-2.5">
            <div className="flex gap-2.5 items-start border border-[#1E1E1E] px-3 py-2.5">
              <ShieldAlert size={14} className="shrink-0 mt-0.5 text-[#FF6B6B]" />
              <p className="text-[11px] text-[#B3B3B3] leading-relaxed">
                This demo uses seeded data that stays recent over time. All writes are disabled — forms and admin mutations return demo-mode errors by design.
              </p>
            </div>

            <a
              href="https://github.com/maxjb-xyz/blackbox"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 w-fit border border-[#1E1E1E] px-3 py-2 text-[#E8E8E8] no-underline text-[11px] tracking-[0.08em] hover:border-[#3A3A3A] transition-colors"
            >
              <ExternalLink size={14} />
              VIEW SOURCE
            </a>
          </div>
        </div>

        <div className="px-4 pb-4 flex justify-end">
          <button
            ref={dismissBtnRef}
            type="button"
            onClick={dismiss}
            className="inline-flex items-center gap-2 bg-transparent border border-[#E8E8E8] text-[#E8E8E8] px-3.5 py-2.5 font-mono text-[11px] tracking-[0.14em] cursor-pointer hover:bg-white/5 transition-colors"
          >
            EXPLORE
            <ArrowRight size={14} />
          </button>
        </div>
      </div>
    </div>
  )
}
