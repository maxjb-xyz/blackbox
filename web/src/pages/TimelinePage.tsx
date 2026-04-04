import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { createNote, fetchEntries, fetchEntry, fetchNotes } from '../api/client'
import type { Entry, EntryNote } from '../api/client'
import { useNodePulse } from '../components/NodePulse'
import { useWebSocketContext } from '../components/WebSocketProvider'

const SOURCE_OPTIONS = ['', 'docker', 'files', 'agent', 'webhook']

const SOURCE_TINT: Record<string, string> = {
  docker: 'rgba(26, 58, 92, 0.35)',
  files: 'rgba(58, 46, 10, 0.35)',
  webhook: 'rgba(42, 26, 58, 0.35)',
  agent: 'rgba(10, 46, 46, 0.35)',
}

function eventBorderColor(event: string): string {
  if (event === 'die' || event === 'down') return '#FF4444'
  if (event === 'start' || event === 'up') return 'var(--success)'
  if (event === 'update') return '#FF9900'
  return 'var(--border)'
}

function eventTextColor(event: string): string {
  if (event === 'die' || event === 'down') return '#FF4444'
  if (event === 'start' || event === 'up') return 'var(--success)'
  return 'var(--text)'
}

function formatTimestamp(ts?: string | null) {
  if (!ts) return ''
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return ''
  return d.toISOString().replace('T', ' ').substring(0, 16)
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

function entryTimestampMs(entry: Entry): number {
  const ts = Date.parse(entry.timestamp)
  return Number.isNaN(ts) ? 0 : ts
}

function compareEntries(a: Entry, b: Entry): number {
  const tsDiff = entryTimestampMs(b) - entryTimestampMs(a)
  if (tsDiff !== 0) return tsDiff
  if (a.id === b.id) return 0
  return a.id < b.id ? 1 : -1
}

function mergeEntries(existing: Entry[], incoming: Entry[]): Entry[] {
  const merged = new Map<string, Entry>()
  for (const entry of existing) merged.set(entry.id, entry)
  for (const entry of incoming) merged.set(entry.id, entry)
  return Array.from(merged.values()).sort(compareEntries)
}

function matchesEntryFilters(entry: Entry, nodeFilter: string, sourceFilter: string, qFilter: string, hideHeartbeat: boolean): boolean {
  if (hideHeartbeat && entry.source === 'agent' && entry.event === 'heartbeat') return false
  if (nodeFilter && entry.node_name !== nodeFilter) return false
  if (sourceFilter && entry.source !== sourceFilter) return false
  if (qFilter) {
    const q = qFilter.toLowerCase()
    const haystacks = [entry.content, entry.service].filter(Boolean).map(value => value.toLowerCase())
    if (!haystacks.some(value => value.includes(q))) return false
  }
  return true
}

export default function TimelinePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const { nodes } = useNodePulse()
  const [viewMode, setViewMode] = useState<ViewMode>(getStoredViewMode)
  const [hideHeartbeat, setHideHeartbeat] = useState<boolean>(getStoredHideHeartbeat)

  const nodeFilter = searchParams.get('node') ?? ''
  const sourceFilter = searchParams.get('source') ?? ''
  const qFilter = searchParams.get('q') ?? ''

  function setFilter(key: string, value: string) {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      if (value) next.set(key, value)
      else next.delete(key)
      return next
    })
  }

  function toggleViewMode() {
    setViewMode(prev => {
      const next: ViewMode = prev === 'cards' ? 'rows' : 'cards'
      localStorage.setItem('timeline_view', next)
      return next
    })
  }

  function toggleHideHeartbeat() {
    setHideHeartbeat(prev => {
      const next = !prev
      localStorage.setItem('timeline_hide_heartbeats', String(next))
      return next
    })
  }

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div
        style={{
          display: 'flex',
          gap: 12,
          padding: '10px 24px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--surface)',
          alignItems: 'center',
          flexShrink: 0,
          flexWrap: 'wrap',
        }}
      >
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>FILTER:</span>

        <select
          value={nodeFilter}
          onChange={e => setFilter('node', e.target.value)}
          style={{
            background: 'var(--bg)',
            color: nodeFilter ? 'var(--text)' : 'var(--muted)',
            border: '1px solid var(--border)',
            fontSize: '11px',
            padding: '2px 6px',
            fontFamily: 'inherit',
          }}
        >
          <option value="">ALL NODES</option>
          {nodes.map(node => (
            <option key={node.id} value={node.name}>{node.name}</option>
          ))}
        </select>

        <select
          value={sourceFilter}
          onChange={e => setFilter('source', e.target.value)}
          style={{
            background: 'var(--bg)',
            color: sourceFilter ? 'var(--text)' : 'var(--muted)',
            border: '1px solid var(--border)',
            fontSize: '11px',
            padding: '2px 6px',
            fontFamily: 'inherit',
          }}
        >
          {SOURCE_OPTIONS.map(source => (
            <option key={source} value={source}>
              {source ? source.toUpperCase() : 'ALL SOURCES'}
            </option>
          ))}
        </select>

        <input
          type="text"
          placeholder="SEARCH..."
          value={qFilter}
          onChange={e => setFilter('q', e.target.value)}
          style={{
            background: 'var(--bg)',
            color: 'var(--text)',
            border: '1px solid var(--border)',
            fontSize: '11px',
            padding: '2px 8px',
            fontFamily: 'inherit',
            width: 200,
          }}
        />

        {(nodeFilter || sourceFilter || qFilter) && (
          <span
            onClick={() => {
              setSearchParams(prev => {
                const next = new URLSearchParams(prev)
                next.delete('node')
                next.delete('source')
                next.delete('q')
                return next
              })
            }}
            style={{ color: 'var(--muted)', fontSize: '11px', cursor: 'pointer', letterSpacing: '0.05em' }}
          >
            CLEAR
          </span>
        )}

        <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'center' }}>
          <button
            onClick={toggleHideHeartbeat}
            style={{
              background: 'none',
              border: '1px solid var(--border)',
              color: hideHeartbeat ? 'var(--muted)' : 'var(--accent)',
              fontSize: '10px',
              padding: '2px 8px',
              fontFamily: 'inherit',
              cursor: 'pointer',
              letterSpacing: '0.08em',
            }}
          >
            {hideHeartbeat ? 'HEARTBEATS HIDDEN' : 'SHOW HEARTBEATS'}
          </button>

          <button
            onClick={toggleViewMode}
            style={{
              background: 'none',
              border: '1px solid var(--border)',
              color: 'var(--muted)',
              fontSize: '10px',
              padding: '2px 8px',
              fontFamily: 'inherit',
              cursor: 'pointer',
              letterSpacing: '0.08em',
            }}
          >
            {viewMode === 'cards' ? 'ROWS' : 'CARDS'}
          </button>
        </div>
      </div>

      <TimelineFeed
        key={`${nodeFilter}:${sourceFilter}:${qFilter}:${hideHeartbeat}`}
        nodeFilter={nodeFilter}
        sourceFilter={sourceFilter}
        qFilter={qFilter}
        hideHeartbeat={hideHeartbeat}
        viewMode={viewMode}
      />
    </div>
  )
}

interface TimelineFeedProps {
  nodeFilter: string
  sourceFilter: string
  qFilter: string
  hideHeartbeat: boolean
  viewMode: ViewMode
}

function TimelineFeed({ nodeFilter, sourceFilter, qFilter, hideHeartbeat, viewMode }: TimelineFeedProps) {
  const { lastMessage } = useWebSocketContext()
  const [entries, setEntries] = useState<Entry[]>([])
  const [nextCursor, setNextCursor] = useState<string | undefined>()
  const [loading, setLoading] = useState(true)
  const [done, setDone] = useState(false)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [ghostEntry, setGhostEntry] = useState<Entry | null>(null)
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)
  const [sentinelVisible, setSentinelVisible] = useState(false)

  const sentinelRef = useRef<HTMLDivElement>(null)
  const renderedIdsRef = useRef<Set<string>>(new Set())
  const expandedIdRef = useRef<string | null>(null)
  const ghostEntryRef = useRef<Entry | null>(null)
  const mountedRef = useRef(true)

  const loadPage = useEffectEvent(async (cursor?: string) => {
    setLoading(true)
    try {
      const page = await fetchEntries({
        cursor,
        limit: 50,
        node: nodeFilter || undefined,
        source: sourceFilter || undefined,
        q: qFilter || undefined,
        hideHeartbeat,
      })
      if (!mountedRef.current) return

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
    } catch (err) {
      if (mountedRef.current) {
        console.error(cursor ? 'loadMore:' : 'loadEntries:', err)
      }
    } finally {
      if (mountedRef.current) setLoading(false)
    }
  })

  useEffect(() => {
    mountedRef.current = true
    void loadPage()
    return () => { mountedRef.current = false }
  }, [])

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
    if (loading || done || !nextCursor || !sentinelVisible) return
    void loadPage(nextCursor)
  }, [done, loading, nextCursor, nodeFilter, qFilter, sentinelVisible, sourceFilter])

  useEffect(() => {
    if (!lastMessage || lastMessage.type !== 'entry') return
    const newEntry = lastMessage.data as Entry
    if (renderedIdsRef.current.has(newEntry.id)) return
    if (!matchesEntryFilters(newEntry, nodeFilter, sourceFilter, qFilter, hideHeartbeat)) return
    setEntries(prev => {
      const mergedEntries = mergeEntries(prev, [newEntry])
      renderedIdsRef.current = new Set(mergedEntries.map(entry => entry.id))
      return mergedEntries
    })
  }, [hideHeartbeat, lastMessage, nodeFilter, qFilter, sourceFilter])

  function handleRowClick(entry: Entry) {
    if (expandedId === entry.id) {
      if (ghostEntryRef.current) {
        renderedIdsRef.current.delete(ghostEntryRef.current.id)
        ghostEntryRef.current = null
      }
      expandedIdRef.current = null
      setExpandedId(null)
      setGhostEntry(null)
      return
    }
    const requestedEntryId = entry.id
    if (ghostEntryRef.current) {
      renderedIdsRef.current.delete(ghostEntryRef.current.id)
      ghostEntryRef.current = null
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
    if (ghostEntryRef.current) {
      renderedIdsRef.current.delete(ghostEntryRef.current.id)
      ghostEntryRef.current = null
    }
    expandedIdRef.current = null
    setExpandedId(null)
    setGhostEntry(null)
  }, [])

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      if (!expandedIdRef.current) return
      if (ghostEntryRef.current) {
        renderedIdsRef.current.delete(ghostEntryRef.current.id)
        ghostEntryRef.current = null
      }
      expandedIdRef.current = null
      setExpandedId(null)
      setGhostEntry(null)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const displayEntries = (() => {
    if (!expandedId || !ghostEntry) return entries
    const expandedIndex = entries.findIndex(entry => entry.id === expandedId)
    if (expandedIndex === -1) return entries
    const result = [...entries]
    result.splice(expandedIndex, 0, ghostEntry)
    return result
  })()

  return (
    <>
      {viewMode === 'rows' && (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: '20px 130px 100px 70px 100px 90px 1fr',
            gap: '0 8px',
            padding: '4px 24px',
            borderBottom: '1px solid var(--border)',
            background: 'var(--surface)',
            color: 'var(--muted)',
            fontSize: '10px',
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
            <AnimatePresence>
              {displayEntries.map(entry => (
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
          <AnimatePresence>
            {displayEntries.map(entry => (
              <TimelineRow
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
        )}

        <div ref={sentinelRef} style={{ height: 1 }} />

        {loading && (
          <div style={{ padding: '8px 24px', color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
        )}
        {done && !loading && entries.length > 0 && (
          <div style={{ padding: '8px 24px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
            - end of timeline -
          </div>
        )}
        {done && entries.length === 0 && (
          <div style={{ padding: '24px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
            no entries found
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
  onClick: () => void
  onTooltip: (tooltip: TooltipState) => void
  onTooltipClear: () => void
}

function ExpandedDetails({ entry }: { entry: Entry }) {
  const [notes, setNotes] = useState<EntryNote[]>([])
  const [notesLoaded, setNotesLoaded] = useState(false)
  const [noteInput, setNoteInput] = useState('')
  const [noteLoading, setNoteLoading] = useState(false)
  const [metaExpanded, setMetaExpanded] = useState(false)

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

  const formattedMeta = entry.metadata && entry.metadata !== '{}' ? formatMetadata(entry.metadata) : null

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} style={{ marginTop: 10, paddingTop: 10, borderTop: '1px solid var(--border)' }}>
      {formattedMeta && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em', marginBottom: 4 }}>METADATA</div>
          <div style={{ position: 'relative' }}>
            <pre
              style={{
                color: 'var(--text)',
                fontSize: '11px',
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
                  fontSize: '10px',
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
        <div style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em', marginBottom: 6 }}>NOTES</div>
        {!notesLoaded && (
          <div style={{ marginBottom: 8 }}>
            {[80, 60, 72].map((width, i) => (
              <div key={i} style={{ height: 10, width: `${width}%`, background: 'var(--border)', marginBottom: 6, opacity: 0.6 }} />
            ))}
          </div>
        )}
        {notesLoaded && notes.length === 0 && (
          <div style={{ color: 'var(--muted)', fontSize: '11px', marginBottom: 8 }}>no notes yet</div>
        )}
        {notes.map(note => (
          <div key={note.id} style={{ marginBottom: 4, fontSize: '11px' }}>
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
              fontSize: '11px',
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
              fontSize: '11px',
              fontWeight: 'bold',
              letterSpacing: '0.05em',
              cursor: noteInput.trim() ? 'pointer' : 'not-allowed',
            }}
          >
            ADD NOTE
          </button>
        </div>
      </div>
    </motion.div>
  )
}

function TimelineCard({ entry, isExpanded, isDimmed, isGhost, onClick, onTooltip, onTooltipClear }: EntryProps) {
  const possibleCause = entry.correlated_id ? parsePossibleCause(entry.metadata) : null
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    onClick()
  }

  return (
    <motion.div
      layout
      data-row
      role="button"
      tabIndex={isDimmed ? -1 : 0}
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
        background: SOURCE_TINT[entry.source] ?? 'var(--surface)',
        borderLeft: `3px solid ${eventBorderColor(entry.event)}`,
        padding: '14px 18px',
        cursor: 'pointer',
        opacity: isDimmed ? 0.2 : 1,
        filter: isDimmed ? 'blur(2px)' : 'none',
        pointerEvents: isDimmed ? 'none' : 'auto',
        outline: isGhost ? '1px dashed var(--accent)' : 'none',
        transition: 'opacity 0.2s ease, filter 0.2s ease',
        userSelect: 'none',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px' }}>
          {formatTimestamp(entry.timestamp)}
          {entry.node_name && <span style={{ margin: '0 6px' }}>|</span>}
          {entry.node_name}
          {entry.source && <span style={{ margin: '0 6px' }}>|</span>}
          <span style={{ color: 'var(--muted)' }}>{entry.source}</span>
        </span>
        <span
          style={{
            color: eventTextColor(entry.event),
            fontSize: '10px',
            fontWeight: 'bold',
            letterSpacing: '0.1em',
            background: 'var(--bg)',
            padding: '2px 6px',
            border: `1px solid ${eventBorderColor(entry.event)}`,
          }}
        >
          {entry.event.toUpperCase()}
        </span>
      </div>

      {entry.service && (
        <div style={{ color: 'var(--text)', fontSize: '12px', fontWeight: 'bold', marginBottom: 2 }}>
          {entry.service}
        </div>
      )}

      <div style={{ color: 'var(--text)', fontSize: '13px', marginBottom: 0 }}>
        {entry.content}
        {isGhost && (
          <span style={{ marginLeft: 8, color: 'var(--accent)', fontSize: '10px', letterSpacing: '0.05em' }}>[LINKED]</span>
        )}
        {entry.correlated_id && (
          <span style={{ marginLeft: 8, color: 'var(--accent)', fontSize: '10px' }}>^</span>
        )}
      </div>

      {isExpanded && <ExpandedDetails entry={entry} />}
    </motion.div>
  )
}

function TimelineRow({ entry, isExpanded, isDimmed, isGhost, onClick, onTooltip, onTooltipClear }: EntryProps) {
  const possibleCause = entry.correlated_id ? parsePossibleCause(entry.metadata) : null
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    onClick()
  }

  const rowClassName = ['timeline-row', isDimmed ? 'dimmed' : '', isGhost ? 'ghost-card' : '']
    .filter(Boolean).join(' ')

  return (
    <motion.div
      layout
      data-row
      className={rowClassName}
      role="button"
      tabIndex={isDimmed ? -1 : 0}
      style={{
        display: 'grid',
        gridTemplateColumns: '20px 130px 100px 70px 100px 90px 1fr',
        gap: '0 8px',
        padding: '4px 24px',
        cursor: 'pointer',
        background: isExpanded ? 'var(--surface)' : 'transparent',
        fontSize: '13px',
        alignItems: 'start',
        userSelect: 'none',
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
      <span style={{ color: 'var(--accent)', fontSize: '11px', lineHeight: '20px' }}>
        {entry.correlated_id ? '^' : ''}
      </span>
      <span style={{ color: 'var(--muted)', fontSize: '11px', whiteSpace: 'nowrap' }}>
        {formatTimestamp(entry.timestamp)}
      </span>
      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: 'var(--text)' }}>
        {entry.node_name}
      </span>
      <span style={{ color: 'var(--muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {entry.source}
      </span>
      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {entry.service}
      </span>
      <span style={{
        color: eventTextColor(entry.event),
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
      }}>
        {entry.event}
      </span>

      <div>
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

        {isExpanded && <ExpandedDetails entry={entry} />}
      </div>
    </motion.div>
  )
}
