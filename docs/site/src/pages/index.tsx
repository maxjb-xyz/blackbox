import clsx from 'clsx';
import Link from '@docusaurus/Link';
import Layout from '@theme/Layout';

import styles from './index.module.css';

const quickLinks = [
  {
    title: 'Start Here',
    description:
      'Get a single-node install running, complete first-time setup, and understand the core Blackbox model.',
    links: [
      {label: 'Quick Start', to: '/docs/getting-started/quick-start'},
      {label: 'First Login And Setup', to: '/docs/getting-started/first-login-and-setup'},
      {label: 'How Blackbox Thinks', to: '/docs/getting-started/how-blackbox-thinks'}
    ]
  },
  {
    title: 'Deploy And Operate',
    description:
      'Run one central server, scale out to multiple nodes, and keep data sources, persistence, and troubleshooting clear.',
    links: [
      {label: 'Multi-Node Deployment', to: '/docs/deployment/multi-node'},
      {label: 'Data Sources Overview', to: '/docs/data-sources/overview'},
      {label: 'Troubleshooting Agents And Nodes', to: '/docs/operations/troubleshooting-agents-and-nodes'}
    ]
  },
  {
    title: 'Build And Contribute',
    description:
      'Set up a development environment, understand the repo layout, and add new source types without guessing.',
    links: [
      {label: 'Contributing Overview', to: '/docs/contributing/overview'},
      {label: 'Development Setup', to: '/docs/contributing/development-setup'},
      {label: 'Adding A Data Source', to: '/docs/contributing/adding-a-data-source'}
    ]
  }
];

const coverage = [
  'Docker, file watcher, systemd, and webhook data sources',
  'Timeline entries, incidents, correlation, and optional AI analysis',
  'OIDC, notifications, MCP, API docs, and operational troubleshooting'
];

export default function Home() {
  return (
    <Layout
      title="Blackbox Docs"
      description="Deployment, data source, operations, and contributor guidance for Blackbox"
    >
      <header className={clsx('hero hero--primary', styles.heroBanner)}>
        <div className={styles.container}>
          <span className={styles.eyebrow}>Blackbox documentation hub</span>
          <h1 className={styles.title}>Blackbox Docs</h1>
          <p className={styles.subtitle}>
            The operational and contributor guide for deploying Blackbox,
            connecting data sources, understanding incidents, and extending the
            platform safely.
          </p>
          <div className={styles.actions}>
            <Link
              className="button button--primary button--lg"
              to="/docs/getting-started/quick-start"
            >
              Start with quick start
            </Link>
            <Link
              className={clsx(
                'button button--outline button--lg',
                styles.secondaryButton
              )}
              to="/docs/deployment/multi-node"
            >
              Plan a multi-node deploy
            </Link>
          </div>
        </div>
      </header>

      <main>
        <section className={styles.section}>
          <div className={styles.container}>
            <div className={styles.coverageBlock}>
              <p className={styles.sectionEyebrow}>What these docs cover</p>
              <ul className={styles.coverageList}>
                {coverage.map((item) => (
                  <li key={item} className={styles.coverageItem}>
                    {item}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </section>

        <section className={styles.section}>
          <div className={styles.container}>
            <div className={styles.sectionHeader}>
              <p className={styles.sectionEyebrow}>Start paths</p>
              <h2 className={styles.sectionTitle}>Choose the job you need to do</h2>
              <p className={styles.sectionText}>
                The docs are organized around common operator and contributor
                tasks instead of mirroring the repository one-to-one.
              </p>
            </div>

            <div className={styles.cardGrid}>
              {quickLinks.map((group) => (
                <article key={group.title} className={styles.card}>
                  <h3 className={styles.cardTitle}>{group.title}</h3>
                  <p className={styles.cardBody}>{group.description}</p>
                  <div className={styles.linkList}>
                    {group.links.map((link) => (
                      <Link key={link.to} className={styles.inlineLink} to={link.to}>
                        {link.label}
                      </Link>
                    ))}
                  </div>
                </article>
              ))}
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}
