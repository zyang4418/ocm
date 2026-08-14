import { can } from '../utils/perms'

interface TabDef {
  pagePath: string
  text: string
  iconPath: string
  selectedIconPath: string
  gate: () => boolean
}

// Mirrors app.json tabBar.list. Native tab bars are static, so the AI tab is
// filtered here: users without `ai:chat` must not even see the button —
// letting them open the page and then blocking them is not acceptable.
const TAB_DEFS: TabDef[] = [
  { pagePath: '/pages/index/index', text: '首页', iconPath: '/assets/tabbar/home.png', selectedIconPath: '/assets/tabbar/home_active.png', gate: () => true },
  { pagePath: '/pages/console/console', text: '控制台', iconPath: '/assets/tabbar/console.png', selectedIconPath: '/assets/tabbar/console_active.png', gate: () => true },
  { pagePath: '/pages/ai/ai', text: 'AI助手', iconPath: '/assets/tabbar/ai.png', selectedIconPath: '/assets/tabbar/ai_active.png', gate: () => can('ai:chat') },
  { pagePath: '/pages/messages/messages', text: '消息', iconPath: '/assets/tabbar/messages.png', selectedIconPath: '/assets/tabbar/messages_active.png', gate: () => true },
  { pagePath: '/pages/profile/profile', text: '我的', iconPath: '/assets/tabbar/profile.png', selectedIconPath: '/assets/tabbar/profile_active.png', gate: () => true }
]

Component({
  data: {
    selected: '',
    items: [] as TabDef[]
  },

  lifetimes: {
    // setData in attached lands before the first render, so gated items never
    // flash for users without permission.
    attached() {
      this.refresh()
    }
  },

  pageLifetimes: {
    // Each tab page hosts its own tab-bar instance; recompute on every show so
    // permission changes made in this session reflect without a restart.
    show() {
      this.refresh()
    }
  },

  methods: {
    refresh() {
      this.setData({ items: TAB_DEFS.filter((t) => t.gate()) })
    },

    onTap(e: WechatMiniprogram.TouchEvent) {
      const { path } = e.currentTarget.dataset
      if (!path || path === this.data.selected) return
      this.setData({ selected: path })
      wx.switchTab({ url: path })
    }
  }
})
