import { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { createNote, fetchEntries, fetchEntry, fetchNotes } from '../api/client'
import type { Entry, EntryNote } from '../api/client'
import { useNodePulse } from '../components/NodePulse'

const SOURCE_OPTIONS = ['', 'docker', 'files', 'agent', 'webhook']

function formatTimestamp(ts: string) {
  const d = new Date(ts)
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

interface TooltipState {
  text: string
  x: number
  y: number
}

export default function TimelinePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const { nodes } = useNodePulse()

  const nodeFilter = searchParams.get('node') ?? ''
  const sourceFilter = searchParams.get('source') ?? ''
  const qFilter = searchParams.get('q') ?? ''

  const [entries, setEntries] = useState<Entry[]>([])
  const [nextCursor, setNextCursor] = useState<string | undefined>()
  const [loading, setLoading] = useState(false)
  const [done, setDone] = useState(false)
  const [renderedIds, setRenderedIds] = useState<Set<string>>(new Set())

  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [ghostEntry, setGhostEntry] = useState<Entry | null>(null)
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)

  const sentinelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    setEntries([])
    setNextCursor(undefined)
    setDone(false)
    setRenderedIds(new Set())
    setExpandedId(null)
    setGhostEntry(null)
  }, [nodeFilter, sourceFilter, qFilter])

  const loadMore = useCallback(async () => {
    if (loading || done) return
    setLoading(true)
    try {
      const page = await fetchEntries({
        cursor: nextCursor,
        limit: 50,
        node: nodeFilter || undefined,
        source: sourceFilter || undefined,
        q: qFilter || undefined,
      })
      setEntries(prev => {
        const nextIds = new Set(renderedIds)
        const newEntries = page.entries.filter(entry => !nextIds.has(entry.id))
        newEntries.forEach(entry => nextIds.add(entry.id))
        setRenderedIds(nextIds)
        return [...prev, ...newEntries]
      })
      if (!page.next_cursor) {
        setDone(true)
      } else {
        setNextCursor(page.next_cursor)
      }
    } catch (err) {
      console.error('loadMore:', err)
    } finally {
      setLoading(false)
    }
  }, [done, loading, nextCursor, nodeFilter, qFilter, renderedIds, sourceFilter])

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel) return

    const observer = new IntersectionObserver(
      observerEntries => {
        if (observerEntries[0]?.isIntersecting) loadMore()
      },
      { rootMargin: '200px' },
    )

    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [loadMore])

  useEffect(() => {
    void loadMore()
  }, [loadMore, nodeFilter, qFilter, sourceFilter])

  function handleRowClick(entry: Entry) {
    if (expandedId === entry.id) {
      setExpandedId(null)
      setGhostEntry(null)
      return
    }

    setExpandedId(entry.id)
    setGhostEntry(null)

    if (entry.correlated_id) {
      const alreadyInDom = entries.find(item => item.id === entry.correlated_id)
      if (!alreadyInDom) {
        fetchEntry(entry.correlated_id)
          .then(ghost => {
            if (!renderedIds.has(ghost.id)) {
              setGhostEntry(ghost)
              setRenderedIds(prev => new Set([...prev, ghost.id]))
            }
          })
          .catch(() => {})
      }
    }
  }

  function handleOverlayClick() {
    if (expandedId) {
      if (ghostEntry) {
        setRenderedIds(prev => {
          const next = new Set(prev)
          next.delete(ghostEntry.id)
          return next
        })
      }
      setExpandedId(null)
      setGhostEntry(null)
    }
  }

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') handleOverlayClick()
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  })

  function setFilter(key: string, value: string) {
    const next = new URLSearchParams(searchParams)
    if (value) next.set(key, value)
    else next.delete(key)
    setSearchParams(next)
  }

  const displayEntries = (() => {
    if (!expandedId || !ghostEntry) return entries
    const expandedIndex = entries.findIndex(entry => entry.id === expandedId)
    if (expandedIndex === -1) return entries
    const result = [...entries]
    result.splice(expandedIndex, 0, ghostEntry)
    return result
  })()

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div
        style={{
          display: 'flex',
          gap: 12,
          padding: '8px 16px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--surface)',
          alignItems: 'center',
          flexShrink: 0,
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
            <option key={node.id} value={node.name}>
              {node.name}
            </option>
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
              setFilter('node', '')
              setFilter('source', '')
              setFilter('q', '')
            }}
            style={{ color: 'var(--muted)', fontSize: '11px', cursor: 'pointer', letterSpacing: '0.05em' }}
          >
            CLEAR
          </span>
        )}
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '20px 130px 100px 70px 100px 70px 1fr',
          gap: '0 8px',
          padding: '4px 16px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--surface)',
          color: 'var(--muted)',
          fontSize: '10px',
          letterSpacing: '0.1em',
          flexShrink: 0,
        }}
      >
        <span />
        <span>TIMESTAMP</span>
        <span>NODE</span>
        <span>SOURCE</span>
        <span>SERVICE</span>
        <span>EVENT</span>
        <span>CONTENT</span>
      </div>

      <div
        style={{ flex: 1, overflowY: 'auto', position: 'relative' }}
        onClick={e => {
          if ((e.target as HTMLElement).closest('[data-row]')) return
          handleOverlayClick()
        }}
      >
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

        <div ref={sentinelRef} style={{ height: 1 }} />

        {loading && (
          <div style={{ padding: '8px 16px', color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
        )}
        {done && !loading && entries.length > 0 && (
          <div style={{ padding: '8px 16px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
            - end of timeline -
          </div>
        )}
        {done && entries.length === 0 && (
          <div style={{ padding: '24px 16px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
            no entries found
          </div>
        )}
      </div>

      {tooltip && (
        <div className="tooltip-portal" style={{ top: tooltip.y - 8, left: tooltip.x + 12 }}>
          {tooltip.text}
        </div>
      )}
    </div>
  )
}

interface RowProps {
  entry: Entry
  isExpanded: boolean
  isDimmed: boolean
  isGhost: boolean
  onClick: () => void
  onTooltip: (tooltip: TooltipState) => void
  onTooltipClear: () => void
}

function TimelineRow({
  entry,
  isExpanded,
  isDimmed,
  isGhost,
  onClick,
  onTooltip,
  onTooltipClear,
}: RowProps) {
  const possibleCause = entry.correlated_id ? parsePossibleCause(entry.metadata) : null
  const [notes, setNotes] = useState<EntryNote[]>([])
  const [notesLoaded, setNotesLoaded] = useState(false)
  const [noteInput, setNoteInput] = useState('')
  const [noteLoading, setNoteLoading] = useState(false)

  useEffect(() => {
    if (isExpanded && !notesLoaded) {
      fetchNotes(entry.id)
        .then(data => {
          setNotes(data)
          setNotesLoaded(true)
        })
        .catch(() => setNotesLoaded(true))
    }
  }, [entry.id, isExpanded, notesLoaded])

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

  const rowClassName = ['timeline-row', isDimmed ? 'dimmed' : '', isGhost ? 'ghost-card' : '']
    .filter(Boolean)
    .join(' ')

  const rowStyle: React.CSSProperties = {
    display: 'grid',
    gridTemplateColumns: '20px 130px 100px 70px 100px 70px 1fr',
    gap: '0 8px',
    padding: '4px 16px',
    borderBottom: '1px solid var(--border)',
    cursor: 'pointer',
    background: isExpanded ? 'var(--surface)' : 'transparent',
    fontSize: '12px',
    alignItems: 'start',
    userSelect: 'none',
  }

  return (
    <motion.div
      layout
      data-row
      className={rowClassName}
      style={rowStyle}
      onClick={onClick}
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
        {entry.correlated_id ? '↗' : ''}
      </span>

      <span style={{ color: 'var(--muted)', fontSize: '11px', whiteSpace: 'nowrap' }}>
        {formatTimestamp(entry.timestamp)}
      </span>

      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: 'var(--text)' }}>
        {entry.node_name}
      </span>

      <span style={{ color: 'var(--muted)' }}>{entry.source}</span>

      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {entry.service}
      </span>

      <span
        style={{
          color:
            entry.event === 'die' || entry.event === 'down'
              ? '#FF4444'
              : entry.event === 'start' || entry.event === 'up'
                ? 'var(--accent)'
                : 'var(--text)',
        }}
      >
        {entry.event}
      </span>

      <div>
        <span
          style={{
            color: 'var(--text)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: isExpanded ? 'normal' : 'nowrap',
            display: 'block',
          }}
        >
          {entry.content}
          {isGhost && (
            <span style={{ marginLeft: 8, color: 'var(--accent)', fontSize: '10px', letterSpacing: '0.05em' }}>
              [LINKED]
            </span>
          )}
        </span>

        {isExpanded && (
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} style={{ marginTop: 8, paddingTop: 8, borderTop: '1px solid var(--border)' }}>
            {entry.metadata && entry.metadata !== '{}' && (
              <div style={{ marginBottom: 12 }}>
                <div style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em', marginBottom: 4 }}>
                  METADATA
                </div>
                <pre style={{ color: 'var(--text)', fontSize: '11px', margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                  {formatMetadata(entry.metadata)}
                </pre>
              </div>
            )}

            <div>
              <div style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em', marginBottom: 6 }}>
                NOTES
              </div>
              {!notesLoaded && (
                <div style={{ marginBottom: 8 }}>
                  {[80, 60, 72].map((width, index) => (
                    <div
                      key={index}
                      style={{
                        height: 10,
                        width: `${width}%`,
                        background: 'var(--border)',
                        marginBottom: 6,
                        opacity: 0.6,
                      }}
                    />
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
                  onKeyDown={e => {
                    if (e.key === 'Enter') {
                      e.preventDefault()
                      void handleAddNote()
                    }
                  }}
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
                  onClick={(e: React.MouseEvent<HTMLButtonElement>) => {
                    e.stopPropagation()
                    void handleAddNote()
                  }}
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
        )}
      </div>
    </motion.div>
  )
}
