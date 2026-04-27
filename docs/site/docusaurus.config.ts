import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'Blackbox Docs',
  tagline: 'Documentation for the Blackbox project',
  url: 'https://docs.blackboxd.dev',
  baseUrl: '/',
  onBrokenLinks: 'throw',
  organizationName: 'blackboxd',
  projectName: 'blackbox',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn'
    }
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts'
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css'
        }
      } satisfies Preset.Options
    ]
  ],

  themeConfig: {
    navbar: {
      title: 'Blackbox Docs',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'tutorialSidebar',
          position: 'left',
          label: 'Docs'
        }
      ]
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {
              label: 'Introduction',
              to: '/docs/intro'
            }
          ]
        }
      ],
      copyright: `Copyright ${new Date().getFullYear()} Blackbox`
    },
    colorMode: {
      defaultMode: 'dark',
      disableSwitch: false,
      respectPrefersColorScheme: false
    }
  } satisfies Preset.ThemeConfig
};

export default config;
