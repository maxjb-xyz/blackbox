export type Preset = '15m' | '1h' | '6h' | '24h' | '7d'

export interface PresetTimeRange {
  start: Date
  end: Date
}

export const DEFAULT_TIME_PRESET: Preset = '6h'

export const PRESETS: { label: string; value: Preset; ms: number }[] = [
  { label: '15m', value: '15m', ms: 15 * 60 * 1000 },
  { label: '1h', value: '1h', ms: 60 * 60 * 1000 },
  { label: '6h', value: '6h', ms: 6 * 60 * 60 * 1000 },
  { label: '24h', value: '24h', ms: 24 * 60 * 60 * 1000 },
  { label: '7d', value: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
]

function truncateToMinute(date: Date): Date {
  const truncated = new Date(date)
  truncated.setSeconds(0, 0)
  return truncated
}

export function getPresetRange(preset: Preset): PresetTimeRange {
  const ms = PRESETS.find(p => p.value === preset)!.ms
  const end = truncateToMinute(new Date())
  const start = truncateToMinute(new Date(end.getTime() - ms))
  return { start, end }
}
