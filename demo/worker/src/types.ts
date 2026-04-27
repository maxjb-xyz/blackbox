export interface SessionUser {
  user_id: string
  username: string
  is_admin: boolean
  email: string
  oidc_linked: boolean
}

export interface SetupStatus {
  bootstrapped: boolean
}

export interface HealthStatus {
  database: 'ok' | 'error'
  oidc: 'ok' | 'unavailable' | 'disabled'
  oidc_enabled: boolean
}

export interface Entry {
  id: string
  timestamp: string
  node_name: string
  source: string
  service: string
  compose_service?: string
  event: string
  content: string
  metadata: Record<string, unknown>
  correlated_id?: string
}

export interface Node {
  id: string
  name: string
  last_seen: string
  status: 'online' | 'offline'
  agent_version: string
  ip_address: string
  os_info: string
  capabilities: string[]
}

export interface Incident {
  id: string
  opened_at: string
  resolved_at: string | null
  status: 'open' | 'resolved'
  confidence: 'confirmed' | 'suspected'
  title: string
  services: string[]
  root_cause_id?: string
  trigger_id?: string
  node_names: string[]
  metadata: Record<string, unknown>
}

export interface IncidentLinkRecord {
  incident_id: string
  entry_id: string
  role: 'trigger' | 'cause' | 'immediate_cause' | 'context' | 'evidence' | 'recovery' | 'ai_cause'
  score: number
  reason?: string
}

export interface AdminUser {
  id: string
  username: string
  is_admin: boolean
  token_version: number
  created_at: string
  email?: string
}

export interface AdminConfig {
  webhook_secret: string
  file_watcher_redact_secrets: boolean
  ai_provider: 'ollama' | 'openai_compat'
  ai_url: string
  ai_model: string
  ai_api_key_set: boolean
  ai_mode: 'analysis' | 'enhanced'
  mcp_enabled: boolean
  mcp_port: number
  mcp_auth_token_set: boolean
  mcp_auth_token_suffix: string
  mcp_running: boolean
  base_url: string
}

export interface AuditLogEntry {
  id: string
  actor_user_id: string
  actor_email: string
  action: string
  target_type: string
  target_id: string
  metadata: Record<string, unknown>
  ip_address: string
  created_at: string
}

export interface WebhookDelivery {
  id: string
  source: string
  received_at: string
  payload_snippet: string
  matched_incident_id: string
  status: string
  error_message: string
}

export interface DataSourceInstance {
  id: string
  type: string
  scope: 'server' | 'agent'
  node_id?: string
  name: string
  config: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface SourceTypeDef {
  type: string
  scope: 'server' | 'agent'
  singleton: boolean
  name: string
  description: string
  mechanism: string
}

export interface NodeSources {
  capabilities: string[]
  agent_version: string
  status: string
  sources: DataSourceInstance[]
}

export interface SourcesResponse {
  server: DataSourceInstance[]
  nodes: Record<string, NodeSources>
  orphans: DataSourceInstance[]
}

export interface NotificationDest {
  id: string
  name: string
  type: 'discord' | 'slack' | 'ntfy'
  url: string
  events: string[]
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface ExcludedTarget {
  id: string
  type: 'container' | 'stack'
  name: string
  created_at: string
}

export interface OIDCProviderConfig {
  id: string
  name: string
  issuer: string
  client_id: string
  client_secret: string
  redirect_url: string
  require_verified_email: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface InviteCode {
  id: string
  code: string
  used: boolean
  created_at: string
  expires_at: string
}

export interface WebhookEndpoint {
  id: string
  method: string
  path: string
  source: string
  description: string
}

export interface DemoData {
  dataCreatedAt: number
  setupStatus: SetupStatus
  healthStatus: HealthStatus
  sessionUser: SessionUser
  nodes: Node[]
  users: AdminUser[]
  entries: Entry[]
  incidents: Incident[]
  incidentLinks: IncidentLinkRecord[]
  auditLogs: AuditLogEntry[]
  webhookDeliveries: WebhookDelivery[]
  notificationDests: NotificationDest[]
  dataSourceInstances: DataSourceInstance[]
  sourceTypes: SourceTypeDef[]
  excludedTargets: ExcludedTarget[]
  oidcProviders: OIDCProviderConfig[]
  oidcPolicy: 'open' | 'existing_only' | 'invite_required'
  invites: InviteCode[]
  adminConfig: AdminConfig
  webhooks: WebhookEndpoint[]
  sources: SourcesResponse
}

