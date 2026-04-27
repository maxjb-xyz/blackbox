import clsx from 'clsx';
import Link from '@docusaurus/Link';
import Layout from '@theme/Layout';

import styles from './index.module.css';

const cards = [
  {
    title: 'Quick Start',
    body: 'Get a single-node Blackbox install running in minutes with the shortest path from compose file to first timeline events.'
  },
  {
    title: 'Multi-Node Deployment',
    body: 'Run one central server with multiple agents and keep node tokens, capabilities, and source setup understandable.'
  },
  {
    title: 'Contributing',
    body: 'Follow the contributor path for development setup, project structure, testing, and adding new data sources.'
  }
];

export default function Home() {
  return (
    <Layout
      title="Blackbox Docs"
      description="Standalone documentation site for Blackbox"
    >
      <header className={clsx('hero hero--primary', styles.heroBanner)}>
        <div className={styles.container}>
          <span className={styles.eyebrow}>Operator docs for Blackbox</span>
          <h1 className={styles.title}>Blackbox Docs</h1>
          <p className={styles.subtitle}>
            Deployment, data source, operations, and contributor guidance for
            running Blackbox across one node or many.
          </p>
          <div className={styles.actions}>
            <Link
              className="button button--primary button--lg"
              to="/docs/getting-started/quick-start"
            >
              Open quick start
            </Link>
            <Link
              className={clsx(
                'button button--outline button--lg',
                styles.secondaryButton
              )}
              to="/docs/intro"
            >
              Browse all docs
            </Link>
          </div>
        </div>
      </header>

      <main className={styles.section}>
        <div className={styles.container}>
          <div className={styles.cardGrid}>
            {cards.map((card) => (
              <article key={card.title} className={styles.card}>
                <h2 className={styles.cardTitle}>{card.title}</h2>
                <p className={styles.cardBody}>{card.body}</p>
              </article>
            ))}
          </div>
        </div>
      </main>
    </Layout>
  );
}
