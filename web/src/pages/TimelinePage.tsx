import { useCallback, useEffect, useEffectEvent, useLayoutEffect, useRef, useState } from 'react'
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import {
  createNote,
  fetchEntries,
  fetchEntry,
  fetchEntryServices,
  fetchIncidentsForEntryIds,
  fetchNotes,
} from '../api/client'
import type { Entry, EntryNote } from '../api/client'
import { useNodePulse } from '../components/NodePulse'
import TimeFilter from '../components/TimeFilter'
import type { TimeRange } from '../components/TimeFilter'
import { DEFAULT_TIME_PRESET, getPresetRange } from '../components/timeFilterPresets'
import { useWebSocketContext } from '../components/WebSocketProvider'
import { formatLocalTimestamp } from '../utils/time'
import { eventBorderColor, eventTextColor } from '../utils/eventColors'

function isEntry(value: unknown): value is Entry {
  if (typeof value !== 'object' || value === null) return false
  const e = value as Record<string, unknown>
  const requiredKeys: (keyof Entry)[] = ['id', 'timestamp', 'node_name', 'source', 'service', 'event', 'content', 'metadata']
  return requiredKeys.every(key => typeof e[key] === 'string')
}

function extractComposeService(entry: Entry): string | null {
  if (entry.source !== 'docker') return null
  try {
    const meta = JSON.parse(entry.metadata || '{}')
    // Direct event: metadata has the label directly
    const direct = meta['com.docker.compose.service']
    if (direct && direct !== entry.service) return direct
    // Collapsed event: label lives inside raw_events[0].attributes
    const rawEvents = meta['raw_events']
    if (Array.isArray(rawEvents) && rawEvents.length > 0) {
      const fromRaw = rawEvents[0]?.attributes?.['com.docker.compose.service']
      if (fromRaw && fromRaw !== entry.service) return fromRaw
    }
  } catch {
    return null
  }
  return null
}

const SOURCE_OPTIONS = ['', 'docker', 'files', 'systemd', 'agent', 'webhook']

const FILTER_CONTROL_STYLE = {
  background: 'var(--bg)',
  border: '1px solid var(--border)',
  fontSize: '12px',
  padding: '2px 6px',
  fontFamily: 'inherit',
} as const

const ROW_GRID_TEMPLATE = '20px 130px 110px 80px 140px 90px minmax(0, 1fr)'


function formatTimestamp(ts?: string | null) {
  if (!ts) return ''
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return ''
  return formatLocalTimestamp(d)
}

function parsePossibleCause(metadata: string): string | null {
  try {
    const parsed = JSON.parse(metadata)
    return typeof parsed.possible_cause === 'string' ? parsed.possible_cause : null
  } catch {
    return null
  }
}

function formatMetadata(metadata: string) {
  try {
    return JSON.stringify(JSON.parse(metadata || '{}'), null, 2)
  } catch {
    return metadata
  }
}

function parseMetadataObject(metadata: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(metadata || '{}')
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : null
  } catch {
    return null
  }
}

function formatMetadataWithoutDiff(metadata: string) {
  const parsed = parseMetadataObject(metadata)
  if (!parsed || typeof parsed.diff !== 'string') return formatMetadata(metadata)
  const clone = { ...parsed }
  delete clone.diff
  return JSON.stringify(clone, null, 2)
}

function webhookProviderLabel(entry: Entry): string | null {
  if (entry.source !== 'webhook') return null
  const parsed = parseMetadataObject(entry.metadata)
  if (parsed) {
    if (typeof parsed['watchtower.title'] === 'string' || typeof parsed['watchtower.level'] === 'string') {
      return 'watchtower'
    }
    if (typeof parsed.monitor === 'string') {
      return 'kuma'
    }
  }
  if (entry.event === 'update') return 'watchtower'
  if (entry.event === 'up' || entry.event === 'down') return 'kuma'
  return 'webhook'
}

type ViewMode = 'cards' | 'rows'

function getStoredViewMode(): ViewMode {
  return (localStorage.getItem('timeline_view') as ViewMode) ?? 'cards'
}

function getStoredHideHeartbeat(): boolean {
  const v = localStorage.getItem('timeline_hide_heartbeats')
  return v === null ? true : v === 'true'
}

interface TooltipState {
  text: string
  x: number
  y: number
}

interface FileDiffMetadata {
  path: string
  op: string
  diff: string
  diffRedacted: boolean
}

interface DiffRow {
  kind: 'separator' | 'content'
  text?: string
  left?: string
  right?: string
  leftType?: 'context' | 'remove' | 'empty'
  rightType?: 'context' | 'add' | 'empty'
}

function extractFileDiffMetadata(entry: Entry): FileDiffMetadata | null {
  if (entry.source !== 'files') return null
  const parsed = parseMetadataObject(entry.metadata)
  if (!parsed) return null
  if (typeof parsed.path !== 'string' || typeof parsed.op !== 'string' || typeof parsed.diff_status !== 'string') return null
  if (parsed.diff_status !== 'included' || typeof parsed.diff !== 'string' || !parsed.diff.trim()) return null
  return {
    path: parsed.path,
    op: parsed.op,
    diff: parsed.diff,
    diffRedacted: parsed.diff_redacted !== false,
  }
}

const DIFF_STATUS_LABELS: Record<string, string> = {
  no_baseline: 'no baseline snapshot — diff available on next change',
  unchanged: 'file content unchanged',
  skipped_too_large: 'file too large to diff',
  skipped_binary: 'binary file — diff skipped',
  skipped_read_error: 'file read error — diff skipped',
  skipped_too_many_lines: 'too many changed lines to diff',
}

function extractFileDiffStatus(entry: Entry): { path: string; status: string } | null {
  if (entry.source !== 'files') return null
  const parsed = parseMetadataObject(entry.metadata)
  if (!parsed) return null
  if (typeof parsed.path !== 'string' || typeof parsed.diff_status !== 'string') return null
  if (parsed.diff_status === 'included') return null // handled by extractFileDiffMetadata
  return { path: parsed.path, status: parsed.diff_status }
}

function buildDiffRows(diff: string): DiffRow[] {
  const sourceLines = diff.split('\n')
  const rows: DiffRow[] = []

  for (let i = 0; i < sourceLines.length; i++) {
    const line = sourceLines[i]
    if (!line || line === '--- before' || line === '+++ after') continue
    if (line.startsWith('@@')) {
      rows.push({ kind: 'separator', text: line })
      continue
    }
    if (line.startsWith(' ')) {
      const value = line.slice(1)
      rows.push({ kind: 'content', left: value, right: value, leftType: 'context', rightType: 'context' })
      continue
    }

    const removed: string[] = []
    const added: string[] = []
    while (i < sourceLines.length) {
      const current = sourceLines[i]
      if (!current || current === '--- before' || current === '+++ after') {
        i++
        continue
      }
      if (current.startsWith('@@') || current.startsWith(' ')) {
        i--
        break
      }
      if (current.startsWith('-')) removed.push(current.slice(1))
      if (current.startsWith('+')) added.push(current.slice(1))
      i++
    }

    const lineCount = Math.max(removed.length, added.length)
    for (let index = 0; index < lineCount; index++) {
      rows.push({
        kind: 'content',
        left: removed[index] ?? '',
        right: added[index] ?? '',
        leftType: removed[index] !== undefined ? 'remove' : 'empty',
        rightType: added[index] !== undefined ? 'add' : 'empty',
      })
    }
  }

  return rows
}

function isInteractiveEntryTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const interactiveAncestor = target.closest('input, textarea, button, select, option, a, [contenteditable="true"], [role="dialog"]')
  return interactiveAncestor !== null
}

function entryTimestampMs(entry: Entry): number {
  const ts = Date.parse(entry.timestamp)
  return Number.isNaN(ts) ? 0 : ts
}

function entryTimestampSortKey(entry: Entry): string {
  const ts = entry.timestamp?.trim() ?? ''
  const match = ts.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z$/)
  if (!match) return ts
  const [, base, fraction = ''] = match
  return `${base}.${fraction.padEnd(9, '0')}Z`
}

function compareEntries(a: Entry, b: Entry): number {
  const tsDiff = entryTimestampMs(b) - entryTimestampMs(a)
  if (tsDiff !== 0) return tsDiff
  const tsKeyDiff = entryTimestampSortKey(b).localeCompare(entryTimestampSortKey(a))
  if (tsKeyDiff !== 0) return tsKeyDiff
  if (a.id === b.id) return 0
  return a.id < b.id ? 1 : -1
}

function mergeEntries(existing: Entry[], incoming: Entry[]): Entry[] {
  const merged = new Map<string, Entry>()
  for (const entry of existing) merged.set(entry.id, entry)
  for (const entry of incoming) merged.set(entry.id, entry)
  return Array.from(merged.values()).sort(compareEntries)
}

function mergeServiceOptions(existing: string[], incoming: string[]): string[] {
  const merged = new Set(existing)
  for (const service of incoming) {
    if (service) merged.add(service)
  }
  return Array.from(merged).sort((a, b) => a.localeCompare(b))
}

function matchesEntryFilters(
  entry: Entry,
  nodeFilter: string,
  sourceFilter: string,
  serviceFilter: string,
  qFilter: string,
  hideHeartbeat: boolean,
): boolean {
  if (hideHeartbeat && entry.source === 'agent' && entry.event === 'heartbeat') return false
  if (nodeFilter && entry.node_name !== nodeFilter) return false
  if (sourceFilter && entry.source !== sourceFilter) return false
  if (serviceFilter && entry.service !== serviceFilter) return false
  if (qFilter) {
    const q = qFilter.toLowerCase()
    const haystacks = [entry.content, entry.service].filter(Boolean).map(value => value.toLowerCase())
    if (!haystacks.some(value => value.includes(q))) return false
  }
  return true
}

interface SearchableSelectProps {
  value: string
  options: string[]
  placeholder: string
  onChange: (value: string) => void
}

function SearchableSelect({ value, options, placeholder, onChange }: SearchableSelectProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [highlightedIndex, setHighlightedIndex] = useState(0)

  const rootRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([])
  const listboxId = 'timeline-service-filter-options'

  const filteredOptions = options.filter(option => option.toLowerCase().includes(query.trim().toLowerCase()))
  const highlightedOptionIndex = (() => {
    if (filteredOptions.length === 0) return 0
    return Math.min(highlightedIndex, filteredOptions.length - 1)
  })()

  useEffect(() => {
    if (!isOpen) return

    function handlePointerDown(event: MouseEvent) {
      if (rootRef.current?.contains(event.target as Node)) return
      setIsOpen(false)
      setQuery('')
    }

    document.addEventListener('mousedown', handlePointerDown)
    return () => document.removeEventListener('mousedown', handlePointerDown)
  }, [isOpen])

  useEffect(() => {
    if (!isOpen) return
    const frame = requestAnimationFrame(() => inputRef.current?.focus())
    return () => cancelAnimationFrame(frame)
  }, [isOpen])

  useEffect(() => {
    if (!isOpen) return
    optionRefs.current[highlightedOptionIndex]?.scrollIntoView({ block: 'nearest' })
  }, [highlightedOptionIndex, isOpen])

  function openMenu(nextQuery = '') {
    const nextOptions = options.filter(option => option.toLowerCase().includes(nextQuery.trim().toLowerCase()))
    const selectedIndex = value ? nextOptions.findIndex(option => option === value) : -1
    setHighlightedIndex(selectedIndex >= 0 ? selectedIndex : 0)
    setQuery(nextQuery)
    setIsOpen(true)
  }

  function closeMenu() {
    setIsOpen(false)
    setQuery('')
  }

  function handleSelect(option: string) {
    onChange(option)
    closeMenu()
  }

  function handleInputKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setHighlightedIndex(Math.min(highlightedOptionIndex + 1, Math.max(filteredOptions.length - 1, 0)))
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      setHighlightedIndex(Math.max(highlightedOptionIndex - 1, 0))
      return
    }
    if (event.key === 'Enter') {
      event.preventDefault()
      const nextOption = filteredOptions[highlightedOptionIndex]
      if (nextOption) handleSelect(nextOption)
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      closeMenu()
    }
  }

  function handleTriggerKeyDown(event: React.KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      openMenu()
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      closeMenu()
      return
    }
    if (event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey) {
      event.preventDefault()
      openMenu(event.key)
    }
  }

  return (
    <div ref={rootRef} style={{ position: 'relative', width: 140 }}>
      <button
        type="button"
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label="Service filter"
        onClick={() => {
          if (isOpen) closeMenu()
          else openMenu()
        }}
        onKeyDown={handleTriggerKeyDown}
        style={{
          ...FILTER_CONTROL_STYLE,
          width: '100%',
          color: value ? 'var(--text)' : 'var(--muted)',
          cursor: 'pointer',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          paddingRight: value ? 42 : 22,
        }}
      >
        <span
          style={{
            minWidth: 0,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {value || placeholder}
        </span>
      </button>

      {value && (
        <button
          type="button"
          title="Clear service filter"
          aria-label="Clear service filter"
          onClick={event => {
            event.stopPropagation()
            onChange('')
            closeMenu()
          }}
          style={{
            position: 'absolute',
            top: '50%',
            right: 20,
            transform: 'translateY(-50%)',
            background: 'none',
            border: 'none',
            color: 'var(--muted)',
            fontSize: '12px',
            lineHeight: 1,
            cursor: 'pointer',
            padding: 0,
            fontFamily: 'inherit',
          }}
        >
          ×
        </button>
      )}

      <span
        aria-hidden="true"
        style={{
          position: 'absolute',
          top: '50%',
          right: 8,
          transform: 'translateY(-50%)',
          color: 'var(--muted)',
          fontSize: '10px',
          pointerEvents: 'none',
        }}
      >
        {isOpen ? '^' : 'v'}
      </span>

      {isOpen && (
        <div
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            right: 0,
            zIndex: 20,
            background: 'var(--surface)',
            border: '1px solid var(--border)',
            boxShadow: '0 10px 24px rgba(0, 0, 0, 0.45)',
          }}
        >
          <div style={{ padding: 6, borderBottom: '1px solid var(--border)' }}>
            <input
              ref={inputRef}
              type="text"
              role="combobox"
              aria-expanded={isOpen}
              aria-controls={listboxId}
              aria-activedescendant={filteredOptions[highlightedOptionIndex] ? `${listboxId}-option-${highlightedOptionIndex}` : undefined}
              value={query}
              onChange={event => setQuery(event.target.value)}
              onKeyDown={handleInputKeyDown}
              placeholder="FILTER SERVICES..."
              style={{
                ...FILTER_CONTROL_STYLE,
                width: '100%',
                color: 'var(--text)',
                padding: '4px 8px',
              }}
            />
          </div>

          <div id={listboxId} role="listbox" style={{ maxHeight: 200, overflowY: 'auto' }}>
            {filteredOptions.length === 0 ? (
              <div
                style={{
                  padding: '8px',
                  color: 'var(--muted)',
                  fontSize: '11px',
                  letterSpacing: '0.05em',
                }}
              >
                NO MATCHES
              </div>
            ) : (
              filteredOptions.map((option, index) => (
                <button
                  ref={element => { optionRefs.current[index] = element }}
                  id={`${listboxId}-option-${index}`}
                  key={option}
                  type="button"
                  role="option"
                  aria-selected={option === value}
                  onClick={() => handleSelect(option)}
                  onMouseEnter={() => setHighlightedIndex(index)}
                  style={{
                    width: '100%',
                    background: index === highlightedOptionIndex ? 'var(--bg)' : 'transparent',
                    color: option === value ? 'var(--accent)' : 'var(--text)',
                    border: 'none',
                    borderBottom: index === filteredOptions.length - 1 ? 'none' : '1px solid var(--border)',
                    padding: '6px 8px',
                    textAlign: 'left',
                    fontFamily: 'inherit',
                    fontSize: '11px',
                    cursor: 'pointer',
                  }}
                >
                  {option}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export default function TimelinePage() {
  const location = useLocation()
  const [searchParams, setSearchParams] = useSearchParams()
  const { nodes } = useNodePulse()
  const [viewMode, setViewMode] = useState<ViewMode>(getStoredViewMode)
  const [hideHeartbeat, setHideHeartbeat] = useState<boolean>(getStoredHideHeartbeat)
  const [serviceOptions, setServiceOptions] = useState<string[]>([])
  const [timeRange, setTimeRange] = useState<TimeRange>(() => {
    const fromParam = searchParams.get('from')
    const toParam = searchParams.get('to')
    if (fromParam || toParam) {
      const start = fromParam ? new Date(fromParam) : null
      const end = toParam ? new Date(toParam) : null
      const validStart = start && !Number.isNaN(start.getTime()) ? start : null
      const validEnd = end && !Number.isNaN(end.getTime()) ? end : null
      if (validStart || validEnd) return { start: validStart, end: validEnd }
    }
    return getPresetRange(DEFAULT_TIME_PRESET)
  })
  const [visibleCount, setVisibleCount] = useState(0)

  const serviceMountedRef = useRef(true)
  const serviceRefreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const serviceRefreshPromiseRef = useRef<Promise<void> | null>(null)
  const serviceRefreshQueuedRef = useRef(false)

  const nodeFilter = searchParams.get('node') ?? ''
  const sourceFilter = searchParams.get('source') ?? ''
  const serviceFilter = searchParams.get('service') ?? ''
  const qFilter = searchParams.get('q') ?? ''
  const focusEntry = (() => {
    const state = location.state
    if (
      state !== null &&
      typeof state === 'object' &&
      'focusEntry' in state &&
      isEntry((state as { focusEntry: unknown }).focusEntry)
    ) {
      return (state as { focusEntry: Entry }).focusEntry
    }
    return null
  })()

  const refreshServices = useCallback(function scheduleServiceRefresh() {
    if (serviceRefreshTimeoutRef.current) return

    serviceRefreshTimeoutRef.current = setTimeout(() => {
      serviceRefreshTimeoutRef.current = null

      if (serviceRefreshPromiseRef.current) {
        serviceRefreshQueuedRef.current = true
        return
      }

      const refreshPromise = fetchEntryServices()
        .then(({ services }) => {
          if (!serviceMountedRef.current) return
          setServiceOptions(prev => mergeServiceOptions(prev, services))
        })
        .catch(err => {
          if (serviceMountedRef.current) console.error('fetchEntryServices:', err)
        })
        .finally(() => {
          serviceRefreshPromiseRef.current = null
          if (serviceRefreshQueuedRef.current) {
            serviceRefreshQueuedRef.current = false
            scheduleServiceRefresh()
          }
        })

      serviceRefreshPromiseRef.current = refreshPromise
    }, 150)
  }, [])

  useEffect(() => {
    serviceMountedRef.current = true
    refreshServices()
    return () => {
      serviceMountedRef.current = false
      if (serviceRefreshTimeoutRef.current) {
        clearTimeout(serviceRefreshTimeoutRef.current)
        serviceRefreshTimeoutRef.current = null
      }
    }
  }, [refreshServices])

  function setFilter(key: string, value: string) {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      if (value) next.set(key, value)
      else next.delete(key)
      return next
    })
  }

  function selectViewMode(mode: ViewMode) {
    localStorage.setItem('timeline_view', mode)
    setViewMode(mode)
  }

  function toggleHideHeartbeat() {
    setHideHeartbeat(prev => {
      const next = !prev
      localStorage.setItem('timeline_hide_heartbeats', String(next))
      return next
    })
  }

  return (
    <div style={{ height: 'calc(100vh - var(--topbar-height))', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <TimeFilter initialRange={timeRange} onChange={setTimeRange} />
      <div
        style={{
          display: 'flex',
          gap: 16,
          padding: '8px 24px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--surface)',
          alignItems: 'center',
          flexShrink: 0,
          flexWrap: 'wrap',
        }}
      >
        {/* SOURCE */}
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <span style={{ color: '#555', fontSize: 10, letterSpacing: '0.12em' }}>SOURCE</span>
          <select
            value={sourceFilter}
            onChange={e => setFilter('source', e.target.value)}
            style={{ ...FILTER_CONTROL_STYLE, color: sourceFilter ? 'var(--text)' : 'var(--muted)' }}
          >
            {SOURCE_OPTIONS.map(source => (
              <option key={source} value={source}>
                {source ? source.toUpperCase() : 'ALL'}
              </option>
            ))}
          </select>
        </span>

        {/* SERVICE */}
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <span style={{ color: '#555', fontSize: 10, letterSpacing: '0.12em' }}>SERVICE</span>
          <SearchableSelect
            value={serviceFilter}
            options={serviceOptions}
            placeholder="ALL"
            onChange={value => setFilter('service', value)}
          />
        </span>

        {/* NODE */}
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <span style={{ color: '#555', fontSize: 10, letterSpacing: '0.12em' }}>NODE</span>
          <select
            value={nodeFilter}
            onChange={e => setFilter('node', e.target.value)}
            style={{ ...FILTER_CONTROL_STYLE, color: nodeFilter ? 'var(--text)' : 'var(--muted)' }}
          >
            <option value="">ALL</option>
            {nodes.map(node => (
              <option key={node.id} value={node.name}>{node.name}</option>
            ))}
          </select>
        </span>

        {/* SEARCH */}
        <input
          type="text"
          placeholder="SEARCH..."
          value={qFilter}
          onChange={e => setFilter('q', e.target.value)}
          style={{ ...FILTER_CONTROL_STYLE, color: 'var(--text)', padding: '2px 8px', width: 160 }}
        />

        {(nodeFilter || sourceFilter || serviceFilter || qFilter) && (
          <button
            type="button"
            onClick={() => {
              setSearchParams(prev => {
                const next = new URLSearchParams(prev)
                next.delete('node')
                next.delete('source')
                next.delete('service')
                next.delete('q')
                return next
              })
            }}
            style={{
              background: 'none',
              border: 'none',
              padding: 0,
              color: '#555',
              fontSize: '11px',
              cursor: 'pointer',
              letterSpacing: '0.08em',
              fontFamily: 'inherit',
            }}
          >
            CLEAR
          </button>
        )}

        {/* Right side */}
        <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 16 }}>
          {/* Heartbeat checkbox */}
          <label style={{ display: 'inline-flex', alignItems: 'center', gap: 6, cursor: 'pointer', userSelect: 'none' }}>
            <input
              type="checkbox"
              checked={hideHeartbeat}
              onChange={toggleHideHeartbeat}
              style={{ accentColor: 'var(--accent)', width: 12, height: 12, cursor: 'pointer' }}
            />
            <span style={{ fontSize: 11, letterSpacing: '0.08em', color: 'var(--muted)' }}>HIDE HEARTBEATS</span>
          </label>

          {/* Entry count */}
          <span style={{ fontSize: 11, color: '#555', letterSpacing: '0.08em' }}>
            {visibleCount} {visibleCount === 1 ? 'ENTRY' : 'ENTRIES'}
          </span>

          {/* CARDS / ROWS */}
          <div style={{ display: 'flex' }}>
            {(['cards', 'rows'] as ViewMode[]).map((mode, i) => (
              <button
                key={mode}
                onClick={() => selectViewMode(mode)}
                style={{
                  background: viewMode === mode ? 'rgba(255,51,51,0.08)' : 'none',
                  border: '1px solid var(--border)',
                  borderLeft: i === 1 ? 'none' : '1px solid var(--border)',
                  color: viewMode === mode ? 'var(--accent)' : 'var(--muted)',
                  fontSize: '12px',
                  padding: '2px 10px',
                  fontFamily: 'inherit',
                  cursor: 'pointer',
                  letterSpacing: '0.08em',
                }}
              >
                {mode.toUpperCase()}
              </button>
            ))}
          </div>
        </div>
      </div>

      <TimelineFeed
        nodeFilter={nodeFilter}
        sourceFilter={sourceFilter}
        serviceFilter={serviceFilter}
        qFilter={qFilter}
        hideHeartbeat={hideHeartbeat}
        viewMode={viewMode}
        focusEntry={focusEntry}
        onEntriesChanged={refreshServices}
        onCountChanged={setVisibleCount}
        timeStart={timeRange.start}
        timeEnd={timeRange.end}
      />
    </div>
  )
}

interface TimelineFeedProps {
  nodeFilter: string
  sourceFilter: string
  serviceFilter: string
  qFilter: string
  hideHeartbeat: boolean
  viewMode: ViewMode
  focusEntry: Entry | null
  onEntriesChanged: () => void
  onCountChanged: (count: number) => void
  timeStart?: Date | null
  timeEnd?: Date | null
}

function TimelineFeed({
  nodeFilter,
  sourceFilter,
  serviceFilter,
  qFilter,
  hideHeartbeat,
  viewMode,
  focusEntry,
  onEntriesChanged,
  onCountChanged,
  timeStart,
  timeEnd,
}: TimelineFeedProps) {
  const { lastMessage } = useWebSocketContext()
  const [entries, setEntries] = useState<Entry[]>([])
  const [nextCursor, setNextCursor] = useState<string | undefined>()
  const [loading, setLoading] = useState(true)
  const [done, setDone] = useState(false)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [ghostEntry, setGhostEntry] = useState<Entry | null>(null)
  const [pinnedEntry, setPinnedEntry] = useState<Entry | null>(null)
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)
  const [sentinelVisible, setSentinelVisible] = useState(false)
  const [entryIncidentMap, setEntryIncidentMap] = useState<Record<string, { id: string; confidence: string }>>({})

  const sentinelRef = useRef<HTMLDivElement>(null)
  const renderedIdsRef = useRef<Set<string>>(new Set())
  const expandedIdRef = useRef<string | null>(null)
  const ghostEntryRef = useRef<Entry | null>(null)
  const visibleEntryIDsRef = useRef<string[]>([])
  const mountedRef = useRef(true)
  const pageRequestIdRef = useRef(0)
  const entryIncidentMapReqIdRef = useRef(0)
  const timeStartMs = timeStart?.getTime() ?? null
  const timeEndMs = timeEnd?.getTime() ?? null

  const consumeMaterializedGhost = useEffectEvent((incoming: Entry[]) => {
    const ghost = ghostEntryRef.current
    if (!ghost) return false
    if (!incoming.some(entry => entry.id === ghost.id)) return false
    renderedIdsRef.current.delete(ghost.id)
    ghostEntryRef.current = null
    setGhostEntry(null)
    return true
  })

  const loadIncidentMembership = useEffectEvent(async (entryIDs: string[]) => {
    const requestId = entryIncidentMapReqIdRef.current + 1
    entryIncidentMapReqIdRef.current = requestId

    if (viewMode !== 'rows' || entryIDs.length === 0) {
      if (mountedRef.current && entryIncidentMapReqIdRef.current === requestId) {
        setEntryIncidentMap({})
      }
      return
    }

    try {
      const map = await fetchIncidentsForEntryIds(entryIDs)
      if (mountedRef.current && entryIncidentMapReqIdRef.current === requestId) {
        setEntryIncidentMap(map)
      }
    } catch {
      if (mountedRef.current && entryIncidentMapReqIdRef.current === requestId) {
        setEntryIncidentMap({})
      }
    }
  })

  const loadPage = useEffectEvent(async (cursor?: string) => {
    const requestId = cursor ? pageRequestIdRef.current : pageRequestIdRef.current + 1
    if (!cursor) pageRequestIdRef.current = requestId
    setLoading(true)
    try {
      const page = await fetchEntries({
        cursor,
        limit: 50,
        node: nodeFilter || undefined,
        source: sourceFilter || undefined,
        service: serviceFilter || undefined,
        q: qFilter || undefined,
        hideHeartbeat,
        timeStart,
        timeEnd,
      })
      if (!mountedRef.current || requestId !== pageRequestIdRef.current) return

      consumeMaterializedGhost(page.entries)

      if (!cursor) {
        const mergedEntries = mergeEntries([], page.entries)
        renderedIdsRef.current = new Set(mergedEntries.map(entry => entry.id))
        setEntries(mergedEntries)
      } else {
        setEntries(prev => {
          const mergedEntries = mergeEntries(prev, page.entries)
          renderedIdsRef.current = new Set(mergedEntries.map(entry => entry.id))
          return mergedEntries
        })
      }

      setNextCursor(page.next_cursor)
      setDone(!page.next_cursor)
      onEntriesChanged()
    } catch (err) {
      if (mountedRef.current && requestId === pageRequestIdRef.current) {
        console.error(cursor ? 'loadMore:' : 'loadEntries:', err)
      }
    } finally {
      if (mountedRef.current && requestId === pageRequestIdRef.current) setLoading(false)
    }
  })

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  useEffect(() => {
    if (ghostEntryRef.current) {
      renderedIdsRef.current.delete(ghostEntryRef.current.id)
      ghostEntryRef.current = null
    }
    expandedIdRef.current = null
    setExpandedId(null)
    setGhostEntry(null)
    setPinnedEntry(null)
    setTooltip(null)
    setNextCursor(undefined)
    setDone(false)
    setEntryIncidentMap({})
    void loadPage()
  }, [hideHeartbeat, nodeFilter, qFilter, serviceFilter, sourceFilter, timeEndMs, timeStartMs])

  useEffect(() => {
    if (!focusEntry) return
    if (ghostEntryRef.current) {
      renderedIdsRef.current.delete(ghostEntryRef.current.id)
      ghostEntryRef.current = null
    }
    renderedIdsRef.current.delete(focusEntry.id)
    expandedIdRef.current = focusEntry.id
    setExpandedId(focusEntry.id)
    setGhostEntry(null)
    setPinnedEntry(current => (current?.id === focusEntry.id ? current : focusEntry))
  }, [focusEntry])

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel) return
    const observer = new IntersectionObserver(
      obs => { setSentinelVisible(Boolean(obs[0]?.isIntersecting)) },
      { rootMargin: '200px' },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (!lastMessage) return

    if (lastMessage.type === 'entry_replaced') {
      const { entry } = lastMessage.data as { old_id: string; entry: Entry }
      onEntriesChanged()
      setEntries(prev => {
        if (!prev.some(existing => existing.id === entry.id)) return prev
        return prev.map(existing => (existing.id === entry.id ? entry : existing))
      })
      return
    }

    if (lastMessage.type !== 'entry') return
    onEntriesChanged()
    const newEntry = lastMessage.data as Entry
    const materializedGhost = ghostEntryRef.current?.id === newEntry.id
    if (renderedIdsRef.current.has(newEntry.id) && !materializedGhost) return
    if (!matchesEntryFilters(newEntry, nodeFilter, sourceFilter, serviceFilter, qFilter, hideHeartbeat)) return
    if (materializedGhost) {
      renderedIdsRef.current.delete(newEntry.id)
      ghostEntryRef.current = null
      setGhostEntry(null)
    }
    setEntries(prev => {
      const mergedEntries = mergeEntries(prev, [newEntry])
      renderedIdsRef.current = new Set(mergedEntries.map(entry => entry.id))
      return mergedEntries
    })
  }, [hideHeartbeat, lastMessage, nodeFilter, onEntriesChanged, qFilter, serviceFilter, sourceFilter])

  function handleRowClick(entry: Entry) {
    if (expandedId === entry.id) {
      if (ghostEntryRef.current) {
        renderedIdsRef.current.delete(ghostEntryRef.current.id)
        ghostEntryRef.current = null
      }
      expandedIdRef.current = null
      setExpandedId(null)
      setGhostEntry(null)
      if (pinnedEntry?.id === entry.id) {
        setPinnedEntry(null)
      }
      return
    }
    const requestedEntryId = entry.id
    if (ghostEntryRef.current) {
      renderedIdsRef.current.delete(ghostEntryRef.current.id)
      ghostEntryRef.current = null
    }
    if (pinnedEntry?.id && pinnedEntry.id !== requestedEntryId) {
      setPinnedEntry(null)
    }
    expandedIdRef.current = requestedEntryId
    setExpandedId(requestedEntryId)
    setGhostEntry(null)

    if (entry.correlated_id) {
      const alreadyInDom = entries.find(item => item.id === entry.correlated_id)
      if (!alreadyInDom) {
        fetchEntry(entry.correlated_id)
          .then(ghost => {
            if (expandedIdRef.current !== requestedEntryId || renderedIdsRef.current.has(ghost.id)) return
            renderedIdsRef.current.add(ghost.id)
            ghostEntryRef.current = ghost
            setGhostEntry(ghost)
          })
          .catch(() => {})
      }
    }
  }

  const handleOverlayClick = useCallback(() => {
    if (!expandedIdRef.current) return
    const prevExpandedId = expandedIdRef.current
    if (ghostEntryRef.current) {
      renderedIdsRef.current.delete(ghostEntryRef.current.id)
      ghostEntryRef.current = null
    }
    setPinnedEntry(current => (current?.id === prevExpandedId ? null : current))
    expandedIdRef.current = null
    setExpandedId(null)
    setGhostEntry(null)
  }, [])

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      if (!expandedIdRef.current) return
      const prevExpandedId = expandedIdRef.current
      if (ghostEntryRef.current) {
        renderedIdsRef.current.delete(ghostEntryRef.current.id)
        ghostEntryRef.current = null
      }
      setPinnedEntry(current => (current?.id === prevExpandedId ? null : current))
      expandedIdRef.current = null
      setExpandedId(null)
      setGhostEntry(null)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const displayEntries = (() => {
    const latestPinnedEntry = pinnedEntry
      ? entries.find(entry => entry.id === pinnedEntry.id) ?? pinnedEntry
      : null
    const result = latestPinnedEntry
      ? [latestPinnedEntry, ...entries.filter(entry => entry.id !== latestPinnedEntry.id)]
      : [...entries]
    if (expandedId && ghostEntry) {
      const expandedIndex = result.findIndex(entry => entry.id === expandedId)
      if (expandedIndex !== -1) {
        result.splice(expandedIndex, 0, ghostEntry)
      }
    }
    return result
  })()
  const filteredEntries = displayEntries.filter(entry => {
    if (pinnedEntry?.id === entry.id) return true
    return matchesEntryFilters(entry, nodeFilter, sourceFilter, serviceFilter, qFilter, hideHeartbeat)
  })
  const timeFilteredEntries = filteredEntries.filter(entry => {
    if (pinnedEntry?.id === entry.id) return true
    if (!timeStart && !timeEnd) return true
    const ts = new Date(entry.timestamp)
    if (timeStart && ts < timeStart) return false
    if (timeEnd && ts > timeEnd) return false
    return true
  })
  const expandedVisibleInFilteredEntries = expandedId == null || timeFilteredEntries.some(entry => entry.id === expandedId)
  const reachedFilteredEnd = done
  const visibleEntryIDs = timeFilteredEntries.map(entry => entry.id)
  const visibleEntryIDsKey = visibleEntryIDs.join('|')
  visibleEntryIDsRef.current = visibleEntryIDs

  useEffect(() => {
    if (expandedVisibleInFilteredEntries) return
    if (ghostEntryRef.current) {
      renderedIdsRef.current.delete(ghostEntryRef.current.id)
      ghostEntryRef.current = null
    }
    expandedIdRef.current = null
    setExpandedId(null)
    setGhostEntry(null)
  }, [expandedVisibleInFilteredEntries])

  useLayoutEffect(() => {
    onCountChanged(timeFilteredEntries.length)
  }, [onCountChanged, timeFilteredEntries.length])

  useEffect(() => {
    if (loading || done || !nextCursor || !sentinelVisible) return
    void loadPage(nextCursor)
  }, [done, loading, nextCursor, sentinelVisible])

  useEffect(() => {
    void loadIncidentMembership(visibleEntryIDsRef.current)
  }, [viewMode, visibleEntryIDsKey])

  useEffect(() => {
    if (!lastMessage) return
    if (lastMessage.type === 'incident_opened' || lastMessage.type === 'incident_updated' || lastMessage.type === 'incident_resolved') {
      void loadIncidentMembership(visibleEntryIDsRef.current)
    }
  }, [lastMessage, visibleEntryIDsKey])

  return (
    <>
      {viewMode === 'rows' && (
        <div
          className="timeline-header-row"
          style={{
            display: 'grid',
            gridTemplateColumns: ROW_GRID_TEMPLATE,
            gap: '0 8px',
            padding: '4px 24px',
            borderBottom: '1px solid var(--border)',
            background: 'var(--surface)',
            color: 'var(--muted)',
            fontSize: '11px',
            letterSpacing: '0.1em',
            flexShrink: 0,
          }}
        >
          <span /><span>TIMESTAMP</span><span>NODE</span><span>SOURCE</span>
          <span>SERVICE</span><span>EVENT</span><span>CONTENT</span>
        </div>
      )}

      <div
        style={{ flex: 1, overflowY: 'auto', position: 'relative' }}
        onClick={e => {
          if ((e.target as HTMLElement).closest('[data-row]')) return
          handleOverlayClick()
        }}
      >
        {viewMode === 'cards' ? (
          <div style={{ maxWidth: 760, margin: '0 auto', padding: '16px 24px', display: 'flex', flexDirection: 'column', gap: 10 }}>
            <AnimatePresence initial={false}>
              {timeFilteredEntries.map(entry => (
                <TimelineCard
                  key={entry.id}
                  entry={entry}
                  isExpanded={expandedId === entry.id}
                  isDimmed={expandedId !== null && expandedId !== entry.id}
                  isGhost={ghostEntry?.id === entry.id}
                  onClick={() => handleRowClick(entry)}
                  onTooltip={setTooltip}
                  onTooltipClear={() => setTooltip(null)}
                />
              ))}
            </AnimatePresence>
          </div>
        ) : (
          <AnimatePresence initial={false}>
            {timeFilteredEntries.map(entry => (
              <TimelineRow
                key={entry.id}
                entry={entry}
                isExpanded={expandedId === entry.id}
                isDimmed={expandedId !== null && expandedId !== entry.id}
                isGhost={ghostEntry?.id === entry.id}
                incident={entryIncidentMap[entry.id]}
                onClick={() => handleRowClick(entry)}
                onTooltip={setTooltip}
                onTooltipClear={() => setTooltip(null)}
              />
            ))}
          </AnimatePresence>
        )}

        <div ref={sentinelRef} style={{ height: 1 }} />

        {loading && (
          <div style={{ padding: '8px 24px', color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
        )}
        {reachedFilteredEnd && !loading && timeFilteredEntries.length > 0 && (
          <div style={{ padding: '8px 24px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
            - end of timeline -
          </div>
        )}
        {reachedFilteredEnd && !loading && entries.length === 0 && (
          <div style={{ padding: '24px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
            no entries found
          </div>
        )}
        {reachedFilteredEnd && !loading && entries.length > 0 && timeFilteredEntries.length === 0 && (
          <div style={{ padding: '24px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
            no entries in selected time range
          </div>
        )}
      </div>

      {tooltip && (
        <div className="tooltip-portal" style={{ top: tooltip.y - 8, left: tooltip.x + 12 }}>
          {tooltip.text}
        </div>
      )}
    </>
  )
}

interface EntryProps {
  entry: Entry
  isExpanded: boolean
  isDimmed: boolean
  isGhost: boolean
  incident?: { id: string; confidence: string }
  onClick: () => void
  onTooltip: (tooltip: TooltipState) => void
  onTooltipClear: () => void
}

function ExpandableSection({ isOpen, children }: { isOpen: boolean, children: React.ReactNode }) {
  return (
    <AnimatePresence initial={false}>
      {isOpen && (
        <motion.div
          initial={{ height: 0, opacity: 0 }}
          animate={{ height: 'auto', opacity: 1 }}
          exit={{ height: 0, opacity: 0 }}
          transition={{
            height: { duration: 0.16, ease: 'easeOut' },
            opacity: { duration: 0.12, ease: 'easeOut' },
          }}
          style={{ overflow: 'hidden', width: '100%' }}
        >
          {children}
        </motion.div>
      )}
    </AnimatePresence>
  )
}

function ExpandedDetails({ entry }: { entry: Entry }) {
  const [notes, setNotes] = useState<EntryNote[]>([])
  const [notesLoaded, setNotesLoaded] = useState(false)
  const [noteInput, setNoteInput] = useState('')
  const [noteLoading, setNoteLoading] = useState(false)
  const [metaExpanded, setMetaExpanded] = useState(false)
  const [diffOpen, setDiffOpen] = useState(false)

  useEffect(() => {
    fetchNotes(entry.id)
      .then(n => { setNotes(n); setNotesLoaded(true) })
      .catch(() => setNotesLoaded(true))
  }, [entry.id])

  async function handleAddNote() {
    if (!noteInput.trim() || noteLoading) return
    setNoteLoading(true)
    try {
      const note = await createNote(entry.id, noteInput.trim())
      setNotes(prev => [...prev, note])
      setNoteInput('')
    } catch (err) {
      console.error('addNote:', err)
    } finally {
      setNoteLoading(false)
    }
  }

  const fileDiff = extractFileDiffMetadata(entry)
  const fileDiffStatus = !fileDiff ? extractFileDiffStatus(entry) : null
  const formattedMeta = entry.metadata && entry.metadata !== '{}'
    ? formatMetadataWithoutDiff(entry.metadata)
    : null

  return (
    <div style={{ marginTop: 10, paddingTop: 10, borderTop: '1px solid var(--border)' }}>
      {fileDiff && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
            <div>
              <div style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em', marginBottom: 4 }}>FILE DIFF</div>
              <div style={{ color: 'var(--text)', fontSize: '12px', wordBreak: 'break-all' }}>{fileDiff.path}</div>
            </div>
            <button
              type="button"
              onClick={e => { e.stopPropagation(); setDiffOpen(true) }}
              style={{
                background: 'none',
                border: '1px solid var(--accent)',
                color: 'var(--accent)',
                padding: '6px 10px',
                fontFamily: 'inherit',
                fontSize: '11px',
                letterSpacing: '0.08em',
                cursor: 'pointer',
              }}
            >
              OPEN DIFF
            </button>
          </div>
          <div style={{ color: 'var(--muted)', fontSize: '11px', marginTop: 6 }}>
            {fileDiff.diffRedacted ? 'diff is redacted per current agent setting' : 'diff captured without redaction'}
          </div>
          <DiffModal entry={entry} diff={fileDiff} open={diffOpen} onClose={() => setDiffOpen(false)} />
        </div>
      )}

      {fileDiffStatus && (
        <div style={{ marginBottom: 12, padding: '8px 10px', border: '1px solid var(--border)', background: 'rgba(255,255,255,0.02)' }}>
          <div style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.12em', marginBottom: 4 }}>FILE DIFF</div>
          <div style={{ color: 'var(--muted)', fontSize: '12px', wordBreak: 'break-all', marginBottom: 4 }}>{fileDiffStatus.path}</div>
          <div style={{ fontSize: '11px', color: 'var(--muted)', fontStyle: 'italic' }}>
            {DIFF_STATUS_LABELS[fileDiffStatus.status] ?? fileDiffStatus.status}
          </div>
        </div>
      )}

      {formattedMeta && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em', marginBottom: 4 }}>METADATA</div>
          <div style={{ position: 'relative' }}>
            <pre
              style={{
                color: 'var(--text)',
                fontSize: '12px',
                margin: 0,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                maxHeight: metaExpanded ? 'none' : 240,
                overflow: 'hidden',
              }}
            >
              {formattedMeta}
            </pre>
            {formattedMeta.length > 500 && (
              <button
                onClick={e => { e.stopPropagation(); setMetaExpanded(p => !p) }}
                style={{
                  background: 'none',
                  border: 'none',
                  color: 'var(--accent)',
                  fontSize: '11px',
                  cursor: 'pointer',
                  padding: '4px 0 0 0',
                  fontFamily: 'inherit',
                  letterSpacing: '0.05em',
                }}
              >
                {metaExpanded ? 'SHOW LESS' : 'SHOW MORE'}
              </button>
            )}
          </div>
        </div>
      )}

      <div>
        <div style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em', marginBottom: 6 }}>NOTES</div>
        {!notesLoaded && (
          <div style={{ marginBottom: 8 }}>
            {[80, 60, 72].map((width, i) => (
              <div key={i} style={{ height: 10, width: `${width}%`, background: 'var(--border)', marginBottom: 6, opacity: 0.6 }} />
            ))}
          </div>
        )}
        {notesLoaded && notes.length === 0 && (
          <div style={{ color: 'var(--muted)', fontSize: '12px', marginBottom: 8 }}>no notes yet</div>
        )}
        {notes.map(note => (
          <div key={note.id} style={{ marginBottom: 4, fontSize: '12px' }}>
            <span style={{ color: 'var(--accent)' }}>{note.username}</span>
            <span style={{ color: 'var(--muted)', margin: '0 6px' }}>{formatTimestamp(note.created_at)}</span>
            <span style={{ color: 'var(--text)' }}>- {note.content}</span>
          </div>
        ))}
        <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
          <input
            type="text"
            value={noteInput}
            onChange={e => setNoteInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); void handleAddNote() } }}
            onClick={e => e.stopPropagation()}
            placeholder="add a note..."
            style={{
              flex: 1,
              background: 'var(--bg)',
              border: '1px solid var(--border)',
              color: 'var(--text)',
              padding: '4px 8px',
              fontFamily: 'inherit',
              fontSize: '12px',
              outline: 'none',
            }}
          />
          <button
            onClick={(e: React.MouseEvent<HTMLButtonElement>) => { e.stopPropagation(); void handleAddNote() }}
            disabled={noteLoading || !noteInput.trim()}
            style={{
              background: noteInput.trim() ? 'var(--accent)' : 'var(--border)',
              color: '#000',
              border: 'none',
              padding: '4px 10px',
              fontFamily: 'inherit',
              fontSize: '12px',
              fontWeight: 'bold',
              letterSpacing: '0.05em',
              cursor: noteInput.trim() ? 'pointer' : 'not-allowed',
            }}
          >
            ADD NOTE
          </button>
        </div>
      </div>
    </div>
  )
}

function DiffModal({ entry, diff, open, onClose }: { entry: Entry; diff: FileDiffMetadata; open: boolean; onClose: () => void }) {
  if (!open) return null
  const rows = buildDiffRows(diff.diff)

  return (
    <div
      className="diff-modal"
      onClick={e => { e.stopPropagation(); onClose() }}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0, 0, 0, 0.72)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
      }}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 'min(1200px, 100%)',
          height: 'min(760px, 100%)',
          background: 'var(--bg)',
          border: '1px solid var(--border)',
          display: 'grid',
          gridTemplateRows: 'auto auto 1fr',
          overflow: 'hidden',
          boxShadow: '0 18px 80px rgba(0, 0, 0, 0.45)',
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, padding: '14px 16px', borderBottom: '1px solid var(--border)' }}>
          <div>
            <div style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.12em' }}>DIFF WINDOW</div>
            <div style={{ color: 'var(--text)', fontSize: '13px', marginTop: 4 }}>{entry.content}</div>
          </div>
          <button
            type="button"
            onClick={onClose}
            style={{
              background: 'none',
              border: '1px solid var(--border)',
              color: 'var(--muted)',
              padding: '6px 10px',
              fontFamily: 'inherit',
              fontSize: '11px',
              letterSpacing: '0.08em',
              cursor: 'pointer',
            }}
          >
            CLOSE
          </button>
        </div>

        <div style={{ padding: '10px 16px', borderBottom: '1px solid var(--border)', display: 'flex', gap: 16, flexWrap: 'wrap', color: 'var(--muted)', fontSize: '11px' }}>
          <span>{diff.path}</span>
          <span>op: {diff.op}</span>
          <span>{diff.diffRedacted ? 'redaction: on' : 'redaction: off'}</span>
        </div>

        <div style={{ overflow: 'auto', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace', fontSize: '12px' }}>
          <div className="diff-grid" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', borderBottom: '1px solid var(--border)', position: 'sticky', top: 0, background: 'var(--surface)', zIndex: 1 }}>
            <div className="diff-cell-before" style={{ padding: '8px 12px', borderRight: '1px solid var(--border)', color: 'var(--danger)' }}>BEFORE</div>
            <div style={{ padding: '8px 12px', color: 'var(--success)' }}>AFTER</div>
          </div>

          {rows.map((row, index) => {
            if (row.kind === 'separator') {
              return (
                <div key={`${row.text}-${index}`} style={{ padding: '6px 12px', borderBottom: '1px solid var(--border)', color: 'var(--muted)', background: 'rgba(255,255,255,0.03)' }}>
                  {row.text}
                </div>
              )
            }

            const leftBackground = row.leftType === 'remove'
              ? 'rgba(255, 80, 80, 0.16)'
              : row.leftType === 'context'
                ? 'transparent'
                : 'rgba(255,255,255,0.02)'
            const rightBackground = row.rightType === 'add'
              ? 'rgba(80, 220, 120, 0.16)'
              : row.rightType === 'context'
                ? 'transparent'
                : 'rgba(255,255,255,0.02)'

            return (
              <div className="diff-grid" key={`row-${index}`} style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', borderBottom: '1px solid rgba(255,255,255,0.05)' }}>
                <pre className="diff-cell-before" style={{ margin: 0, padding: '6px 12px', whiteSpace: 'pre-wrap', wordBreak: 'break-word', borderRight: '1px solid var(--border)', background: leftBackground, color: row.leftType === 'remove' ? '#ff9b9b' : 'var(--text)', minHeight: 32 }}>
                  {row.left}
                </pre>
                <pre style={{ margin: 0, padding: '6px 12px', whiteSpace: 'pre-wrap', wordBreak: 'break-word', background: rightBackground, color: row.rightType === 'add' ? '#9cffb4' : 'var(--text)', minHeight: 32 }}>
                  {row.right}
                </pre>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function TimelineCard({ entry, isExpanded, isDimmed, isGhost, onClick, onTooltip, onTooltipClear }: EntryProps) {
  const possibleCause = entry.correlated_id ? parsePossibleCause(entry.metadata) : null
  const composeService = extractComposeService(entry)
  const sourceLabel = webhookProviderLabel(entry) ?? entry.source
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (isInteractiveEntryTarget(e.target)) return
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    onClick()
  }

  return (
    <motion.div
      data-row
      role="button"
      aria-expanded={isExpanded}
      tabIndex={isDimmed ? -1 : 0}
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: isDimmed ? 0.2 : 1, y: 0 }}
      exit={{ opacity: 0, y: -6 }}
      transition={{ duration: 0.16, ease: 'easeOut' }}
      onClick={onClick}
      onKeyDown={handleKeyDown}
      onMouseEnter={
        possibleCause
          ? (e: React.MouseEvent<HTMLDivElement>) => {
              const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
              onTooltip({ text: `possible cause: ${possibleCause}`, x: rect.left, y: rect.top })
            }
          : undefined
      }
      onMouseLeave={possibleCause ? onTooltipClear : undefined}
      style={{
        background: '#0F0F0F',
        border: '1px solid #1E1E1E',
        borderLeft: `3px solid ${eventBorderColor(entry.event)}`,
        cursor: 'pointer',
        filter: isDimmed ? 'blur(2px)' : 'none',
        pointerEvents: isDimmed ? 'none' : 'auto',
        outline: isGhost ? '1px dashed var(--accent)' : 'none',
        transition: 'filter 0.2s ease',
        userSelect: 'none',
        overflow: 'hidden',
      }}
    >
      {/* Header row */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '14px 16px 10px',
          flexWrap: 'wrap',
        }}
      >
        <span
          style={{
            fontSize: 14,
            fontWeight: 600,
            letterSpacing: '0.04em',
            color: eventTextColor(entry.event),
          }}
        >
          {entry.event}
        </span>
        {entry.service && (
          <span style={{ fontSize: 13, color: '#D0D0D0' }}>{entry.service}</span>
        )}
        {composeService && (
          <span style={{ fontSize: 10, color: '#71717A', fontFamily: 'monospace' }}>· {composeService}</span>
        )}
        {isGhost && (
          <span
            style={{
              fontSize: 10,
              color: 'var(--accent)',
              border: '1px solid var(--accent)',
              padding: '2px 6px',
              letterSpacing: '0.08em',
            }}
          >
            LINKED
          </span>
        )}
        {entry.correlated_id && !isGhost && (
          <span style={{ fontSize: 10, color: 'var(--accent)' }}>^</span>
        )}
        <span
          style={{
            marginLeft: 'auto',
            fontSize: 12,
            color: '#AAB4BD',
            letterSpacing: '0.04em',
            whiteSpace: 'nowrap',
          }}
        >
          {formatTimestamp(entry.timestamp)}
        </span>
      </div>

      {/* Meta row */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 14,
          padding: '0 16px 14px',
          flexWrap: 'wrap',
        }}
      >
        {entry.node_name && (
          <span style={{ fontSize: 11 }}>
            <span style={{ color: '#8B949E', letterSpacing: '0.1em', fontSize: 10 }}>NODE </span>
            <span style={{ color: '#C3CDD6' }}>{entry.node_name}</span>
          </span>
        )}
        <span style={{ fontSize: 11 }}>
          <span style={{ color: '#8B949E', letterSpacing: '0.1em', fontSize: 10 }}>SOURCE </span>
          <span style={{ color: '#C3CDD6' }}>{sourceLabel}</span>
        </span>
        {entry.content && (
          <span
            style={{
              fontSize: 12,
              color: '#888',
              fontStyle: 'italic',
              marginLeft: isExpanded ? 0 : 'auto',
              overflow: isExpanded ? 'visible' : 'hidden',
              textOverflow: isExpanded ? 'clip' : 'ellipsis',
              whiteSpace: isExpanded ? 'normal' : 'nowrap',
              maxWidth: isExpanded ? '100%' : '50%',
              flexBasis: isExpanded ? '100%' : 'auto',
              overflowWrap: isExpanded ? 'anywhere' : 'normal',
              lineHeight: 1.5,
            }}
          >
            {entry.content}
          </span>
        )}
      </div>

      {/* Expanded details */}
      <ExpandableSection isOpen={isExpanded}>
        <div style={{ borderTop: '1px solid #181818', padding: '14px 16px 14px 20px' }}>
          <ExpandedDetails entry={entry} />
        </div>
      </ExpandableSection>
    </motion.div>
  )
}

function TimelineRow({ entry, isExpanded, isDimmed, isGhost, incident, onClick, onTooltip, onTooltipClear }: EntryProps) {
  const navigate = useNavigate()
  const possibleCause = entry.correlated_id ? parsePossibleCause(entry.metadata) : null
  const composeService = extractComposeService(entry)
  const sourceLabel = webhookProviderLabel(entry) ?? entry.source
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (isInteractiveEntryTarget(e.target)) return
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    onClick()
  }

  const rowClassName = ['timeline-row', isDimmed ? 'dimmed' : '', isGhost ? 'ghost-card' : '']
    .filter(Boolean).join(' ')

  return (
    <motion.div
      data-row
      className={rowClassName}
      role="button"
      tabIndex={isDimmed ? -1 : 0}
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: isDimmed ? 0.2 : isGhost ? 0.85 : 1, y: 0 }}
      exit={{ opacity: 0, y: -6 }}
      transition={{ duration: 0.16, ease: 'easeOut' }}
      style={{
        display: 'grid',
        gridTemplateColumns: ROW_GRID_TEMPLATE,
        gap: '0 8px',
        padding: '4px 24px',
        cursor: 'pointer',
        background: isExpanded ? 'var(--surface)' : 'transparent',
        fontSize: '13px',
        alignItems: 'start',
        userSelect: 'none',
        overflow: 'hidden',
      }}
      onClick={onClick}
      onKeyDown={handleKeyDown}
      onMouseEnter={
        possibleCause
          ? (e: React.MouseEvent<HTMLDivElement>) => {
              const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
              onTooltip({ text: `possible cause: ${possibleCause}`, x: rect.left, y: rect.top })
            }
          : undefined
      }
      onMouseLeave={possibleCause ? onTooltipClear : undefined}
    >
      <div className="tr-indicator" style={{ width: 20, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        {incident ? (
          <button
            type="button"
            title={`Incident: ${incident.id}`}
            aria-label={`Open incidents for incident ${incident.id}`}
            style={{
              width: 6,
              height: 6,
              borderRadius: '50%',
              background: incident.confidence === 'confirmed'
                ? 'var(--danger)'
                : 'var(--warning)',
              cursor: 'pointer',
              display: 'inline-block',
              border: 'none',
              padding: 0,
              appearance: 'none',
            }}
            onClick={event => {
              event.stopPropagation()
              navigate('/incidents')
            }}
          />
        ) : (
          <span style={{ color: 'var(--accent)', fontSize: '11px', lineHeight: '20px' }}>
            {entry.correlated_id ? '^' : ''}
          </span>
        )}
      </div>
      <span className="tr-timestamp" style={{ color: 'var(--muted)', fontSize: '12px', whiteSpace: 'nowrap' }}>
        {formatTimestamp(entry.timestamp)}
      </span>
      <span className="tr-node" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: 'var(--text)', minWidth: 0 }}>
        {entry.node_name}
      </span>
      <span className="tr-source" style={{ color: 'var(--muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0 }}>
        {sourceLabel}
      </span>
      <div className="tr-service" style={{ overflow: 'hidden', minWidth: 0 }}>
        {entry.service && (
          <span
            style={{
              display: 'inline-block',
              maxWidth: '100%',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              padding: '2px 6px',
              border: '1px solid var(--border)',
              color: 'var(--accent)',
              fontSize: '10px',
              letterSpacing: '0.05em',
              background: 'var(--bg)',
            }}
          >
            {entry.service}
          </span>
        )}
        {composeService && (
          <span style={{ display: 'block', fontSize: '10px', color: '#71717A', fontFamily: 'monospace', marginTop: '2px' }}>{composeService}</span>
        )}
      </div>
      <span className="tr-event" style={{
        color: eventTextColor(entry.event),
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        minWidth: 0,
      }}>
        {entry.event}
      </span>

      <div className="tr-content" style={{ minWidth: 0, width: '100%' }}>
        <span style={{
          color: 'var(--text)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: isExpanded ? 'normal' : 'nowrap',
          display: 'block',
        }}>
          {entry.content}
          {isGhost && (
            <span style={{ marginLeft: 8, color: 'var(--accent)', fontSize: '10px', letterSpacing: '0.05em' }}>[LINKED]</span>
          )}
        </span>

        <ExpandableSection isOpen={isExpanded}>
          <ExpandedDetails entry={entry} />
        </ExpandableSection>
      </div>
    </motion.div>
  )
}
