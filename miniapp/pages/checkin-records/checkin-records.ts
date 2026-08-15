import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { formatDateTime } from '../../utils/format'

interface MyCheckin {
  checkinId: number
  title: string
  courseName: string
  teachingClassName: string
  sessionText: string
  startsAt: string
  status: string
  checkedAt: string | null
}

const STATUS_META: Record<string, { text: string; theme: string }> = {
  present: { text: '出勤', theme: 'green' },
  late: { text: '迟到', theme: 'orange' },
  absent: { text: '缺勤', theme: 'red' },
  leave: { text: '请假', theme: 'gray' }
}

Page({
  data: {
    allowed: false,
    list: {
      items: [] as any[],
      total: 0,
      q: '',
      loading: true,
      error: '',
      hasMore: false
    }
  },

  async onLoad() {
    await ensureAuth()
    this.setData({ allowed: can('attendance:checkin') })
    // 只含「我有签到记录」的活动（不含未签到的推导），口径在页面说明。
    this._list = createPagedList({
      path: '/api/checkins/me',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((c: MyCheckin) => {
          const meta = STATUS_META[c.status] || { text: c.status, theme: 'gray' }
          return {
            ...c,
            statusText: meta.text,
            theme: meta.theme,
            startsAtText: formatDateTime(c.startsAt),
            checkedAtText: c.checkedAt ? formatDateTime(c.checkedAt) : ''
          }
        })
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
  },

  onReachBottom() {
    this._list.loadMore()
  }
})
