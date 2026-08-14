import { ensureAuth, getUser } from '../../utils/auth'
import { getNavInfo } from '../../utils/nav'
import { can, canAny } from '../../utils/perms'
import { request } from '../../utils/request'
import { today, periodLabel } from '../../utils/format'

interface Booking {
  id: number
  classroomName: string
  date: string
  periodStart: number
  periodEnd: number
  purpose: string
  status: string
}

Page({
  data: {
    statusBarHeight: getNavInfo().statusBarHeight,
    // 快入口:url 非空且权限通过则跳转,否则保留 toast
    quickItems: [
      { type: '课表查询', icon: '/assets/icons/quick_calendar.png', url: '/pages/timetable/timetable', gate: 'course:read' },
      { type: '教室预约', icon: '/assets/icons/quick_door.png', url: '/pages/bookings/bookings', gate: 'booking' },
      { type: '设备控制', icon: '/assets/icons/quick_device.png', url: '', gate: '' },
      { type: '签到中心', icon: '/assets/icons/quick_check.png', url: '', gate: '' }
    ],
    // 功能网格:url 非空且权限通过则跳转(更多=控制台)
    gridItems: [
      { name: '课程管理', icon: '/assets/icons/grid_course.png', url: '/pages/courses/courses', gate: 'course:read' },
      { name: '学生管理', icon: '/assets/icons/grid_student.png', url: '/pages/admin-classes/admin-classes', gate: 'admin_class:read' },
      { name: '排课系统', icon: '/assets/icons/grid_schedule.png', url: '/pages/timetable/timetable', gate: 'course:read' },
      { name: '考勤统计', icon: '/assets/icons/grid_attendance.png', url: '', gate: '' },
      { name: '教室管理', icon: '/assets/icons/grid_monitor.png', url: '/pages/classrooms/classrooms', gate: 'classroom:read' },
      { name: '数据导入', icon: '/assets/icons/grid_folder.png', url: '/pages/imports/imports', gate: 'import' },
      { name: '审计日志', icon: '/assets/icons/grid_report.png', url: '/pages/logs/logs', gate: 'log:read' },
      { name: '帮助中心', icon: '/assets/icons/grid_help.png', url: '', gate: '' },
      { name: '更多', icon: '/assets/icons/grid_more.png', url: 'tab://pages/console/console', gate: '' },
      { name: '通知公告', icon: '/assets/icons/grid_notice.png', url: '', gate: '' }
    ],
    // 近期预约(替代原 mock 今日课程)
    showRecent: false,
    recentLoading: true,
    recent: [] as any[]
  },

  async onLoad() {
    await ensureAuth()
  },

  onShow() {
    this.syncTabBar()
    this.loadRecent()
  },

  /** 自定义 tabBar:每个 tab 页须同步当前选中项(按 pagePath 键,不受过滤后数量影响)。 */
  syncTabBar() {
    const tb = (this as any).getTabBar && (this as any).getTabBar()
    if (tb) tb.setData({ selected: '/pages/index/index' })
  },

  /** 我的近期预约:已通过且今天起;为空回退本人全部预约。 */
  async loadRecent() {
    const u = getUser()
    if (!u || !canAny(['classroom:read', 'classroom:book'])) {
      this.setData({ showRecent: false, recent: [], recentLoading: false })
      return
    }
    this.setData({ showRecent: true, recentLoading: true })
    try {
      let data = await request<{ items: Booking[] }>({
        path: '/api/bookings',
        params: { user_id: u.id, status: 'approved', from: today(), page_size: 5 }
      })
      if (!data || !(data.items || []).length) {
        data = await request<{ items: Booking[] }>({
          path: '/api/bookings',
          params: { user_id: u.id, from: today(), page_size: 5 }
        })
      }
      const view = ((data && data.items) || []).map((b: Booking) => ({
        ...b,
        periodText: periodLabel(b)
      }))
      this.setData({ recent: view, recentLoading: false })
    } catch {
      this.setData({ recent: [], recentLoading: false })
    }
  },

  onTapAction(e: WechatMiniprogram.TouchEvent) {
    const { type, url, gate } = e.currentTarget.dataset
    if (url && this.gateOk(gate)) {
      this.navTo(url)
      return
    }
    wx.showToast({ title: type, icon: 'none', duration: 1000 })
  },

  onTapGrid(e: WechatMiniprogram.TouchEvent) {
    const { name, url, gate } = e.currentTarget.dataset
    if (url && this.gateOk(gate)) {
      this.navTo(url)
      return
    }
    wx.showToast({ title: name, icon: 'none', duration: 1000 })
  },

  gateOk(gate: string): boolean {
    if (!gate) return true
    if (gate === 'booking') return canAny(['classroom:read', 'classroom:book'])
    if (gate === 'import') {
      return canAny(['course:manage', 'classroom:manage', 'admin_class:manage', 'teaching_class:manage', 'booking:approve'])
    }
    return can(gate)
  },

  navTo(url: string) {
    if (url.indexOf('tab://') === 0) {
      wx.switchTab({ url: url.slice(6) })
      return
    }
    wx.navigateTo({ url })
  },

  onTapScheduleMore() {
    if (this.gateOk('course:read')) {
      this.navTo('/pages/timetable/timetable')
    } else {
      wx.showToast({ title: '课表查询', icon: 'none', duration: 1000 })
    }
  }
})
