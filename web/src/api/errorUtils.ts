export async function readErrorMessage(res: Response, fallback: string) {
  const text = await res.text().catch(() => '')
  if (!text) return fallback
  try {
    const data = JSON.parse(text) as { error?: string }
    if (typeof data.error === 'string' && data.error) return data.error
  } catch {
    return text
  }
  return text
}
