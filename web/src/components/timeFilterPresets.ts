export type Preset = '15m' | '1h' | '6h' | '24h' | '7d'

export type PresetTimeRange = { start: Date; end: Date | null }

export const DEFAULT_TIME_PRESET: Preset = '6h'

export const PRESETS: { label: string; value: Preset; ms: number }[] = [
  { label: '15m', value: '15m', ms: 15 * 60 * 1000 },
  { label: '1h', value: '1h', ms: 60 * 60 * 1000 },
  { label: '6h', value: '6h', ms: 6 * 60 * 60 * 1000 },
  { label: '24h', value: '24h', ms: 24 * 60 * 60 * 1000 },
  { label: '7d', value: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
]

export function truncateToMinute(date: Date | null): Date | null {
  if (!date) return null
  const truncated = new Date(date)
  truncated.setSeconds(0, 0)
  return truncated
}

export function getPresetRange(preset: Preset): PresetTimeRange {
  const presetEntry = PRESETS.find(p => p.value === preset)
  if (!presetEntry) {
    throw new Error(`Unknown time preset: ${preset}`)
  }
  const end = truncateToMinute(new Date())
  if (!end) {
    throw new Error('Failed to truncate end time for preset range')
  }
  const start = truncateToMinute(new Date(end.getTime() - presetEntry.ms))
  if (!start) {
    throw new Error(`Failed to truncate start time for preset range: ${preset}`)
  }
  return { start, end: null }
}
