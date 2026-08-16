import { getNavInfo } from '../../utils/nav'
import { getUser } from '../../utils/auth'
import { can, canAny } from '../../utils/perms'

interface HubEntry {
  name: string
  icon: string
  url: string
  gate: () => boolean
  /** Page shipped yet? Flipped true as each page lands (phased rollout). */
  ready: boolean
}

interface HubSection {
  title: string
  entries: { name: string; icon: string; url: string }[]
}

// All management entries with their permission gates, mirroring the web
// console's nav visibility. `ready` gates pages that exist in a later phase
// so the hub never links to a page the build doesn't contain yet.
const ALL_ENTRIES: (HubEntry & { group: string })[] = [
  // 教室与排课
  { group: 'classroom', name: '教室预约', icon: '/assets/icons/quick_door.png', url: '/pages/bookings/bookings', gate: () => canAny(['classroom:read', 'classroom:book']), ready: true },
  { group: 'classroom', name: '教室管理', icon: '/assets/icons/grid_monitor.png', url: '/pages/classrooms/classrooms', gate: () => can('classroom:read'), ready: true },
  { group: 'classroom', name: '课程管理', icon: '/assets/icons/grid_course.png', url: '/pages/courses/courses', gate: () => can('course:read'), ready: true },
  { group: 'classroom', name: '作息设置', icon: '/assets/icons/ai_schedule.png', url: '/pages/schedule/schedule', gate: () => can('course:read'), ready: true },
  { group: 'classroom', name: '教室课表', icon: '/assets/icons/quick_calendar.png', url: '/pages/timetable/timetable', gate: () => can('course:read'), ready: true },
  { group: 'classroom', name: '听课评课', icon: '/assets/icons/grid_report.png', url: '/pages/observations/observations', gate: () => canAny(['observation:read', 'observation:write', 'observation:manage']), ready: true },
  { group: 'classroom', name: '教室报修', icon: '/assets/icons/quick_device.png', url: '/pages/repairs/repairs', gate: () => canAny(['repair:create', 'repair:assign']), ready: true },
  // 组织与权限
  { group: 'org', name: '用户管理', icon: '/assets/icons/grid_student.png', url: '/pages/users/users', gate: () => can('user:read'), ready: true },
  { group: 'org', name: '角色管理', icon: '/assets/icons/profile_account.png', url: '/pages/roles/roles', gate: () => can('role:read'), ready: true },
  { group: 'org', name: '用户组管理', icon: '/assets/icons/grid_notice.png', url: '/pages/groups/groups', gate: () => can('group:read'), ready: true },
  { group: 'org', name: '行政班管理', icon: '/assets/icons/ai_attendance.png', url: '/pages/admin-classes/admin-classes', gate: () => can('admin_class:read'), ready: true },
  { group: 'org', name: '教学班管理', icon: '/assets/icons/ai_lesson.png', url: '/pages/teaching-classes/teaching-classes', gate: () => can('teaching_class:read'), ready: true },
  // 系统
  { group: 'system', name: '数据导入', icon: '/assets/icons/grid_folder.png', url: '/pages/imports/imports', gate: () => canAny(['course:manage', 'classroom:manage', 'admin_class:manage', 'teaching_class:manage', 'booking:approve']), ready: true },
  { group: 'system', name: '审计日志', icon: '/assets/icons/grid_report.png', url: '/pages/logs/logs', gate: () => can('log:read'), ready: true },
  { group: 'system', name: '参数配置', icon: '/assets/icons/profile_settings.png', url: '/pages/settings/settings', gate: () => can('*'), ready: true }
]

const SECTION_ORDER: { group: string; title: string }[] = [
  { group: 'classroom', title: '教室与排课' },
  { group: 'org', title: '组织与权限' },
  { group: 'system', title: '系统' }
]

Page({
  data: {
    statusBarHeight: getNavInfo().statusBarHeight,
    sections: [] as HubSection[],
    hasAny: false
  },

  onShow() {
    this.syncTabBar()
    // Re-read the cached user on every show so permission changes made in
    // this session (e.g. via the users page) reflect without a restart.
    const user = getUser()
    const sections: HubSection[] = []
    for (const { group, title } of SECTION_ORDER) {
      const entries = ALL_ENTRIES.filter((e) => e.group === group && e.ready && e.gate())
        .map(({ name, icon, url }) => ({ name, icon, url }))
      if (entries.length) sections.push({ title, entries })
    }
    this.setData({ sections, hasAny: sections.length > 0 })
  },

  onTapEntry(e: WechatMiniprogram.TouchEvent) {
    const { url } = e.currentTarget.dataset
    wx.navigateTo({ url })
  },

  /** 自定义 tabBar:每个 tab 页须同步当前选中项。 */
  syncTabBar() {
    const tb = (this as any).getTabBar && (this as any).getTabBar()
    if (tb) tb.setData({ selected: '/pages/console/console' })
  }
})
