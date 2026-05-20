export function resolveMcpEndpointUrl(
  endpointUrl: string | null | undefined,
  origin: string,
  configuredBaseUrl?: string | null | undefined,
): string {
  const trimmedEndpointUrl = endpointUrl?.trim()
  if (trimmedEndpointUrl) return trimmedEndpointUrl
  if (configuredBaseUrl?.trim()) return ''

  const normalizedOrigin = origin.endsWith('/') ? origin : `${origin}/`
  return new URL('mcp', normalizedOrigin).toString()
}
