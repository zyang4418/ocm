import type {Config} from '@docusaurus/types';

const config: Config = {
  title: 'OCM',
  tagline: 'OCM is a open-source classroom management system.',
  url: 'https://docs.zyang4418.cn',
  baseUrl: '/',
  trailingSlash: false,

  onBrokenLinks: 'throw',

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  i18n: {
    defaultLocale: 'zh-CN',
    locales: ['zh-CN'],
  },

  presets: [
    [
      'classic',
      {
        // docs-only 模式:根 URL '/' 直接渲染文档落地页(docs/docs/index.md),无首页、无重定向。
        docs: {
          routeBasePath: '/',
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/zyang4418/ocm/edit/main/docs/docs/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      },
    ],
  ],

  themes: [
    [
      '@easyops-cn/docusaurus-search-local',
      {
        language: ['zh', 'en'],
        hashed: true,
        indexBlog: false,
      },
    ],
  ],

  themeConfig: {
    colorMode: {
      defaultMode: 'light',
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'OCM',
      items: [
        {href: 'https://github.com/zyang4418/ocm', label: 'GitHub', position: 'right'},
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: '文档',
          items: [
            {label: '快速开始', to: '/'},
            {label: '课表导入', to: '/guide/import'},
            {label: 'API', to: '/api'},
          ],
        },
        {
          title: '更多',
          items: [
            {label: 'GitHub', href: 'https://github.com/zyang4418/ocm'},
          ],
        },
      ],
      copyright: 'Copyright © 2026 zyang4418/ocm. Built with Docusaurus.',
    },
  },
};

export default config;
