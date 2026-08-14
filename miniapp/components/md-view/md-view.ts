import { parseMarkdown, MdBlock } from '../../utils/markdown'

/** AI 回答的 Markdown 渲染组件(轻量子集,见 utils/markdown.ts)。 */
Component({
  properties: {
    content: { type: String, value: '' }
  },

  data: {
    blocks: [] as MdBlock[]
  },

  lifetimes: {
    attached() {
      this.refresh()
    }
  },

  observers: {
    // 流式输出期间 content 逐段增长,每段全量重解析(回答短,开销可忽略)
    content(v: string) {
      this.setData({ blocks: parseMarkdown(v) })
    }
  },

  methods: {
    refresh() {
      this.setData({ blocks: parseMarkdown(this.data.content) })
    },

    onTapLink(e: WechatMiniprogram.TouchEvent) {
      const { url } = e.currentTarget.dataset
      if (!url) return
      wx.setClipboardData({
        data: url,
        success: () => wx.showToast({ title: '链接已复制', icon: 'none', duration: 1200 })
      })
    }
  }
})
