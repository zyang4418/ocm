Page({
  data: {
    statusBarHeight: 44,
    gridItems: [
      { name: '课程管理', icon: '/assets/icons/grid_course.png' },
      { name: '学生管理', icon: '/assets/icons/grid_student.png' },
      { name: '排课系统', icon: '/assets/icons/grid_schedule.png' },
      { name: '考勤统计', icon: '/assets/icons/grid_attendance.png' },
      { name: '教室监控', icon: '/assets/icons/grid_monitor.png' },
      { name: '资源中心', icon: '/assets/icons/grid_folder.png' },
      { name: '通知公告', icon: '/assets/icons/grid_notice.png' },
      { name: '帮助中心', icon: '/assets/icons/grid_help.png' },
      { name: '更多', icon: '/assets/icons/grid_more.png' },
      { name: '数据报表', icon: '/assets/icons/grid_report.png' }
    ],
    todayClasses: [
      { time: '08:00', end: '09:40', subject: '高等数学', room: '教学楼 A-301', teacher: '王老师' },
      { time: '10:00', end: '11:40', subject: '数据结构', room: '实验楼 B-205', teacher: '李老师' },
      { time: '14:00', end: '15:40', subject: '英语阅读', room: '教学楼 C-102', teacher: '张老师' }
    ]
  },

  onLoad() {
    const info = wx.getWindowInfo()
    this.setData({ statusBarHeight: info.statusBarHeight })
  },

  onTapAction(e: WechatMiniprogram.TouchEvent) {
    const { type } = e.currentTarget.dataset
    wx.showToast({ title: type, icon: 'none', duration: 1000 })
  },

  onTapGrid(e: WechatMiniprogram.TouchEvent) {
    const { name } = e.currentTarget.dataset
    wx.showToast({ title: name, icon: 'none', duration: 1000 })
  }
})
