import { bindAccount, silentLogin, getToken } from '../../utils/auth'

Page({
  data: {
    statusBarHeight: 44,
    username: '',
    password: '',
    showPassword: false,
    loading: false,
    // While true, a silent re-login is attempted on entry; the form is hidden
    // so a bound user never sees it flash.
    autoChecking: true,
  },

  async onLoad() {
    const info = wx.getWindowInfo()
    this.setData({ statusBarHeight: info.statusBarHeight })

    // Already holding a token -> straight into the app.
    if (getToken()) {
      wx.reLaunch({ url: '/pages/index/index' })
      return
    }
    // Bound user whose token was cleared (e.g. after a 401): silently re-log in
    // and return to the app without showing the form.
    try {
      await silentLogin()
      wx.reLaunch({ url: '/pages/index/index' })
    } catch (_e) {
      // Not bound (404) or network error: show the bind form.
      this.setData({ autoChecking: false })
    }
  },

  onUsernameInput(e: WechatMiniprogram.Input) {
    this.setData({ username: e.detail.value })
  },

  onPasswordInput(e: WechatMiniprogram.Input) {
    this.setData({ password: e.detail.value })
  },

  togglePassword() {
    this.setData({ showPassword: !this.data.showPassword })
  },

  async onSubmit() {
    if (this.data.loading) return
    const username = this.data.username.trim()
    const password = this.data.password
    if (!username) {
      wx.showToast({ title: '请输入账号', icon: 'none' })
      return
    }
    if (!password) {
      wx.showToast({ title: '请输入密码', icon: 'none' })
      return
    }

    this.setData({ loading: true })
    try {
      await bindAccount(username, password)
      wx.showToast({ title: '绑定成功', icon: 'success', duration: 600 })
      setTimeout(() => wx.reLaunch({ url: '/pages/index/index' }), 500)
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '绑定失败，请重试', icon: 'none', duration: 2000 })
    } finally {
      this.setData({ loading: false })
    }
  },
})
