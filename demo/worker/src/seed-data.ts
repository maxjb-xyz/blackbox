import type {
  AdminConfig,
  AdminUser,
  AuditLogEntry,
  DataSourceInstance,
  DemoData,
  Entry,
  ExcludedTarget,
  HealthStatus,
  Incident,
  IncidentLinkRecord,
  InviteCode,
  Node,
  NotificationDest,
  OIDCProviderConfig,
  SessionUser,
  SourceTypeDef,
  SourcesResponse,
  WebhookDelivery,
  WebhookEndpoint,
} from './types'

const MINUTE = 60_000
const DAY = 24 * 60

const nodeCatalog = [
  {
    id: 'node-mikoshi',
    name: 'mikoshi',
    status: 'online' as const,
    agent_version: '1.14.2',
    ip_address: '10.0.10.11',
    os_info: 'docker / Debian 12',
    capabilities: ['docker', 'filewatcher', 'tailscale', 'volumes'],
    services: ['nginx', 'jellyfin', 'vaultwarden', 'gitea', 'postgres', 'redis', 'immich-server'],
  },
  {
    id: 'node-hexcore',
    name: 'hexcore',
    status: 'online' as const,
    agent_version: '1.14.2',
    ip_address: '10.0.10.22',
    os_info: 'docker / Ubuntu 24.04',
    capabilities: ['docker', 'gpu', 'systemd'],
    services: ['ollama', 'paperless', 'minio', 'buildkitd', 'postgres-exporter', 'scrutiny-web'],
  },
  {
    id: 'node-atlas',
    name: 'atlas',
    status: 'online' as const,
    agent_version: '1.14.1',
    ip_address: '10.0.10.30',
    os_info: 'docker / Alpine 3.21',
    capabilities: ['docker', 'systemd', 'edge-proxy'],
    services: ['caddy', 'uptime-kuma', 'crowdsec', 'tailscale', 'oauth2-proxy'],
  },
]

function iso(nowMs: number, minutesAgo: number): string {
  return new Date(nowMs - minutesAgo * MINUTE).toISOString()
}

function dockerMetadata(service: string, nodeName: string, phase: string, extra: Record<string, unknown> = {}) {
  return {
    image: `ghcr.io/maxjb-xyz/${service}:latest`,
    container_name: `${service}-${nodeName}`,
    'com.docker.compose.service': service,
    phase,
    ...extra,
  }
}

function buildSources(nowMs: number, nodes: Node[]): { dataSourceInstances: DataSourceInstance[]; sources: SourcesResponse } {
  const createdAt = iso(nowMs, 12 * DAY)
  const updatedAt = iso(nowMs, 180)
  const dataSourceInstances: DataSourceInstance[] = [
    {
      id: 'source-kuma',
      type: 'webhook_uptime_kuma',
      scope: 'server',
      name: 'Uptime Kuma',
      config: JSON.stringify({ secret: 'demo-kuma-secret', expected_path: '/api/webhooks/uptime' }),
      enabled: true,
      created_at: createdAt,
      updated_at: updatedAt,
    },
    {
      id: 'source-watchtower',
      type: 'webhook_watchtower',
      scope: 'server',
      name: 'Watchtower',
      config: JSON.stringify({ secret: 'demo-watchtower-secret', expected_path: '/api/webhooks/watchtower' }),
      enabled: true,
      created_at: createdAt,
      updated_at: updatedAt,
    },
    {
      id: 'source-filewatcher-mikoshi',
      type: 'filewatcher',
      scope: 'agent',
      node_id: 'mikoshi',
      name: 'File Watcher',
      config: JSON.stringify({ paths: ['/etc/nginx', '/srv/compose'], redact_secrets: true }),
      enabled: true,
      created_at: createdAt,
      updated_at: updatedAt,
    },
    {
      id: 'source-systemd-hexcore',
      type: 'systemd',
      scope: 'agent',
      node_id: 'hexcore',
      name: 'Systemd',
      config: JSON.stringify({ units: ['nvidia-persistenced.service', 'tailscaled.service'] }),
      enabled: true,
      created_at: createdAt,
      updated_at: updatedAt,
    },
    {
      id: 'source-systemd-atlas',
      type: 'systemd',
      scope: 'agent',
      node_id: 'atlas',
      name: 'Systemd',
      config: JSON.stringify({ units: ['caddy.service', 'tailscaled.service'] }),
      enabled: true,
      created_at: createdAt,
      updated_at: updatedAt,
    },
  ]

  const sources: SourcesResponse = {
    server: dataSourceInstances.filter(source => source.scope === 'server'),
    nodes: {},
    orphans: [],
  }

  for (const node of nodes) {
    sources.nodes[node.name] = {
      capabilities: node.capabilities,
      agent_version: node.agent_version,
      status: node.status,
      sources: dataSourceInstances.filter(source => source.scope === 'agent' && source.node_id === node.name),
    }
  }

  return { dataSourceInstances, sources }
}

export function createDemoData(nowMs = Date.now()): DemoData {
  const setupStatus = { bootstrapped: true }
  const healthStatus: HealthStatus = { database: 'ok', oidc: 'disabled', oidc_enabled: false }

  const sessionUser: SessionUser = {
    user_id: 'user-admin',
    username: 'operator',
    is_admin: true,
    email: 'operator@blackboxd.dev',
    oidc_linked: false,
  }

  const nodes: Node[] = nodeCatalog.map((node, index) => ({
    id: node.id,
    name: node.name,
    status: node.status,
    last_seen: iso(nowMs, index * 3 + 1),
    agent_version: node.agent_version,
    ip_address: node.ip_address,
    os_info: node.os_info,
    capabilities: node.capabilities,
  }))

  const users: AdminUser[] = [
    { id: 'user-admin', username: 'operator', is_admin: true, token_version: 8, created_at: iso(nowMs, 140 * DAY), email: 'operator@blackboxd.dev' },
    { id: 'user-jinx', username: 'jinx', is_admin: false, token_version: 3, created_at: iso(nowMs, 120 * DAY), email: 'jinx@blackboxd.dev' },
    { id: 'user-ekko', username: 'ekko', is_admin: false, token_version: 2, created_at: iso(nowMs, 90 * DAY), email: 'ekko@blackboxd.dev' },
    { id: 'user-jayce', username: 'jayce', is_admin: false, token_version: 1, created_at: iso(nowMs, 45 * DAY), email: 'jayce@blackboxd.dev' },
  ]

  const sourceTypes: SourceTypeDef[] = [
    {
      type: 'webhook_uptime_kuma',
      scope: 'server',
      singleton: true,
      name: 'Uptime Kuma webhook',
      description: 'Receives synthetic availability events from the external probe stack.',
      mechanism: 'Inbound webhook',
    },
    {
      type: 'webhook_watchtower',
      scope: 'server',
      singleton: true,
      name: 'Watchtower webhook',
      description: 'Receives container update notifications from rolling image refreshes.',
      mechanism: 'Inbound webhook',
    },
    {
      type: 'filewatcher',
      scope: 'agent',
      singleton: true,
      name: 'File watcher',
      description: 'Watches local config paths and captures diffs for interesting changes.',
      mechanism: 'Agent-side file watcher',
    },
    {
      type: 'systemd',
      scope: 'agent',
      singleton: true,
      name: 'Systemd watcher',
      description: 'Tracks unit start/stop events on nodes that still expose host services.',
      mechanism: 'Agent-side unit polling',
    },
  ]

  const { dataSourceInstances, sources } = buildSources(nowMs, nodes)

  const entries: Entry[] = []
  const incidents: Incident[] = []
  const incidentLinks: IncidentLinkRecord[] = []
  let entryCounter = 0
  let incidentCounter = 0

  const pushEntry = (spec: Omit<Entry, 'id'>): string => {
    const id = `entry-${String(++entryCounter).padStart(4, '0')}`
    entries.push({ id, ...spec })
    return id
  }

  const pushIncident = (spec: Omit<Incident, 'id'>): string => {
    const id = `incident-${String(++incidentCounter).padStart(3, '0')}`
    incidents.push({ id, ...spec })
    return id
  }

  const linkIncident = (
    incidentId: string,
    entryId: string,
    role: IncidentLinkRecord['role'],
    score: number,
    reason?: string,
  ) => {
    incidentLinks.push({ incident_id: incidentId, entry_id: entryId, role, score, ...(reason ? { reason } : {}) })
  }

  const routinePatterns = [
    { offset: 1320, event: 'pull', phase: 'prefetch', content: (service: string) => `pulled ${service}:latest for the morning refresh window` },
    { offset: 1290, event: 'create', phase: 'replace', content: (service: string, node: string) => `created replacement ${service} container on ${node}` },
    { offset: 1286, event: 'start', phase: 'replace', content: (service: string) => `${service} started and entered healthcheck warmup` },
    { offset: 900, event: 'stop', phase: 'handoff', content: (service: string) => `stopped previous ${service} instance after traffic handoff` },
    { offset: 896, event: 'destroy', phase: 'handoff', content: (service: string) => `removed retired ${service} container from the compose stack` },
    { offset: 540, event: 'pull', phase: 'prefetch', content: (service: string) => `prefetched ${service}:latest for the afternoon maintenance window` },
    { offset: 520, event: 'create', phase: 'refresh', content: (service: string, node: string) => `created refresh candidate for ${service} on ${node}` },
    { offset: 516, event: 'start', phase: 'refresh', content: (service: string) => `${service} came back healthy after the refresh` },
  ] as const

  for (let dayIndex = 13; dayIndex >= 0; dayIndex--) {
    const dayOffset = dayIndex * DAY

    for (const [nodeIndex, node] of nodeCatalog.entries()) {
      const serviceA = node.services[(dayIndex + nodeIndex) % node.services.length]
      const serviceB = node.services[(dayIndex + nodeIndex + 2) % node.services.length]
      const serviceC = node.services[(dayIndex + nodeIndex + 4) % node.services.length]
      const serviceRotation = [serviceA, serviceA, serviceA, serviceB, serviceB, serviceC, serviceC, serviceC]

      routinePatterns.forEach((pattern, patternIndex) => {
        const service = serviceRotation[patternIndex]
        pushEntry({
          timestamp: iso(nowMs, dayOffset + pattern.offset + nodeIndex * 17),
          node_name: node.name,
          source: 'docker',
          service,
          compose_service: service,
          event: pattern.event,
          content: pattern.content(service, node.name),
          metadata: dockerMetadata(service, node.name, pattern.phase, {
            restart_count: (dayIndex + patternIndex + nodeIndex) % 3,
          }),
        })
      })
    }
  }

  for (let dayIndex = 13; dayIndex >= 1; dayIndex -= 2) {
    pushEntry({
      timestamp: iso(nowMs, dayIndex * DAY + 420),
      node_name: 'mikoshi',
      source: 'files',
      service: 'nginx',
      event: 'modified',
      content: 'updated /etc/nginx/sites-enabled/media.blackboxd.dev.conf',
      metadata: {
        path: '/etc/nginx/sites-enabled/media.blackboxd.dev.conf',
        op: 'write',
        diff_status: 'included',
        diff_redacted: false,
        diff: '@@ -3,7 +3,7 @@\n proxy_pass http://jellyfin;\n-proxy_read_timeout 30s;\n+proxy_read_timeout 90s;\n proxy_set_header Host $host;',
      },
    })
    pushEntry({
      timestamp: iso(nowMs, dayIndex * DAY + 392),
      node_name: 'mikoshi',
      source: 'files',
      service: 'compose',
      event: 'modified',
      content: 'updated /srv/compose/media-stack/docker-compose.yml',
      metadata: {
        path: '/srv/compose/media-stack/docker-compose.yml',
        op: 'write',
        diff_status: 'included',
        diff_redacted: true,
        diff: '@@ -14,6 +14,6 @@\n image: ghcr.io/maxjb-xyz/jellyfin:stable\n environment:\n-  JELLYFIN_CACHE_SIZE=256\n+  JELLYFIN_CACHE_SIZE=384',
      },
    })
  }

  for (let dayIndex = 12; dayIndex >= 0; dayIndex -= 3) {
    const service = nodeCatalog[0].services[(dayIndex + 1) % nodeCatalog[0].services.length]
    pushEntry({
      timestamp: iso(nowMs, dayIndex * DAY + 230),
      node_name: 'mikoshi',
      source: 'webhook',
      service,
      event: 'update',
      content: `watchtower scheduled ${service} for a rolling image refresh`,
      metadata: {
        'watchtower.title': `Refreshing ${service}`,
        'watchtower.level': 'info',
        result: 'queued',
      },
    })
  }

  const probePairs = [
    { minutesAgo: 9 * DAY + 180, nodeName: 'mikoshi', service: 'jellyfin', monitor: 'media-external' },
    { minutesAgo: 6 * DAY + 150, nodeName: 'mikoshi', service: 'vaultwarden', monitor: 'vaultwarden-login' },
    { minutesAgo: 4 * DAY + 240, nodeName: 'atlas', service: 'caddy', monitor: 'edge-homepage' },
  ]
  for (const pair of probePairs) {
    pushEntry({
      timestamp: iso(nowMs, pair.minutesAgo + 12),
      node_name: pair.nodeName,
      source: 'webhook',
      service: pair.service,
      event: 'down',
      content: `${pair.monitor} reported sustained HTTP failures`,
      metadata: { monitor: pair.monitor, status: 'down', response_time_ms: 1800 },
    })
    pushEntry({
      timestamp: iso(nowMs, pair.minutesAgo),
      node_name: pair.nodeName,
      source: 'webhook',
      service: pair.service,
      event: 'up',
      content: `${pair.monitor} recovered after automatic retry`,
      metadata: { monitor: pair.monitor, status: 'up', response_time_ms: 240 },
    })
  }

  const crashLoopTrigger = pushEntry({
    timestamp: iso(nowMs, 53 * 60 + 42),
    node_name: 'hexcore',
    source: 'docker',
    service: 'ollama',
    compose_service: 'ollama',
    event: 'die',
    content: 'ollama exited with code 137 while loading a 14B model',
    metadata: dockerMetadata('ollama', 'hexcore', 'incident', {
      exit_code: 137,
      oom_killed: true,
      log_snippet: [
        'llama_model_loader: mmap failed for model shard 03',
        'cudaMalloc failed: out of memory',
        'fatal error: allocator terminated process',
      ],
    }),
  })
  const crashLoopRetry = pushEntry({
    timestamp: iso(nowMs, 53 * 60 + 30),
    node_name: 'hexcore',
    source: 'docker',
    service: 'ollama',
    compose_service: 'ollama',
    event: 'start',
    content: 'ollama restarted automatically after the crash',
    metadata: dockerMetadata('ollama', 'hexcore', 'incident', {
      restart_count: 5,
    }),
  })
  const crashLoopCause = pushEntry({
    timestamp: iso(nowMs, 53 * 60 + 18),
    node_name: 'hexcore',
    source: 'docker',
    service: 'ollama',
    compose_service: 'ollama',
    event: 'die',
    content: 'ollama crashed again during model warmup',
    metadata: dockerMetadata('ollama', 'hexcore', 'incident', {
      exit_code: 137,
      oom_killed: true,
      possible_cause: 'GPU memory pressure from concurrent warmup',
      log_snippet: [
        'warmup queue stalled at request 4',
        'cudaMalloc failed: out of memory',
        'process exiting after repeated allocator faults',
      ],
    }),
  })
  const crashLoopRecovery = pushEntry({
    timestamp: iso(nowMs, 53 * 60),
    node_name: 'hexcore',
    source: 'docker',
    service: 'ollama',
    compose_service: 'ollama',
    event: 'start',
    content: 'ollama stabilized after lowering the warmup concurrency',
    metadata: dockerMetadata('ollama', 'hexcore', 'incident', {
      applied_fix: 'OLLAMA_NUM_PARALLEL=1',
    }),
    correlated_id: crashLoopCause,
  })
  const crashLoopIncident = pushIncident({
    opened_at: iso(nowMs, 53 * 60 + 42),
    resolved_at: iso(nowMs, 53 * 60),
    status: 'resolved',
    confidence: 'confirmed',
    title: 'Hexcore ollama entered a crash loop under GPU memory pressure',
    services: ['ollama'],
    root_cause_id: crashLoopCause,
    trigger_id: crashLoopTrigger,
    node_names: ['hexcore'],
    metadata: {
      summary: 'Repeated allocator failures caused the service to flap until warmup concurrency was reduced.',
      confidence_score: 0.94,
    },
  })
  linkIncident(crashLoopIncident, crashLoopTrigger, 'trigger', 97, 'first hard failure in the restart loop')
  linkIncident(crashLoopIncident, crashLoopRetry, 'context', 72)
  linkIncident(crashLoopIncident, crashLoopCause, 'immediate_cause', 99, 'second crash with matching OOM evidence')
  linkIncident(crashLoopIncident, crashLoopRecovery, 'recovery', 88, 'service remained healthy after the concurrency change')

  const watchtowerUpdate = pushEntry({
    timestamp: iso(nowMs, 28 * 60 + 48),
    node_name: 'mikoshi',
    source: 'webhook',
    service: 'vaultwarden',
    event: 'update',
    content: 'watchtower applied a fresh vaultwarden image on mikoshi',
    metadata: {
      'watchtower.title': 'Refreshing vaultwarden',
      'watchtower.level': 'warn',
      image: 'vaultwarden/server:1.33.0',
    },
  })
  const watchtowerPull = pushEntry({
    timestamp: iso(nowMs, 28 * 60 + 44),
    node_name: 'mikoshi',
    source: 'docker',
    service: 'vaultwarden',
    compose_service: 'vaultwarden',
    event: 'pull',
    content: 'pulled vaultwarden/server:1.33.0',
    metadata: dockerMetadata('vaultwarden', 'mikoshi', 'incident', {
      image: 'vaultwarden/server:1.33.0',
    }),
  })
  const vaultwardenDown = pushEntry({
    timestamp: iso(nowMs, 28 * 60 + 30),
    node_name: 'mikoshi',
    source: 'webhook',
    service: 'vaultwarden',
    event: 'down',
    content: 'vaultwarden-login monitor reported 502s after the image refresh',
    metadata: {
      monitor: 'vaultwarden-login',
      status: 'down',
      response_time_ms: 2030,
    },
  })
  const vaultwardenDie = pushEntry({
    timestamp: iso(nowMs, 28 * 60 + 27),
    node_name: 'mikoshi',
    source: 'docker',
    service: 'vaultwarden',
    compose_service: 'vaultwarden',
    event: 'die',
    content: 'vaultwarden failed boot after the image update',
    metadata: dockerMetadata('vaultwarden', 'mikoshi', 'incident', {
      exit_code: 1,
      possible_cause: 'new image expected ADMIN_TOKEN_FILE but only ADMIN_TOKEN was set',
      log_snippet: [
        'error: invalid env combination detected',
        'expected ADMIN_TOKEN_FILE or disabled admin interface',
        'shutting down before accepting connections',
      ],
    }),
  })
  const rollbackEdit = pushEntry({
    timestamp: iso(nowMs, 28 * 60 + 18),
    node_name: 'mikoshi',
    source: 'files',
    service: 'vaultwarden',
    event: 'modified',
    content: 'reverted vaultwarden image pin in /srv/compose/security-stack/docker-compose.yml',
    metadata: {
      path: '/srv/compose/security-stack/docker-compose.yml',
      op: 'write',
      diff_status: 'included',
      diff_redacted: true,
      diff: '@@ -8,7 +8,7 @@\n-image: vaultwarden/server:1.33.0\n+image: vaultwarden/server:1.32.7',
    },
  })
  const vaultwardenRecovery = pushEntry({
    timestamp: iso(nowMs, 28 * 60 + 11),
    node_name: 'mikoshi',
    source: 'docker',
    service: 'vaultwarden',
    compose_service: 'vaultwarden',
    event: 'start',
    content: 'vaultwarden recovered after the rollback',
    metadata: dockerMetadata('vaultwarden', 'mikoshi', 'incident', {
      recovered_with: 'vaultwarden/server:1.32.7',
    }),
    correlated_id: vaultwardenDie,
  })
  const vaultwardenUp = pushEntry({
    timestamp: iso(nowMs, 28 * 60 + 6),
    node_name: 'mikoshi',
    source: 'webhook',
    service: 'vaultwarden',
    event: 'up',
    content: 'vaultwarden-login monitor returned to normal after rollback',
    metadata: {
      monitor: 'vaultwarden-login',
      status: 'up',
      response_time_ms: 180,
    },
    correlated_id: vaultwardenDown,
  })
  const watchtowerIncident = pushIncident({
    opened_at: iso(nowMs, 28 * 60 + 48),
    resolved_at: iso(nowMs, 28 * 60 + 6),
    status: 'resolved',
    confidence: 'suspected',
    title: 'Vaultwarden degraded after a Watchtower image refresh on mikoshi',
    services: ['vaultwarden'],
    root_cause_id: vaultwardenDie,
    trigger_id: watchtowerUpdate,
    node_names: ['mikoshi'],
    metadata: {
      summary: 'Rollback restored service quickly, but the failure pattern strongly implicates the image update.',
      confidence_score: 0.66,
    },
  })
  linkIncident(watchtowerIncident, watchtowerUpdate, 'trigger', 84, 'Watchtower reported the refresh immediately before the outage')
  linkIncident(watchtowerIncident, watchtowerPull, 'context', 61)
  linkIncident(watchtowerIncident, vaultwardenDown, 'evidence', 92, 'external probe saw user-facing downtime')
  linkIncident(watchtowerIncident, vaultwardenDie, 'cause', 95, 'container exited during boot with config mismatch')
  linkIncident(watchtowerIncident, rollbackEdit, 'context', 76)
  linkIncident(watchtowerIncident, vaultwardenRecovery, 'recovery', 90, 'service stabilized after reverting the image pin')
  linkIncident(watchtowerIncident, vaultwardenUp, 'recovery', 79)

  const atlasDown = pushEntry({
    timestamp: iso(nowMs, 3 * 60 + 20),
    node_name: 'atlas',
    source: 'webhook',
    service: 'caddy',
    event: 'down',
    content: 'edge-homepage monitor is still seeing intermittent 502s via atlas',
    metadata: {
      monitor: 'edge-homepage',
      status: 'down',
      response_time_ms: 2500,
      region: 'iad',
    },
  })
  const atlasContext = pushEntry({
    timestamp: iso(nowMs, 3 * 60 + 14),
    node_name: 'atlas',
    source: 'docker',
    service: 'crowdsec',
    compose_service: 'crowdsec',
    event: 'start',
    content: 'crowdsec reloaded after pulling the latest collections bundle',
    metadata: dockerMetadata('crowdsec', 'atlas', 'context', {
      collections: ['crowdsecurity/http-cve', 'crowdsecurity/nginx'],
    }),
  })
  const atlasCauseHint = pushEntry({
    timestamp: iso(nowMs, 3 * 60 + 8),
    node_name: 'atlas',
    source: 'files',
    service: 'caddy',
    event: 'modified',
    content: 'edited /etc/caddy/Caddyfile during live troubleshooting',
    metadata: {
      path: '/etc/caddy/Caddyfile',
      op: 'write',
      diff_status: 'included',
      diff_redacted: false,
      diff: '@@ -21,7 +21,7 @@\n reverse_proxy auth.blackboxd.dev oauth2-proxy:4180\n-handle_errors {\n+handle_errors {\n   respond \"temporary upstream issue\" 502',
    },
  })
  const atlasIncident = pushIncident({
    opened_at: iso(nowMs, 3 * 60 + 20),
    resolved_at: null,
    status: 'open',
    confidence: 'suspected',
    title: 'Atlas edge proxy is still flapping on public health checks',
    services: ['caddy', 'oauth2-proxy'],
    trigger_id: atlasDown,
    node_names: ['atlas'],
    metadata: {
      summary: 'Probe failures are real, but the root cause is still ambiguous during live troubleshooting.',
      confidence_score: 0.42,
    },
  })
  linkIncident(atlasIncident, atlasDown, 'trigger', 88, 'public edge checks are still failing')
  linkIncident(atlasIncident, atlasContext, 'context', 33)
  linkIncident(atlasIncident, atlasCauseHint, 'evidence', 49, 'operator changed the edge config during the same window')

  const minorIncidents = [
    { node: 'mikoshi', service: 'gitea', title: 'Gitea restarted during package refresh', minutesAgo: 12 * DAY + 95, confidence: 'confirmed' as const, mode: 'docker' as const },
    { node: 'hexcore', service: 'paperless', title: 'Paperless worker stalled and restarted cleanly', minutesAgo: 11 * DAY + 160, confidence: 'suspected' as const, mode: 'docker' as const },
    { node: 'atlas', service: 'oauth2-proxy', title: 'OAuth proxy briefly failed readiness checks', minutesAgo: 10 * DAY + 220, confidence: 'suspected' as const, mode: 'webhook' as const },
    { node: 'mikoshi', service: 'jellyfin', title: 'Jellyfin went dark during a storage remount', minutesAgo: 9 * DAY + 310, confidence: 'confirmed' as const, mode: 'webhook' as const },
    { node: 'hexcore', service: 'minio', title: 'Minio restarted after host kernel updates', minutesAgo: 8 * DAY + 245, confidence: 'confirmed' as const, mode: 'docker' as const },
    { node: 'atlas', service: 'tailscale', title: 'Atlas tailscale reconnect caused a short ingress flap', minutesAgo: 7 * DAY + 205, confidence: 'suspected' as const, mode: 'docker' as const },
    { node: 'mikoshi', service: 'immich-server', title: 'Immich queue backlog cleared after service recycle', minutesAgo: 6 * DAY + 320, confidence: 'confirmed' as const, mode: 'docker' as const },
    { node: 'hexcore', service: 'scrutiny-web', title: 'Scrutiny UI stopped responding after image refresh', minutesAgo: 5 * DAY + 260, confidence: 'suspected' as const, mode: 'docker' as const },
    { node: 'atlas', service: 'caddy', title: 'Caddy recovered from a transient upstream mismatch', minutesAgo: 4 * DAY + 110, confidence: 'suspected' as const, mode: 'webhook' as const },
    { node: 'mikoshi', service: 'postgres', title: 'Primary postgres container restarted after backup handoff', minutesAgo: 3 * DAY + 300, confidence: 'confirmed' as const, mode: 'docker' as const },
    { node: 'hexcore', service: 'buildkitd', title: 'Buildkit daemon recycled after cache pressure spike', minutesAgo: 2 * DAY + 340, confidence: 'suspected' as const, mode: 'docker' as const },
    { node: 'mikoshi', service: 'nginx', title: 'Nginx hot reload briefly dropped upstream sessions', minutesAgo: DAY + 270, confidence: 'suspected' as const, mode: 'files' as const },
  ]

  for (const minor of minorIncidents) {
    let triggerId = ''
    let recoveryId = ''
    if (minor.mode === 'docker') {
      triggerId = pushEntry({
        timestamp: iso(nowMs, minor.minutesAgo + 9),
        node_name: minor.node,
        source: 'docker',
        service: minor.service,
        compose_service: minor.service,
        event: 'die',
        content: `${minor.service} stopped unexpectedly during routine maintenance`,
        metadata: dockerMetadata(minor.service, minor.node, 'minor-incident', {
          exit_code: 1,
        }),
      })
      recoveryId = pushEntry({
        timestamp: iso(nowMs, minor.minutesAgo),
        node_name: minor.node,
        source: 'docker',
        service: minor.service,
        compose_service: minor.service,
        event: 'start',
        content: `${minor.service} recovered after an automatic restart`,
        metadata: dockerMetadata(minor.service, minor.node, 'minor-incident', {
          restart_count: 1,
        }),
        correlated_id: triggerId,
      })
    } else if (minor.mode === 'webhook') {
      triggerId = pushEntry({
        timestamp: iso(nowMs, minor.minutesAgo + 8),
        node_name: minor.node,
        source: 'webhook',
        service: minor.service,
        event: 'down',
        content: `${minor.service} probe dropped below the success threshold`,
        metadata: {
          monitor: `${minor.service}-probe`,
          status: 'down',
          response_time_ms: 1700,
        },
      })
      recoveryId = pushEntry({
        timestamp: iso(nowMs, minor.minutesAgo),
        node_name: minor.node,
        source: 'webhook',
        service: minor.service,
        event: 'up',
        content: `${minor.service} probe recovered on its own`,
        metadata: {
          monitor: `${minor.service}-probe`,
          status: 'up',
          response_time_ms: 220,
        },
        correlated_id: triggerId,
      })
    } else {
      triggerId = pushEntry({
        timestamp: iso(nowMs, minor.minutesAgo + 7),
        node_name: minor.node,
        source: 'files',
        service: minor.service,
        event: 'modified',
        content: `edited /etc/nginx/conf.d/${minor.service}.conf during reload`,
        metadata: {
          path: `/etc/nginx/conf.d/${minor.service}.conf`,
          op: 'write',
          diff_status: 'included',
          diff_redacted: false,
          diff: '@@ -1,3 +1,3 @@\n-proxy_read_timeout 30s;\n+proxy_read_timeout 60s;',
        },
      })
      recoveryId = pushEntry({
        timestamp: iso(nowMs, minor.minutesAgo),
        node_name: minor.node,
        source: 'docker',
        service: 'nginx',
        compose_service: 'nginx',
        event: 'start',
        content: 'nginx finished reloading after config validation',
        metadata: dockerMetadata('nginx', minor.node, 'minor-incident'),
        correlated_id: triggerId,
      })
    }

    const incidentId = pushIncident({
      opened_at: iso(nowMs, minor.minutesAgo + 9),
      resolved_at: iso(nowMs, minor.minutesAgo),
      status: 'resolved',
      confidence: minor.confidence,
      title: minor.title,
      services: [minor.service],
      trigger_id: triggerId,
      node_names: [minor.node],
      metadata: {
        summary: 'Small, self-healing incident captured in the seeded homelab activity.',
      },
    })
    linkIncident(incidentId, triggerId, 'trigger', 70)
    linkIncident(incidentId, recoveryId, 'recovery', 64)
  }

  entries.sort((left, right) => {
    const timeDiff = Date.parse(right.timestamp) - Date.parse(left.timestamp)
    if (timeDiff !== 0) return timeDiff
    return right.id.localeCompare(left.id)
  })

  incidents.sort((left, right) => {
    const timeDiff = Date.parse(right.opened_at) - Date.parse(left.opened_at)
    if (timeDiff !== 0) return timeDiff
    return right.id.localeCompare(left.id)
  })

  const notificationDests: NotificationDest[] = [
    {
      id: 'notify-discord',
      name: 'Ops Discord',
      type: 'discord',
      url: 'https://discord.com/api/webhooks/demo/ops',
      events: ['incident_opened_confirmed', 'incident_resolved'],
      enabled: true,
      created_at: iso(nowMs, 90 * DAY),
      updated_at: iso(nowMs, 6 * 60),
    },
    {
      id: 'notify-ntfy',
      name: 'Ntfy phones',
      type: 'ntfy',
      url: 'https://ntfy.sh/blackbox-homelab',
      events: ['incident_opened_suspected', 'incident_resolved'],
      enabled: true,
      created_at: iso(nowMs, 60 * DAY),
      updated_at: iso(nowMs, 3 * 60),
    },
    {
      id: 'notify-slack',
      name: 'Infra Slack mirror',
      type: 'slack',
      url: 'https://hooks.slack.com/services/demo/infra',
      events: ['incident_opened_confirmed'],
      enabled: false,
      created_at: iso(nowMs, 45 * DAY),
      updated_at: iso(nowMs, 14 * 60),
    },
  ]

  const excludedTargets: ExcludedTarget[] = [
    { id: 'exclude-1', type: 'container', name: 'glances', created_at: iso(nowMs, 20 * DAY) },
    { id: 'exclude-2', type: 'stack', name: 'lab-observability', created_at: iso(nowMs, 14 * DAY) },
  ]

  const invites: InviteCode[] = [
    { id: 'invite-1', code: 'DEMO-ALPHA-73C2', used: false, created_at: iso(nowMs, 3 * DAY), expires_at: iso(nowMs, -4 * DAY) },
    { id: 'invite-2', code: 'DEMO-BETA-19F0', used: true, created_at: iso(nowMs, 9 * DAY), expires_at: iso(nowMs, 2 * DAY) },
    { id: 'invite-3', code: 'DEMO-GAMMA-8A51', used: false, created_at: iso(nowMs, DAY), expires_at: iso(nowMs, -6 * DAY) },
  ]

  const auditLogs: AuditLogEntry[] = [
    { id: 'audit-1', actor_user_id: 'user-admin', actor_email: 'operator@blackboxd.dev', action: 'notification.created', target_type: 'notification_destination', target_id: 'notify-discord', metadata: { name: 'Ops Discord' }, ip_address: '10.0.10.5', created_at: iso(nowMs, 7 * DAY) },
    { id: 'audit-2', actor_user_id: 'user-admin', actor_email: 'operator@blackboxd.dev', action: 'source.updated', target_type: 'data_source', target_id: 'source-filewatcher-mikoshi', metadata: { redact_secrets: true }, ip_address: '10.0.10.5', created_at: iso(nowMs, 6 * DAY + 140) },
    { id: 'audit-3', actor_user_id: 'user-emma', actor_email: 'emma@blackboxd.dev', action: 'invite.created', target_type: 'invite', target_id: 'invite-3', metadata: { expires_in_days: 7 }, ip_address: '10.0.10.41', created_at: iso(nowMs, 5 * DAY + 40) },
    { id: 'audit-4', actor_user_id: 'user-admin', actor_email: 'operator@blackboxd.dev', action: 'oidc.policy.updated', target_type: 'oidc_policy', target_id: 'invite_required', metadata: { previous: 'existing_only', next: 'invite_required' }, ip_address: '10.0.10.5', created_at: iso(nowMs, 4 * DAY + 80) },
    { id: 'audit-5', actor_user_id: 'user-jules', actor_email: 'jules@blackboxd.dev', action: 'user.force_logout', target_type: 'user', target_id: 'user-nia', metadata: { reason: 'token rotation drill' }, ip_address: '10.0.10.58', created_at: iso(nowMs, 4 * DAY + 140) },
    { id: 'audit-6', actor_user_id: 'user-admin', actor_email: 'operator@blackboxd.dev', action: 'mcp.token.regenerated', target_type: 'mcp', target_id: 'default', metadata: { suffix: '7KQ9' }, ip_address: '10.0.10.5', created_at: iso(nowMs, 3 * DAY + 120) },
    { id: 'audit-7', actor_user_id: 'user-emma', actor_email: 'emma@blackboxd.dev', action: 'notification.tested', target_type: 'notification_destination', target_id: 'notify-ntfy', metadata: { ok: true }, ip_address: '10.0.10.41', created_at: iso(nowMs, 2 * DAY + 60) },
    { id: 'audit-8', actor_user_id: 'user-admin', actor_email: 'operator@blackboxd.dev', action: 'source.created', target_type: 'data_source', target_id: 'source-systemd-atlas', metadata: { units: ['caddy.service', 'tailscaled.service'] }, ip_address: '10.0.10.5', created_at: iso(nowMs, DAY + 90) },
    { id: 'audit-9', actor_user_id: 'user-nia', actor_email: 'nia@blackboxd.dev', action: 'invite.revoked', target_type: 'invite', target_id: 'invite-2', metadata: { code: 'DEMO-BETA-19F0' }, ip_address: '10.0.10.77', created_at: iso(nowMs, 11 * 60) },
    { id: 'audit-10', actor_user_id: 'user-admin', actor_email: 'operator@blackboxd.dev', action: 'user.role.updated', target_type: 'user', target_id: 'user-jules', metadata: { is_admin: false }, ip_address: '10.0.10.5', created_at: iso(nowMs, 90) },
  ]

  const webhookDeliveries: WebhookDelivery[] = [
    { id: 'delivery-1', source: 'uptime_kuma', received_at: iso(nowMs, 3 * 60 + 20), payload_snippet: '{"monitor":"edge-homepage","status":"down","latency_ms":2500}', matched_incident_id: atlasIncident, status: 'matched', error_message: '' },
    { id: 'delivery-2', source: 'watchtower', received_at: iso(nowMs, 28 * 60 + 48), payload_snippet: '{"title":"Refreshing vaultwarden","level":"warn"}', matched_incident_id: watchtowerIncident, status: 'matched', error_message: '' },
    { id: 'delivery-3', source: 'uptime_kuma', received_at: iso(nowMs, 28 * 60 + 30), payload_snippet: '{"monitor":"vaultwarden-login","status":"down"}', matched_incident_id: watchtowerIncident, status: 'matched', error_message: '' },
    { id: 'delivery-4', source: 'uptime_kuma', received_at: iso(nowMs, 9 * DAY + 180), payload_snippet: '{"monitor":"media-external","status":"down"}', matched_incident_id: '', status: 'ignored', error_message: '' },
    { id: 'delivery-5', source: 'watchtower', received_at: iso(nowMs, 6 * DAY + 300), payload_snippet: '{"title":"Refreshing immich-server","level":"info"}', matched_incident_id: '', status: 'matched', error_message: '' },
    { id: 'delivery-6', source: 'watchtower', received_at: iso(nowMs, 45), payload_snippet: '{"title":"Refreshing oauth2-proxy","level":"info"}', matched_incident_id: '', status: 'error', error_message: 'delivery parsed but did not correlate to an incident' },
  ]

  const oidcProviders: OIDCProviderConfig[] = [
    {
      id: 'homelab',
      name: 'Homelab SSO',
      issuer: 'https://auth.blackboxd.dev/realms/homelab',
      client_id: 'blackbox-web',
      client_secret: 'configured',
      redirect_url: 'https://demo.blackboxd.dev/api/auth/oidc/homelab/callback',
      require_verified_email: true,
      enabled: false,
      created_at: iso(nowMs, 30 * DAY),
      updated_at: iso(nowMs, 8 * DAY),
    },
  ]

  const adminConfig: AdminConfig = {
    webhook_secret: 'demo-webhook-secret',
    file_watcher_redact_secrets: true,
    ai_provider: 'openai_compat',
    ai_url: 'https://api.openai.com/v1',
    ai_model: 'gpt-4o-mini',
    ai_api_key_set: true,
    ai_mode: 'analysis',
    mcp_enabled: true,
    mcp_port: 8765,
    mcp_auth_token_set: true,
    mcp_auth_token_suffix: '7KQ9',
    mcp_running: false,
    base_url: 'https://demo.blackboxd.dev',
  }

  // Recent activity within the default 6h view window
  const recentActivity = [
    { minsAgo: 8,   node: 'mikoshi', service: 'nginx',         event: 'start',   content: 'nginx started after routine config reload',                  metadata: { image: 'library/nginx:alpine', phase: 'reload' } },
    { minsAgo: 23,  node: 'atlas',   service: 'caddy',         event: 'pull',    content: 'pulled caddy:latest for the scheduled update window',        metadata: { image: 'library/caddy:latest', phase: 'prefetch' } },
    { minsAgo: 37,  node: 'hexcore', service: 'ollama',        event: 'start',   content: 'ollama started and entered healthcheck warmup',               metadata: { image: 'ollama/ollama:latest', phase: 'replace' } },
    { minsAgo: 51,  node: 'mikoshi', service: 'vaultwarden',   event: 'stop',    content: 'stopped previous vaultwarden instance after traffic handoff', metadata: { image: 'vaultwarden/server:latest', phase: 'handoff' } },
    { minsAgo: 72,  node: 'atlas',   service: 'crowdsec',      event: 'start',   content: 'crowdsec came back healthy after config refresh',             metadata: { image: 'crowdsecurity/crowdsec:latest', phase: 'refresh' } },
    { minsAgo: 95,  node: 'mikoshi', service: 'jellyfin',      event: 'pull',    content: 'pulled jellyfin:latest for the afternoon maintenance window', metadata: { image: 'jellyfin/jellyfin:latest', phase: 'prefetch' } },
    { minsAgo: 118, node: 'hexcore', service: 'paperless',     event: 'create',  content: 'created replacement paperless container on hexcore',          metadata: { image: 'ghcr.io/paperless-ngx/paperless-ngx:latest', phase: 'replace' } },
    { minsAgo: 140, node: 'mikoshi', service: 'gitea',         event: 'destroy', content: 'removed retired gitea container from the compose stack',      metadata: { image: 'gitea/gitea:latest', phase: 'handoff' } },
    { minsAgo: 175, node: 'atlas',   service: 'tailscale',     event: 'start',   content: 'tailscale started and entered healthcheck warmup',            metadata: { image: 'tailscale/tailscale:latest', phase: 'replace' } },
    { minsAgo: 210, node: 'hexcore', service: 'minio',         event: 'pull',    content: 'pulled minio:latest for the morning refresh window',          metadata: { image: 'minio/minio:latest', phase: 'prefetch' } },
    { minsAgo: 248, node: 'mikoshi', service: 'immich-server', event: 'start',   content: 'immich-server started after image update',                    metadata: { image: 'ghcr.io/immich-app/immich-server:release', phase: 'replace' } },
    { minsAgo: 290, node: 'atlas',   service: 'oauth2-proxy',  event: 'stop',    content: 'stopped previous oauth2-proxy instance after traffic handoff', metadata: { image: 'quay.io/oauth2-proxy/oauth2-proxy:latest', phase: 'handoff' } },
    { minsAgo: 315, node: 'mikoshi', service: 'redis',         event: 'create',  content: 'created replacement redis container on mikoshi',              metadata: { image: 'library/redis:alpine', phase: 'replace' } },
    { minsAgo: 342, node: 'hexcore', service: 'scrutiny-web',  event: 'start',   content: 'scrutiny-web came back healthy after config refresh',         metadata: { image: 'ghcr.io/analogj/scrutiny:master-web', phase: 'refresh' } },
  ]
  for (const r of recentActivity) {
    pushEntry({ timestamp: iso(nowMs, r.minsAgo), node_name: r.node, source: 'docker', service: r.service, compose_service: r.service, event: r.event, content: r.content, metadata: r.metadata })
  }

  const webhooks: WebhookEndpoint[] = [
    { id: 'webhook-uptime', method: 'POST', path: '/api/webhooks/uptime', source: 'uptime_kuma', description: 'Inbound downtime and recovery events from Uptime Kuma.' },
    { id: 'webhook-watchtower', method: 'POST', path: '/api/webhooks/watchtower', source: 'watchtower', description: 'Inbound image refresh notifications from Watchtower.' },
  ]

  return {
    dataCreatedAt: nowMs,
    setupStatus,
    healthStatus,
    sessionUser,
    nodes,
    users,
    entries,
    incidents,
    incidentLinks,
    auditLogs,
    webhookDeliveries,
    notificationDests,
    dataSourceInstances,
    sourceTypes,
    excludedTargets,
    oidcProviders,
    oidcPolicy: 'invite_required',
    invites,
    adminConfig,
    webhooks,
    sources,
  }
}

export const demoData = createDemoData()
