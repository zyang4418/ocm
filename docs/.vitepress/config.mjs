import { defineConfig } from 'vitepress'

export default defineConfig({
  lang: 'zh-CN',
  title: 'OCM',
  description: 'OCM 智慧教室管理系统文档',

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: '指南', link: '/guide/getting-started' },
      { text: 'API', link: '/api/' },
    ],

    sidebar: {
      '/guide/': [
        {
          text: '开始',
          items: [
            { text: '快速开始', link: '/guide/getting-started' },
            { text: '项目结构', link: '/guide/structure' },
          ],
        },
        {
          text: '使用',
          items: [
            { text: '课表导入', link: '/guide/import' },
          ],
        },
        {
          text: '开发',
          items: [
            { text: '小程序', link: '/guide/miniapp' },
            { text: 'Web 端', link: '/guide/web' },
            { text: '后端', link: '/guide/backend' },
          ],
        },
      ],
      '/api/': [
        {
          text: 'API 文档',
          items: [
            { text: '概述', link: '/api/' },
          ],
        },
      ],
    },

    socialLinks: [],

    footer: {},

    search: {
      provider: 'local',
    },

    docFooter: {
      prev: '上一页',
      next: '下一页',
    },

    outline: {
      label: '页面导航',
    },

    lastUpdated: {
      text: '最后更新于',
    },
  },
})
