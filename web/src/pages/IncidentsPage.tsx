import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import {
  fetchIncident,
  fetchIncidents,
  type Incident,
  type IncidentDetail,
  parseIncidentMetadata,
  parseIncidentNodes,
  parseIncidentServices,
} from '../api/client'
import { useNodePulse } from '../components/NodePulse'
import { useWebSocketContext } from '../components/WebSocketProvider'
import PageHeader from '../components/PageHeader'
import StatRow from '../components/StatRow'
import { formatLocalTimestamp } from '../utils/time'

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
  const dash = '\u2014'
  if (!ts) return dash
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return dash
  return formatLocalTimestamp(d) || dash
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
        // ignore errors; detail just won't show
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
              {'\u00B7'} {nodes.join(', ')}
            </span>
          )}
        </span>
        {statusLabel(incident)}
        {aiPending && (
          <span style={{ color: 'var(--accent)', fontSize: 11, marginLeft: 12, whiteSpace: 'nowrap', letterSpacing: '0.1em', border: '1px solid var(--accent)', padding: '2px 6px', lineHeight: 1.4 }}>
            AI THINKING
          </span>
        )}
        <span style={{ fontSize: 11, color: 'var(--muted)', marginLeft: 12, whiteSpace: 'nowrap' }}>
          {formatTs(incident.opened_at)}
          {' \u2192 '}
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
                <div style={{ color: 'var(--muted)', marginBottom: 4, letterSpacing: '0.1em', display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span>EVENT CHAIN</span>
                  {aiCauseEntries.length === 0 && meta.ai_verified === true && !aiPending && (
                    <span
                      style={{
                        border: '1px solid var(--accent)',
                        color: 'var(--accent)',
                        fontSize: 10,
                        lineHeight: 1.4,
                        padding: '2px 6px',
                        letterSpacing: '0.08em',
                      }}
                    >
                      AI VERIFIED
                    </span>
                  )}
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
                        {link.role === 'cause' && detailIncident.root_cause_id === link.entry_id && ' \u2605'}
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
                        lineHeight: 1.4,
                        padding: '2px 6px',
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
                const logSourceRoles = ['cause', 'trigger', 'evidence', 'ai_cause', 'recovery']
                const entryWithLog = logSourceRoles.reduce<{
                  entry: IncidentDetail['entries'][number]['entry']
                  logLines: string[]
                } | null>((match, role) => {
                  if (match) return match
                  for (const { link, entry } of detail.entries) {
                    if (link.role !== role) continue
                    try {
                      const metadata = JSON.parse(entry.metadata) as Record<string, unknown>
                      const logLines = Array.isArray(metadata.log_snippet) ? metadata.log_snippet as string[] : []
                      if (logLines.length === 0) continue
                      return { entry, logLines }
                    } catch {
                      continue
                    }
                  }
                  return match
                }, null)
                if (!entryWithLog) return null
                return (
                  <div style={{ marginBottom: 8 }}>
                    <div style={{ color: 'var(--muted)', marginBottom: 4, letterSpacing: '0.1em' }}>
                      LOG SNIPPET ({entryWithLog.entry.node_name} {'\u00B7'} last {entryWithLog.logLines.length} lines)
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
                      {entryWithLog.logLines.slice(-10).join('\n')}
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
                        [AI {'\u00B7'} {meta.ai_model}]
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
                        [AI {'\u00B7'} {meta.ai_model}]
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

function isToday(dateString: string | null | undefined): boolean {
  if (!dateString) return false
  const d = new Date(dateString)
  if (Number.isNaN(d.getTime())) return false
  const now = new Date()
  return (
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate()
  )
}

export default function IncidentsPage() {
  const [openIncidents, setOpenIncidents] = useState<Incident[]>([])
  const [resolvedIncidents, setResolvedIncidents] = useState<Incident[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const { onlineCount, totalCount } = useNodePulse()
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

  const confirmed = openIncidents.filter(i => i.confidence === 'confirmed').length
  const suspected = openIncidents.filter(i => i.confidence !== 'confirmed').length

  const resolvedToday = resolvedIncidents.filter(i => isToday(i.resolved_at)).length

  return (
    <div style={{ fontFamily: 'inherit' }}>
      <PageHeader title="INCIDENTS" subtitle="real-time incident tracking" />
      <div style={{ padding: '20px 28px 48px' }}>
        <StatRow
          confirmed={confirmed}
          suspected={suspected}
          nodesOnline={onlineCount}
          nodesTotal={totalCount}
          resolvedToday={resolvedToday}
        />

        <div style={{ marginBottom: 28 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
            <span style={{ fontSize: 11, color: 'var(--muted)', letterSpacing: '0.14em' }}>OPEN</span>
            {openIncidents.length > 0 && (
              <span style={{ fontSize: 11, color: 'var(--danger)', letterSpacing: '0.1em' }}>
                {openIncidents.length}
              </span>
            )}
            <div style={{ flex: 1, height: 1, background: '#1E1E1E' }} />
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
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
              <span style={{ fontSize: 11, color: 'var(--muted)', letterSpacing: '0.14em' }}>RECENTLY RESOLVED</span>
              <div style={{ flex: 1, height: 1, background: '#1E1E1E' }} />
            </div>
            {resolvedIncidents.map(inc => (
              <IncidentCard key={inc.id} incident={inc} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
