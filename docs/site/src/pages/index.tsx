import clsx from 'clsx';
import Link from '@docusaurus/Link';
import Layout from '@theme/Layout';

import styles from './index.module.css';

const cards = [
  {
    title: 'Deployable from day one',
    body: 'The site is configured for Cloudflare Pages with docs/site as the project root and build as the publish directory.'
  },
  {
    title: 'Separated from app code',
    body: 'Documentation lives in its own Node project so docs changes stay isolated from the main web application build.'
  },
  {
    title: 'Ready for expansion',
    body: 'Add server, agent, and demo sections later without restructuring the scaffold.'
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
          <span className={styles.eyebrow}>Cloudflare-ready scaffold</span>
          <h1 className={styles.title}>Blackbox Docs</h1>
          <p className={styles.subtitle}>
            A standalone Docusaurus site for publishing Blackbox documentation
            at <code>docs.blackboxd.dev</code>.
          </p>
          <div className={styles.actions}>
            <Link className="button button--primary button--lg" to="/docs/intro">
              Open docs
            </Link>
            <Link
              className={clsx(
                'button button--outline button--lg',
                styles.secondaryButton
              )}
              to="/docs/intro"
            >
              View starter page
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
