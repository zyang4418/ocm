Page({
  data: {
    statusBarHeight: 44,
    cacheSize: '12.5 MB',
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

  onLoad() {
    const info = wx.getWindowInfo()
    this.setData({ statusBarHeight: info.statusBarHeight })
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

  onLogout() {
    wx.showModal({
      title: '退出登录',
      content: '确定要退出当前账号吗？',
      confirmColor: '#FF7A45',
      success: (res) => {
        if (res.confirm) {
          wx.showToast({ title: '已退出登录', icon: 'none', duration: 1200 })
        }
      }
    })
  }
})
