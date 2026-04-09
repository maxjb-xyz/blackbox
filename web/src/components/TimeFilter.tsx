import { useCallback, useEffect, useRef, useState } from 'react'

type Preset = '15m' | '1h' | '6h' | '24h' | '7d'

const PRESETS: { label: string; value: Preset; ms: number }[] = [
  { label: '15m', value: '15m', ms: 15 * 60 * 1000 },
  { label: '1h',  value: '1h',  ms: 60 * 60 * 1000 },
  { label: '6h',  value: '6h',  ms: 6 * 60 * 60 * 1000 },
  { label: '24h', value: '24h', ms: 24 * 60 * 60 * 1000 },
  { label: '7d',  value: '7d',  ms: 7 * 24 * 60 * 60 * 1000 },
]

function formatForInput(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function parseInput(value: string): Date | null {
  const d = new Date(value.replace(' ', 'T'))
  return Number.isNaN(d.getTime()) ? null : d
}

export interface TimeRange {
  start: Date | null
  end: Date | null
}

interface TimeFilterProps {
  onChange: (range: TimeRange) => void
}

function sameTimeValue(a: Date | null, b: Date | null): boolean {
  if (!a && !b) return true
  if (!a || !b) return false
  return a.getTime() === b.getTime()
}

function truncateToMinute(date: Date | null): Date | null {
  if (!date) return null
  const truncated = new Date(date)
  truncated.setSeconds(0, 0)
  return truncated
}

export default function TimeFilter({ onChange }: TimeFilterProps) {
  const [activePreset, setActivePreset] = useState<Preset | null>('6h')
  const [startInput, setStartInput] = useState('')
  const [endInput, setEndInput] = useState('')
  const emittedStartRef = useRef<Date | null>(null)
  const emittedEndRef = useRef<Date | null>(null)
  const onChangeRef = useRef<(range: TimeRange) => void>(() => {})

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  const applyPreset = useCallback((preset: Preset) => {
    const ms = PRESETS.find(p => p.value === preset)!.ms
    const end = truncateToMinute(new Date())!
    const start = truncateToMinute(new Date(end.getTime() - ms))!
    setActivePreset(preset)
    setStartInput(formatForInput(start))
    setEndInput(formatForInput(end))
    emittedStartRef.current = start
    emittedEndRef.current = end
    onChangeRef.current({ start, end })
  }, [])

  useEffect(() => {
    applyPreset('6h')
  }, [applyPreset])

  function handleStartChange(value: string) {
    setStartInput(value)
    const start = parseInput(value)
    if (!start) return
    emittedStartRef.current = start
    setActivePreset(null)
    onChangeRef.current({ start, end: emittedEndRef.current })
  }

  function handleEndChange(value: string) {
    setEndInput(value)
    const end = parseInput(value)
    if (!end) return
    emittedEndRef.current = end
    setActivePreset(null)
    onChangeRef.current({ start: emittedStartRef.current, end })
  }

  function commitRange() {
    const start = truncateToMinute(parseInput(startInput))
    const end = truncateToMinute(parseInput(endInput))
    if (sameTimeValue(start, emittedStartRef.current) && sameTimeValue(end, emittedEndRef.current)) {
      return
    }
    emittedStartRef.current = start
    emittedEndRef.current = end
    setActivePreset(null)
    onChangeRef.current({ start, end })
  }

  function handleInputKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key !== 'Enter') return
    commitRange()
  }

  const btnBase: React.CSSProperties = {
    background: 'transparent',
    border: '1px solid #222',
    color: '#555',
    fontFamily: 'inherit',
    fontSize: 10,
    letterSpacing: '0.1em',
    padding: '4px 10px',
    cursor: 'pointer',
    transition: 'all 0.15s',
  }

  const btnActive: React.CSSProperties = {
    ...btnBase,
    borderColor: 'var(--accent)',
    color: 'var(--accent)',
    background: 'rgba(255,51,51,0.06)',
  }

  const inputStyle: React.CSSProperties = {
    background: '#0F0F0F',
    border: '1px solid #222',
    color: '#888',
    fontFamily: 'inherit',
    fontSize: 11,
    padding: '4px 8px',
    width: 152,
    letterSpacing: '0.04em',
  }

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        paddingBottom: 14,
        marginBottom: 4,
        borderBottom: '1px solid #141414',
        flexWrap: 'wrap',
      }}
    >
      <span style={{ fontSize: 10, color: '#555', letterSpacing: '0.12em' }}>TIME</span>
      {PRESETS.map(p => (
        <button
          key={p.value}
          type="button"
          aria-pressed={activePreset === p.value}
          style={activePreset === p.value ? btnActive : btnBase}
          onClick={() => applyPreset(p.value)}
        >
          {p.label}
        </button>
      ))}
      <span style={{ color: '#2a2a2a', fontSize: 12, margin: '0 2px' }}>|</span>
      <input
        type="text"
        style={inputStyle}
        value={startInput}
        onChange={e => handleStartChange(e.target.value)}
        onBlur={commitRange}
        onKeyDown={handleInputKeyDown}
        placeholder="YYYY-MM-DD HH:MM"
        aria-label="Time range start"
      />
      <span style={{ color: '#444', fontSize: 12 }}>{'->'}</span>
      <input
        type="text"
        style={inputStyle}
        value={endInput}
        onChange={e => handleEndChange(e.target.value)}
        onBlur={commitRange}
        onKeyDown={handleInputKeyDown}
        placeholder="YYYY-MM-DD HH:MM"
        aria-label="Time range end"
      />
    </div>
  )
}
