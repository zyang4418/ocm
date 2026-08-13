import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

// 各文档标题取自其 frontmatter title。
const sidebars: SidebarsConfig = {
  main: [
    {
      type: 'category',
      label: '开始',
      items: ['guide/getting-started', 'guide/structure'],
    },
    {
      type: 'category',
      label: '使用',
      items: ['guide/import'],
    },
    {
      type: 'category',
      label: '开发',
      items: ['guide/miniapp', 'guide/web', 'guide/backend'],
    },
    {
      type: 'category',
      label: 'API',
      items: ['api/index'],
    },
  ],
};

export default sidebars;
