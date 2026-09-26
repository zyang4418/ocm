/**
 * Cached window metrics for custom navigation bars.
 *
 * Every page with a custom nav bar needs statusBarHeight (and ai.ts also needs
 * safeAreaBottom/pageHeight). Calling wx.getWindowInfo in each page's onLoad
 * both duplicates the fetch and (in ai.ts's case) relied on the deprecated
 * getSystemInfoSync. This module reads once and caches (CommonJS singleton), so
 * data initializers can call it synchronously at first-frame render.
 */
interface NavInfo {
  statusBarHeight: number
  safeAreaBottom: number
  pageHeight: number
}

let cached: NavInfo | null = null

export function getNavInfo(): NavInfo {
  if (cached) return cached
  const info = wx.getWindowInfo()
  const sbh = info.statusBarHeight || 44
  cached = {
    statusBarHeight: sbh,
    safeAreaBottom: info.safeArea ? info.screenHeight - info.safeArea.bottom : 0,
    // custom 导航下 windowHeight 即全屏可用高（已含状态栏），可直接作页面根总高。
    pageHeight: info.windowHeight,
  }
  return cached
}
