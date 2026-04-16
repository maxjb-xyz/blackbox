import type { CSSProperties } from 'react'

interface StatTileProps {
  value: string | number
  label: string
  accentColor: string
  valueColor: string
}

function StatTile({ value, label, accentColor, valueColor }: StatTileProps) {
  const tileVars = {
    '--stat-accent': accentColor,
    '--stat-value-color': valueColor,
  } as CSSProperties

  return (
    <div className="stat-tile" style={tileVars}>
      <div className="stat-tile-accent" />
      <div className="stat-tile-value">{value}</div>
      <div className="stat-tile-label">{label}</div>
    </div>
  )
}

interface StatRowProps {
  confirmed: number
  suspected: number
  nodesOnline: number
  nodesTotal: number
  resolvedToday: number
}

export default function StatRow({
  confirmed,
  suspected,
  nodesOnline,
  nodesTotal,
  resolvedToday,
}: StatRowProps) {
  const nodesValue = nodesTotal > 0 ? `${nodesOnline}/${nodesTotal}` : '—'

  return (
    <div className="stat-row">
      <StatTile
        value={confirmed}
        label="CONFIRMED"
        accentColor="var(--danger)"
        valueColor="var(--danger)"
      />
      <StatTile
        value={suspected}
        label="SUSPECTED"
        accentColor="var(--warning)"
        valueColor="var(--warning)"
      />
      <StatTile
        value={nodesValue}
        label="NODES ONLINE"
        accentColor="var(--info)"
        valueColor="var(--info)"
      />
      <StatTile
        value={resolvedToday}
        label="RESOLVED TODAY"
        accentColor="var(--success)"
        valueColor="var(--success)"
      />
    </div>
  )
}
