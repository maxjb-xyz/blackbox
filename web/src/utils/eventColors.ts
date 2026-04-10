export function eventBorderColor(event: string): string {
  const e = event.toLowerCase()
  if (e === 'stop' || e === 'die' || e === 'down' || e === 'shutdown') return 'var(--danger)'
  if (e === 'start' || e === 'up') return 'var(--success)'
  if (e === 'pull') return 'var(--info)'
  if (e === 'restart' || e === 'update') return 'var(--warning)'
  return 'var(--border)'
}

export function eventTextColor(event: string): string {
  const e = event.toLowerCase()
  if (e === 'stop' || e === 'die' || e === 'down' || e === 'shutdown') return 'var(--danger)'
  if (e === 'start' || e === 'up') return 'var(--success)'
  if (e === 'pull') return 'var(--info)'
  if (e === 'restart' || e === 'update') return 'var(--warning)'
  return 'var(--text)'
}
