import { bindAccount, silentLogin, getToken } from '../../utils/auth'
import { getNavInfo } from '../../utils/nav'

Page({
  data: {
    statusBarHeight: getNavInfo().statusBarHeight,
    username: '',
    password: '',
    showPassword: false,
    loading: false,
    // While true, a silent re-login is attempted on entry; the form is hidden
    // so a bound user never sees it flash.
    autoChecking: true,
  },

  async onLoad(options?: { notBound?: string }) {
    // Already holding a token -> straight into the app.
    if (getToken()) {
      wx.reLaunch({ url: '/pages/index/index' })
      return
    }
    // 从 index 的 ensureAuth 经 404 过来:openid 已知未绑定,跳过冗余 probe
    // (index 刚试过)直接出表单。
    if (options && options.notBound) {
      this.setData({ autoChecking: false })
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
      // 成功:loading 保持 true 直到 reLaunch 销毁本页,杜绝双击重提
      wx.showToast({ title: '绑定成功', icon: 'success', duration: 1200 })
      setTimeout(() => wx.reLaunch({ url: '/pages/index/index' }), 1200)
    } catch (err: any) {
      this.setData({ loading: false })
      wx.showToast({ title: (err && err.message) || '绑定失败，请重试', icon: 'none', duration: 2000 })
    }
  },
})
