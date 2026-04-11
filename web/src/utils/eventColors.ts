export function eventBorderColor(event: string): string {
  const e = event.toLowerCase()
  if (e === 'stop' || e === 'stopped' || e === 'die' || e === 'down' || e === 'shutdown' || e === 'failed') return 'var(--danger)'
  if (e === 'start' || e === 'started' || e === 'up') return 'var(--success)'
  if (e === 'pull') return 'var(--info)'
  if (e === 'restart' || e === 'update') return 'var(--warning)'
  return 'var(--border)'
}

export function eventTextColor(event: string): string {
  const e = event.toLowerCase()
  if (e === 'stop' || e === 'stopped' || e === 'die' || e === 'down' || e === 'shutdown' || e === 'failed') return 'var(--danger)'
  if (e === 'start' || e === 'started' || e === 'up') return 'var(--success)'
  if (e === 'pull') return 'var(--info)'
  if (e === 'restart' || e === 'update') return 'var(--warning)'
  return 'var(--text)'
}
