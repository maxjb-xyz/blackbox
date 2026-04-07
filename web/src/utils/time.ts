function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

function parseDate(value?: string | null | Date): Date | null {
  if (!value) return null
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return date
}

export function formatLocalTimestamp(value?: string | null | Date, options?: { includeSeconds?: boolean }): string {
  const date = parseDate(value)
  if (!date) return ''

  const year = date.getFullYear()
  const month = pad2(date.getMonth() + 1)
  const day = pad2(date.getDate())
  const hours = pad2(date.getHours())
  const minutes = pad2(date.getMinutes())
  const seconds = pad2(date.getSeconds())

  return options?.includeSeconds
    ? `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
    : `${year}-${month}-${day} ${hours}:${minutes}`
}

export function formatLocalDate(value?: string | null | Date): string {
  const date = parseDate(value)
  if (!date) return ''

  const year = date.getFullYear()
  const month = pad2(date.getMonth() + 1)
  const day = pad2(date.getDate())
  return `${year}-${month}-${day}`
}
