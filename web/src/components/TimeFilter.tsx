import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { DEFAULT_TIME_PRESET, getPresetRange, PRESETS, truncateToMinute } from './timeFilterPresets'
import type { Preset } from './timeFilterPresets'

function formatForInput(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function formatEndForInput(date: Date | null): string {
  return date ? formatForInput(date) : 'LIVE'
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
  initialRange?: TimeRange
}

function sameTimeValue(a: Date | null, b: Date | null): boolean {
  if (!a && !b) return true
  if (!a || !b) return false
  return a.getTime() === b.getTime()
}

export default function TimeFilter({ onChange, initialRange: providedInitialRange }: TimeFilterProps) {
  const fallbackInitialRange = useMemo(() => getPresetRange(DEFAULT_TIME_PRESET), [])
  const initialStart = providedInitialRange?.start ?? fallbackInitialRange.start
  const initialEnd = providedInitialRange?.end ?? fallbackInitialRange.end
  const initialRange = useMemo<TimeRange>(() => ({ start: initialStart, end: initialEnd }), [initialEnd, initialStart])
  const [activePreset, setActivePreset] = useState<Preset | null>(DEFAULT_TIME_PRESET)
  const [startInput, setStartInput] = useState(() => formatForInput(initialStart))
  const [endInput, setEndInput] = useState(() => formatEndForInput(initialEnd))
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
    setEndInput(formatEndForInput(end))
    emittedStartRef.current = start
    emittedEndRef.current = end
    onChangeRef.current({ start, end })
  }, [])

  useEffect(() => {
    if (
      providedInitialRange &&
      sameTimeValue(providedInitialRange.start, initialRange.start) &&
      sameTimeValue(providedInitialRange.end, initialRange.end)
    ) {
      return
    }
    onChangeRef.current(initialRange)
  }, [initialRange, providedInitialRange])

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
      setEndInput(formatEndForInput(emittedEndRef.current))
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
