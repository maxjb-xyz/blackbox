import { useCallback, useEffect, useRef, useState } from 'react'
import { AlertTriangle, CheckCircle, ChevronDown, ChevronRight } from 'lucide-react'
import {
  fetchIncident,
  fetchIncidents,
  type Incident,
  type IncidentDetail,
  parseIncidentMetadata,
  parseIncidentNodes,
  parseIncidentServices,
} from '../api/client'
import { useWebSocketContext } from '../components/WebSocketProvider'

function incidentBorderColor(inc: Incident): string {
  if (inc.status === 'resolved') return 'var(--success)'
  if (inc.confidence === 'confirmed') return 'var(--danger)'
  return 'var(--warning)'
}

function confidenceBadge(inc: Incident) {
  const color = inc.confidence === 'confirmed' ? 'var(--danger)' : 'var(--warning)'
  return (
    <span style={{ color, fontSize: 11, letterSpacing: '0.1em', marginRight: 8 }}>
      [{inc.confidence.toUpperCase()}]
    </span>
  )
}

function statusLabel(inc: Incident) {
  if (inc.status === 'resolved') {
    return <span style={{ color: 'var(--success)', fontSize: 11 }}>RESOLVED</span>
  }
  return <span style={{ color: 'var(--danger)', fontSize: 11 }}>OPEN</span>
}

function formatTs(ts?: string | null) {
  if (!ts) return '—'
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toISOString().replace('T', ' ').substring(0, 16)
}

function duration(opened: string, resolved?: string | null) {
  const start = new Date(opened).getTime()
  const end = resolved ? new Date(resolved).getTime() : Date.now()
  const secs = Math.floor((end - start) / 1000)
  if (secs < 60) return `${secs}s`
  if (secs < 3600) return `${Math.floor(secs / 60)}m`
  return `${Math.floor(secs / 3600)}h ${Math.floor((secs % 3600) / 60)}m`
}

function roleColor(role: string): string {
  if (role === 'trigger') return 'var(--danger)'
  if (role === 'cause') return 'var(--warning)'
  if (role === 'recovery') return 'var(--success)'
  return 'var(--muted)'
}

function isIncidentAIPending(meta: Record<string, unknown>): boolean {
  return meta.ai_pending === true
}

function incidentFingerprint(incident: Incident): string {
  return [
    incident.id,
    incident.opened_at,
    incident.resolved_at ?? '',
    incident.status,
    incident.confidence,
    incident.title,
    incident.services,
    incident.root_cause_id ?? '',
    incident.trigger_id ?? '',
    incident.node_names,
    incident.metadata,
  ].join('|')
}

function mergeAndDedupeIncidents(preferred: Incident[], fallback: Incident[]): Incident[] {
  const merged = new Map<string, Incident>()
  for (const incident of preferred) {
    merged.set(incident.id, incident)
  }
  for (const incident of fallback) {
    if (!merged.has(incident.id)) {
      merged.set(incident.id, incident)
    }
  }
  return [...merged.values()].sort((left, right) => {
    const tsDiff = new Date(right.opened_at).getTime() - new Date(left.opened_at).getTime()
    if (tsDiff !== 0) return tsDiff
    return right.id.localeCompare(left.id)
  })
}

interface IncidentCardProps {
  incident: Incident
  defaultOpen?: boolean
}

function ConfidenceBar({ score }: { score: number }) {
  const clampedScore = Math.max(0, Math.min(100, score))

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 6 }}>
      <div
        style={{
          flex: 1,
          height: 3,
          background: '#222',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            width: `${clampedScore}%`,
            height: '100%',
            background: '#a855f7',
          }}
        />
      </div>
      <span style={{ color: '#a855f7', fontSize: 10, lineHeight: 1 }}>
        {clampedScore}%
      </span>
    </div>
  )
}

function IncidentCard({ incident, defaultOpen = false }: IncidentCardProps) {
  const [expanded, setExpanded] = useState(defaultOpen)
  const [detail, setDetail] = useState<IncidentDetail | null>(null)
  const [loadingDetail, setLoadingDetail] = useState(false)

  useEffect(() => {
    if (!detail) return
    if (detail.incident.id !== incident.id) {
      setDetail(null)
      return
    }
    if (incidentFingerprint(detail.incident) === incidentFingerprint(incident)) return

    setDetail(prev => prev ? { ...prev, incident } : prev)
    if (!expanded) return

    let cancelled = false
    setLoadingDetail(true)
    void fetchIncident(incident.id)
      .then(nextDetail => {
        if (!cancelled) {
          setDetail(nextDetail)
        }
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) {
          setLoadingDetail(false)
        }
      })

    return () => {
      cancelled = true
    }
  }, [detail, expanded, incident])

  const detailIncident = detail?.incident ?? incident
  const services = parseIncidentServices(detailIncident)
  const nodes = parseIncidentNodes(detailIncident)
  const incidentMeta = parseIncidentMetadata(incident)
  const meta = parseIncidentMetadata(detailIncident)
  const aiPending = isIncidentAIPending(incidentMeta) || isIncidentAIPending(meta)

  const toggle = useCallback(async () => {
    if (!expanded && !detail) {
      setLoadingDetail(true)
      try {
        const d = await fetchIncident(incident.id)
        setDetail(d)
      } catch {
        // ignore — detail just won't show
      } finally {
        setLoadingDetail(false)
      }
    }
    setExpanded(v => !v)
  }, [expanded, detail, incident.id])

  const borderColor = incidentBorderColor(incident)
  const deterministicEntries = detail?.entries.filter(({ link }) => link.role !== 'ai_cause') ?? []
  const aiCauseEntries = detail?.entries.filter(({ link }) => link.role === 'ai_cause') ?? []

  return (
    <div
      style={{
        borderLeft: `2px solid ${borderColor}`,
        opacity: incident.status === 'resolved' ? 0.7 : 1,
        background: 'var(--surface)',
        marginBottom: 4,
      }}
    >
      <button
        type="button"
        onClick={() => void toggle()}
        style={{
          width: '100%',
          background: 'transparent',
          border: 'none',
          padding: '8px 12px',
          cursor: 'pointer',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          fontFamily: 'inherit',
          textAlign: 'left',
        }}
      >
        {expanded
          ? <ChevronDown size={12} color="var(--muted)" />
          : <ChevronRight size={12} color="var(--muted)" />}
        {confidenceBadge(incident)}
        <span style={{ fontSize: 12, color: 'var(--text)', flex: 1, minWidth: 0 }}>
          {services.join(', ')}
          {nodes.length > 0 && (
            <span style={{ color: 'var(--muted)', marginLeft: 6 }}>
              · {nodes.join(', ')}
            </span>
          )}
        </span>
        {statusLabel(incident)}
        {aiPending && (
          <span style={{ color: 'var(--accent)', fontSize: 11, marginLeft: 12, whiteSpace: 'nowrap', letterSpacing: '0.1em' }}>
            AI THINKING
          </span>
        )}
        <span style={{ fontSize: 11, color: 'var(--muted)', marginLeft: 12, whiteSpace: 'nowrap' }}>
          {formatTs(incident.opened_at)}
          {' → '}
          {incident.status === 'resolved'
            ? `${formatTs(incident.resolved_at)} (${duration(incident.opened_at, incident.resolved_at)})`
            : `ongoing (${duration(incident.opened_at)})`}
        </span>
      </button>

      <div style={{ padding: '0 12px 8px 36px', fontSize: 12, color: 'var(--text)' }}>
        {incident.title}
      </div>

      {expanded && (
        <div style={{ padding: '0 12px 12px 36px', fontSize: 11 }}>
          {loadingDetail && (
            <div style={{ color: 'var(--muted)' }}>loading...</div>
          )}

          {detail && (
            <>
              <div style={{ marginBottom: 8 }}>
                <div style={{ color: 'var(--muted)', marginBottom: 4, letterSpacing: '0.1em' }}>
                  EVENT CHAIN
                </div>
                <div style={{ borderTop: '1px solid var(--border)', paddingTop: 4 }}>
                  {deterministicEntries.map(({ link, entry }) => (
                    <div
                      key={link.entry_id}
                      style={{
                        display: 'grid',
                        gridTemplateColumns: '70px 130px 80px 80px 140px 1fr',
                        gap: 8,
                        padding: '2px 0',
                        alignItems: 'start',
                      }}
                    >
                      <span style={{ color: roleColor(link.role) }}>
                        {link.role.toUpperCase()}
                        {link.role === 'cause' && detailIncident.root_cause_id === link.entry_id && ' ★'}
                      </span>
                      <span style={{ color: 'var(--muted)' }}>{formatTs(entry.timestamp)}</span>
                      <span style={{ color: 'var(--muted)' }}>{entry.source}</span>
                      <span style={{ color: 'var(--muted)' }}>{entry.service}</span>
                      <span style={{ color: 'var(--text)' }}>{entry.event}</span>
                      <span style={{ color: 'var(--muted)', wordBreak: 'break-all' }}>
                        {link.score > 0 && `score ${link.score}`}
                      </span>
                    </div>
                  ))}
                </div>
              </div>

              {aiCauseEntries.length > 0 && (
                <div style={{ marginBottom: 8 }}>
                  <div
                    style={{
                      borderTop: '1px solid rgba(255, 255, 255, 0.08)',
                      paddingTop: 8,
                      marginBottom: 6,
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                    }}
                  >
                    <div style={{ color: 'var(--muted)', letterSpacing: '0.1em' }}>
                      AI CORRELATION
                    </div>
                    <span
                      style={{
                        border: '1px solid #a855f7',
                        color: '#a855f7',
                        fontSize: 10,
                        lineHeight: 1,
                        padding: '2px 5px',
                        borderRadius: 999,
                        letterSpacing: '0.08em',
                      }}
                    >
                      AI
                    </span>
                  </div>
                  <div style={{ display: 'grid', gap: 8 }}>
                    {aiCauseEntries.map(({ link, entry }) => (
                      <div
                        key={link.entry_id}
                        style={{
                          borderLeft: '2px solid #a855f7',
                          padding: '2px 0 2px 10px',
                        }}
                      >
                        <div style={{ color: 'var(--text)', marginBottom: 4 }}>
                          {entry.service || 'unknown service'}
                          {' / '}
                          {entry.source || 'unknown source'}
                          {' / '}
                          {entry.event || 'unknown event'}
                        </div>
                        <div style={{ color: 'var(--muted)', lineHeight: 1.5 }}>
                          {entry.content}
                        </div>
                        {link.reason && (
                          <div style={{ color: 'var(--muted)', fontStyle: 'italic', marginTop: 4 }}>
                            {link.reason}
                          </div>
                        )}
                        <ConfidenceBar score={link.score} />
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {(() => {
                const causeWithLog = detail.entries.reduce<{
                  entry: IncidentDetail['entries'][number]['entry']
                  logLines: string[]
                } | null>((match, { link, entry }) => {
                  if (match || link.role !== 'cause') return match
                  try {
                    const metadata = JSON.parse(entry.metadata) as Record<string, unknown>
                    const logLines = Array.isArray(metadata.log_snippet) ? metadata.log_snippet as string[] : []
                    if (logLines.length === 0) return match
                    return { entry, logLines }
                  } catch {
                    return match
                  }
                }, null)
                if (!causeWithLog) return null
                return (
                  <div style={{ marginBottom: 8 }}>
                    <div style={{ color: 'var(--muted)', marginBottom: 4, letterSpacing: '0.1em' }}>
                      LOG SNIPPET ({causeWithLog.entry.node_name} · last {causeWithLog.logLines.length} lines)
                    </div>
                    <div
                      style={{
                        borderTop: '1px solid var(--border)',
                        paddingTop: 4,
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-all',
                        color: 'var(--danger)',
                        maxHeight: 120,
                        overflowY: 'auto',
                      }}
                    >
                      {causeWithLog.logLines.slice(-10).join('\n')}
                    </div>
                  </div>
                )
              })()}

              {aiPending && typeof meta.ai_analysis !== 'string' && (
                <div style={{ marginBottom: 8 }}>
                  <div style={{ color: 'var(--muted)', marginBottom: 4, letterSpacing: '0.1em' }}>
                    AI ANALYSIS
                    {typeof meta.ai_model === 'string' && (
                      <span style={{ color: 'var(--accent)', marginLeft: 8 }}>
                        [AI · {meta.ai_model}]
                      </span>
                    )}
                  </div>
                  <div
                    style={{
                      borderTop: '1px solid var(--border)',
                      paddingTop: 4,
                      color: 'var(--accent)',
                      lineHeight: 1.5,
                    }}
                  >
                    thinking...
                  </div>
                </div>
              )}

              {typeof meta.ai_analysis === 'string' && (
                <div>
                  <div style={{ color: 'var(--muted)', marginBottom: 4, letterSpacing: '0.1em' }}>
                    AI ANALYSIS
                    {typeof meta.ai_model === 'string' && (
                      <span style={{ color: 'var(--accent)', marginLeft: 8 }}>
                        [AI · {meta.ai_model}]
                      </span>
                    )}
                  </div>
                  <div
                    style={{
                      borderTop: '1px solid var(--border)',
                      paddingTop: 4,
                      color: 'var(--text)',
                      lineHeight: 1.5,
                    }}
                  >
                    {meta.ai_analysis}
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}

export default function IncidentsPage() {
  const [openIncidents, setOpenIncidents] = useState<Incident[]>([])
  const [resolvedIncidents, setResolvedIncidents] = useState<Incident[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const { lastMessage } = useWebSocketContext()
  const loadedRef = useRef(false)

  const load = useCallback(async () => {
    try {
      const [open, resolved] = await Promise.all([
        fetchIncidents({ status: 'open', limit: 50 }),
        fetchIncidents({ status: 'resolved', limit: 20 }),
      ])
      const openIDs = new Set(open.incidents.map(incident => incident.id))
      const resolvedIDs = new Set(resolved.incidents.map(incident => incident.id))

      setOpenIncidents(prev => mergeAndDedupeIncidents(
        open.incidents,
        prev.filter(incident => incident.status === 'open' && !resolvedIDs.has(incident.id)),
      ))
      setResolvedIncidents(prev => mergeAndDedupeIncidents(
        resolved.incidents,
        prev.filter(incident => incident.status === 'resolved' && !openIDs.has(incident.id)),
      ))
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load incidents')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!loadedRef.current) {
      loadedRef.current = true
      void load()
    }
  }, [load])

  useEffect(() => {
    if (!lastMessage) return
    const { type, data } = lastMessage
    if (type === 'incident_opened') {
      const inc = data as Incident
      setOpenIncidents(prev => mergeAndDedupeIncidents([inc], prev.filter(i => i.id !== inc.id)))
      setResolvedIncidents(prev => prev.filter(i => i.id !== inc.id))
    } else if (type === 'incident_updated') {
      const inc = data as Incident
      if (inc.status === 'resolved') {
        setOpenIncidents(prev => prev.filter(i => i.id !== inc.id))
        setResolvedIncidents(prev => mergeAndDedupeIncidents([inc], prev.filter(i => i.id !== inc.id)))
      } else {
        setOpenIncidents(prev => mergeAndDedupeIncidents([inc], prev.filter(i => i.id !== inc.id)))
        setResolvedIncidents(prev => prev.filter(i => i.id !== inc.id))
      }
    } else if (type === 'incident_resolved') {
      const inc = data as Incident
      setOpenIncidents(prev => prev.filter(i => i.id !== inc.id))
      setResolvedIncidents(prev => mergeAndDedupeIncidents([inc], prev.filter(i => i.id !== inc.id)))
    }
  }, [lastMessage])

  if (loading) {
    return (
      <div style={{ padding: 24, color: 'var(--muted)', fontSize: 12 }}>loading...</div>
    )
  }

  if (error) {
    return (
      <div style={{ padding: 24, color: 'var(--danger)', fontSize: 12 }}>{error}</div>
    )
  }

  return (
    <div style={{ padding: '16px 24px', fontFamily: 'inherit' }}>
      <div style={{ marginBottom: 24 }}>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            marginBottom: 8,
            fontSize: 11,
            color: 'var(--muted)',
            letterSpacing: '0.1em',
          }}
        >
          <AlertTriangle size={12} />
          OPEN
          {openIncidents.length > 0 && (
            <span style={{ color: 'var(--danger)' }}>({openIncidents.length})</span>
          )}
        </div>
        {openIncidents.length === 0 ? (
          <div style={{ fontSize: 12, color: 'var(--muted)', padding: '8px 0' }}>
            No open incidents.
          </div>
        ) : (
          openIncidents.map(inc => (
            <IncidentCard key={inc.id} incident={inc} />
          ))
        )}
      </div>

      {resolvedIncidents.length > 0 && (
        <div>
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              marginBottom: 8,
              fontSize: 11,
              color: 'var(--muted)',
              letterSpacing: '0.1em',
            }}
          >
            <CheckCircle size={12} />
            RESOLVED
          </div>
          {resolvedIncidents.map(inc => (
            <IncidentCard key={inc.id} incident={inc} />
          ))}
        </div>
      )}
    </div>
  )
}
