const { apiConfig } = require('./config/api')
const { initAuth } = require('./utils/auth')

App({
  globalData: {
    user: null,
  },

  onLaunch() {
    this.initCloud()
    initAuth()
  },

  // callContainer requires wx.cloud.init to be called once (env may be empty;
  // the real Cloud Run env is supplied per-call via config.env). In 'http' mode
  // (self-hosted) cloud is unused, so skip it.
  initCloud() {
    if (apiConfig.transport !== 'callContainer') return
    if (!wx.cloud || typeof wx.cloud.init !== 'function') {
      console.warn('[cloud] wx.cloud 不可用，无法使用 callContainer')
      return
    }
    try {
      wx.cloud.init()
    } catch (e) {
      console.warn('[cloud] init 失败', e)
    }
  },
})
