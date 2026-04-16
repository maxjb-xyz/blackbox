interface StatTileProps {
  value: string | number
  label: string
  accentColor: string
  valueColor: string
}

function StatTile({ value, label, accentColor, valueColor }: StatTileProps) {
  return (
    <div
      className="stat-tile"
      style={{
        background: '#0F0F0F',
        border: '1px solid #1E1E1E',
        padding: '16px 18px',
        position: 'relative',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: 3,
          background: accentColor,
        }}
      />
      <div
        className="stat-tile-value"
        style={{
          fontSize: 30,
          fontWeight: 700,
          lineHeight: 1,
          marginBottom: 6,
          color: valueColor,
          letterSpacing: '-0.02em',
        }}
      >
        {value}
      </div>
      <div className="stat-tile-label" style={{ fontSize: 10, color: '#666', letterSpacing: '0.12em' }}>
        {label}
      </div>
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
    <div
      className="stat-row"
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(4, 1fr)',
        gap: 10,
        marginBottom: 28,
      }}
    >
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
