// 同步获取系统信息，确保首帧渲染时拿到正确高度（避免 100vh 在部分机型初始计算偏差）
const sysInfo = wx.getSystemInfoSync()

Page({
  data: {
    statusBarHeight: sysInfo.statusBarHeight,
    safeAreaBottom: sysInfo.screenHeight - sysInfo.safeArea.bottom,
    pageHeight: sysInfo.windowHeight + sysInfo.statusBarHeight,
    inputValue: '',
    suggestions: [
      { text: '帮我查一下本周高等数学的出勤率' },
      { text: '生成一份数据结构实验课的教案' },
      { text: '下周三下午有哪些空闲教室可用？' },
      { text: '统计一下本月迟到超过 3 次的学生名单' }
    ]
  },

  onTapSuggest(e: WechatMiniprogram.TouchEvent) {
    const { text } = e.currentTarget.dataset
    this.setData({ inputValue: text })
    wx.showToast({ title: '已填入输入框', icon: 'none', duration: 800 })
  },

  onInput(e: WechatMiniprogram.Input) {
    this.setData({ inputValue: e.detail.value })
  },

  onSend() {
    const { inputValue } = this.data
    if (!inputValue.trim()) {
      wx.showToast({ title: '请输入问题', icon: 'none', duration: 1000 })
      return
    }
    wx.showToast({ title: 'AI 思考中…', icon: 'none', duration: 1200 })
    this.setData({ inputValue: '' })
  }
})
