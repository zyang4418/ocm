Page({
  onShow() {
    // 自定义 tabBar:每个 tab 页须同步当前选中项。
    const tb = (this as any).getTabBar && (this as any).getTabBar()
    if (tb) tb.setData({ selected: '/pages/messages/messages' })
  }
})
