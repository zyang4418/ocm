import { getUser, unbind, AppUser } from '../../utils/auth'
import { getNavInfo } from '../../utils/nav'
import { can } from '../../utils/perms'
import { request } from '../../utils/request'

// roleLabelOf renders the user's roles joined by " / ", falling back to the
// account type when no roles are present.
function roleLabelOf(u: AppUser): string {
  const roles = u.roles || []
  if (roles.length > 0) return roles.map((r) => r.name).join(' / ')
  return u.type === 'student' ? '学生' : u.type === 'teacher' ? '教师' : '职员'
}

Page({
  data: {
    statusBarHeight: getNavInfo().statusBarHeight,
    cacheSize: '12.5 MB',
    displayName: '未登录',
    roleLabel: '',
    username: '',
    stats: [] as { value: string; label: string }[],
    showStats: false,
    menuGroup1: [
      { name: '账号与安全', icon: '/assets/icons/profile_account.png', url: '' },
      { name: '通知设置', icon: '/assets/icons/profile_notification.png', url: '' }
    ],
    menuGroup2: [
      { name: '帮助中心', icon: '/assets/icons/profile_help.png', url: '' },
      { name: '隐私政策', icon: '/assets/icons/profile_privacy.png', url: '' },
      { name: '关于我们', icon: '/assets/icons/profile_about.png', url: '' }
    ],
    settingsVisible: false
  },

  onShow() {
    const u = getUser()
    if (u) {
      this.setData({
        displayName: u.displayName || u.username,
        username: u.username,
        roleLabel: roleLabelOf(u)
      })
    } else {
      this.setData({ displayName: '未登录', roleLabel: '', username: '' })
    }
    // 授权变更后菜单与统计随之刷新。
    this.setData({ settingsVisible: can('*') })
    this.loadStats()
  },

  /** 权限门控实时统计(page_size=1 取 total);失败静默置「-」。 */
  async loadStats() {
    const u = getUser()
    if (!u) {
      this.setData({ stats: [], showStats: false })
      return
    }
    const defs: { label: string; path: string; params: Record<string, string | number>; gate: boolean }[] = [
      { label: '我的预约', path: '/api/bookings', params: { user_id: u.id, page_size: 1 }, gate: can('classroom:read') },
      { label: '待审批', path: '/api/bookings', params: { status: 'pending', page_size: 1 }, gate: can('booking:approve') },
      { label: '教室数', path: '/api/classrooms', params: { page_size: 1 }, gate: can('classroom:read') },
      { label: '用户数', path: '/api/users', params: { page_size: 1 }, gate: can('user:read') }
    ]
    const active = defs.filter((d) => d.gate)
    if (!active.length) {
      this.setData({ stats: [], showStats: false })
      return
    }
    this.setData({ showStats: true })
    const stats = await Promise.all(
      active.map(async (d) => {
        try {
          const data = await request<{ total: number }>({ path: d.path, params: d.params })
          return { value: String(data && data.total != null ? data.total : '-'), label: d.label }
        } catch {
          return { value: '-', label: d.label }
        }
      })
    )
    this.setData({ stats })
  },

  onTapAction(e: WechatMiniprogram.TouchEvent) {
    const { type, label } = e.currentTarget.dataset
    wx.showToast({ title: type || label, icon: 'none', duration: 1000 })
  },

  onTapMenu(e: WechatMiniprogram.TouchEvent) {
    const { name, url } = e.currentTarget.dataset
    if (name === '清除缓存') {
      wx.showModal({
        title: '清除缓存',
        content: '确定要清除本地缓存吗？',
        confirmColor: '#2B5FF6',
        success: (res) => {
          if (res.confirm) {
            this.setData({ cacheSize: '0 B' })
            wx.showToast({ title: '清除成功', icon: 'success', duration: 1200 })
          }
        }
      })
      return
    }
    if (url) {
      wx.navigateTo({ url })
      return
    }
    wx.showToast({ title: name, icon: 'none', duration: 1000 })
  },

  // In the bind model the only meaningful "exit" is unbinding: it severs the
  // WeChat<->account link so the next entry requires credentials again.
  onLogout() {
    wx.showModal({
      title: '解绑并退出',
      content: '解绑后将解除当前微信号与账号的关联，下次进入需重新输入账号密码。',
      confirmText: '解绑',
      confirmColor: '#FF7A45',
      success: async (res) => {
        if (!res.confirm) return
        wx.showLoading({ title: '解绑中', mask: true })
        try {
          await unbind()
        } catch (_e) {
          // Even if the unbind request fails (e.g. network), clear locally and
          // return to the login page so the user can re-bind.
        }
        wx.hideLoading()
        wx.reLaunch({ url: '/pages/login/login' })
      }
    })
  }
})
