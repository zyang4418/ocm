import { getUser, unbind } from '../../utils/auth'
import { getNavInfo } from '../../utils/nav'

Page({
  data: {
    statusBarHeight: getNavInfo().statusBarHeight,
    cacheSize: '12.5 MB',
    displayName: '未登录',
    roleLabel: '',
    username: '',
    stats: [
      { value: '18', label: '本周课时' },
      { value: '4', label: '管理班级' },
      { value: '156', label: '学生人数' }
    ],
    menuGroup1: [
      { name: '账号与安全', icon: '/assets/icons/profile_account.png' },
      { name: '通知设置', icon: '/assets/icons/profile_notification.png' }
    ],
    menuGroup2: [
      { name: '帮助中心', icon: '/assets/icons/profile_help.png' },
      { name: '隐私政策', icon: '/assets/icons/profile_privacy.png' },
      { name: '关于我们', icon: '/assets/icons/profile_about.png' }
    ]
  },

  onShow() {
    const u = getUser()
    if (u) {
      this.setData({
        displayName: u.displayName || u.username,
        username: u.username,
        roleLabel: u.role === 'admin' ? '管理员' : '教师'
      })
    } else {
      this.setData({ displayName: '未登录', roleLabel: '', username: '' })
    }
  },

  onTapAction(e: WechatMiniprogram.TouchEvent) {
    const { type, label } = e.currentTarget.dataset
    wx.showToast({ title: type || label, icon: 'none', duration: 1000 })
  },

  onTapMenu(e: WechatMiniprogram.TouchEvent) {
    const { name } = e.currentTarget.dataset
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
