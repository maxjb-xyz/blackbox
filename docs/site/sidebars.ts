import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  tutorialSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/overview',
        'getting-started/quick-start',
        'getting-started/first-login-and-setup',
        'getting-started/how-blackbox-thinks'
      ]
    },
    {
      type: 'category',
      label: 'Deployment',
      items: [
        'deployment/architecture',
        'deployment/single-node',
        'deployment/multi-node',
        'deployment/reverse-proxy-and-tls',
        'deployment/persistent-data',
        'deployment/upgrading'
      ]
    },
    {
      type: 'category',
      label: 'Configuration',
      items: [
        'configuration/server-environment',
        'configuration/agent-environment',
        'configuration/agent-tokens',
        'configuration/timezones-and-logging'
      ]
    },
    {
      type: 'category',
      label: 'Data Sources',
      items: [
        'data-sources/overview',
        'data-sources/docker',
        'data-sources/file-watcher',
        'data-sources/systemd',
        'data-sources/uptime-kuma',
        'data-sources/watchtower',
        'data-sources/node-capabilities'
      ]
    },
    {
      type: 'category',
      label: 'Incidents And Timeline',
      items: [
        'incidents-and-timeline/timeline',
        'incidents-and-timeline/incidents',
        'incidents-and-timeline/correlation-model',
        'incidents-and-timeline/incident-ai-analysis',
        'incidents-and-timeline/pdf-reports'
      ]
    },
    {
      type: 'category',
      label: 'Integrations',
      items: [
        'integrations/notifications',
        'integrations/oidc-sso',
        'integrations/mcp-server',
        'integrations/api-reference'
      ]
    },
    {
      type: 'category',
      label: 'Operations',
      items: [
        'operations/authentication',
        'operations/user-registration-and-invites',
        'operations/nodes-and-heartbeats',
        'operations/admin-observability',
        'operations/troubleshooting-file-watcher',
        'operations/troubleshooting-systemd',
        'operations/troubleshooting-agents-and-nodes',
        'operations/security-model'
      ]
    },
    {
      type: 'category',
      label: 'Contributing',
      items: [
        'contributing/overview',
        'contributing/development-setup',
        'contributing/project-structure',
        'contributing/testing',
        'contributing/adding-a-data-source',
        'contributing/documenting-a-feature',
        'contributing/code-style-and-principles'
      ]
    }
  ]
};

export default sidebars;
