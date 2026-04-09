import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

type Preset = '15m' | '1h' | '6h' | '24h' | '7d'

const PRESETS: { label: string; value: Preset; ms: number }[] = [
  { label: '15m', value: '15m', ms: 15 * 60 * 1000 },
  { label: '1h',  value: '1h',  ms: 60 * 60 * 1000 },
  { label: '6h',  value: '6h',  ms: 6 * 60 * 60 * 1000 },
  { label: '24h', value: '24h', ms: 24 * 60 * 60 * 1000 },
  { label: '7d',  value: '7d',  ms: 7 * 24 * 60 * 60 * 1000 },
]

function getPresetRange(preset: Preset): { start: Date; end: Date } {
  const ms = PRESETS.find(p => p.value === preset)!.ms
  const end = truncateToMinute(new Date())!
  const start = truncateToMinute(new Date(end.getTime() - ms))!
  return { start, end }
}

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
  const initialRange = useMemo(() => getPresetRange('6h'), [])
  const [activePreset, setActivePreset] = useState<Preset | null>('6h')
  const [startInput, setStartInput] = useState(() => formatForInput(initialRange.start))
  const [endInput, setEndInput] = useState(() => formatForInput(initialRange.end))
  const emittedStartRef = useRef<Date | null>(initialRange.start)
  const emittedEndRef = useRef<Date | null>(initialRange.end)
  const onChangeRef = useRef<(range: TimeRange) => void>(() => {})

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  const applyPreset = useCallback((preset: Preset) => {
    const { start, end } = getPresetRange(preset)
    setActivePreset(preset)
    setStartInput(formatForInput(start))
    setEndInput(formatForInput(end))
    emittedStartRef.current = start
    emittedEndRef.current = end
    onChangeRef.current({ start, end })
  }, [])

  useEffect(() => {
    onChangeRef.current(initialRange)
  }, [initialRange])

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
    if (!start || !end) {
      setStartInput(emittedStartRef.current ? formatForInput(emittedStartRef.current) : '')
      setEndInput(emittedEndRef.current ? formatForInput(emittedEndRef.current) : '')
      return
    }
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

  return (
    <div className="time-filter">
      <span className="time-filter-label">TIME</span>
      {PRESETS.map(p => (
        <button
          key={p.value}
          type="button"
          aria-pressed={activePreset === p.value}
          className={activePreset === p.value ? 'time-filter-button time-filter-button-active' : 'time-filter-button'}
          onClick={() => applyPreset(p.value)}
        >
          {p.label}
        </button>
      ))}
      <span className="time-filter-divider">|</span>
      <input
        type="text"
        className="time-filter-input"
        value={startInput}
        onChange={e => handleStartChange(e.target.value)}
        onBlur={commitRange}
        onKeyDown={handleInputKeyDown}
        placeholder="YYYY-MM-DD HH:MM"
        aria-label="Time range start"
      />
      <span className="time-filter-arrow">{'->'}</span>
      <input
        type="text"
        className="time-filter-input"
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
