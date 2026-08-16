import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { request } from '../../utils/request'

interface ScanResult {
  checkinId: number
  title: string
  status: string
  isNew: boolean
  inRoster: boolean
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
    code: '',
    submitting: false,
    error: '',
    result: null as any
  },

  async onLoad() {
    await ensureAuth()
    // 仅学生角色（attendance:checkin 权限）可签到；无权限渲染占位。
    this.setData({ allowed: can('attendance:checkin') })
  },

  /** 数字码输入：仅保留数字，最多 6 位。 */
  onCodeInput(e: WechatMiniprogram.Input) {
    this.setData({ code: e.detail.value.replace(/\D/g, '').slice(0, 6), error: '' })
  },

  onTapScan() {
    wx.scanCode({
      scanType: ['qrCode', 'barCode'],
      success: (res) => {
        const code = this.parseCode(res.result)
        if (!code) {
          this.setData({ error: '二维码无效' })
          return
        }
        this.submitCode(code)
      }
    })
  },

  /** 从扫码内容提取 6 位数字：兼容纯数字与 URL 中 code= 参数两种格式。 */
  parseCode(raw: string): string {
    const s = (raw || '').trim()
    const m = s.match(/code=(\d{6})/) || s.match(/^(\d{6})$/)
    return m ? m[1] : ''
  },

  onTapSubmit() {
    this.submitCode(this.data.code.trim())
  },

  async submitCode(code: string) {
    if (!/^\d{6}$/.test(code)) {
      this.setData({ error: '签到码为 6 位数字' })
      return
    }
    this.setData({ submitting: true, error: '' })
    try {
      const res = await request<ScanResult>({
        path: '/api/checkins/scan',
        method: 'POST',
        data: { code }
      })
      const meta = STATUS_META[res.status] || { text: res.status, theme: 'gray' }
      this.setData({
        submitting: false,
        code: '',
        result: { ...res, statusText: meta.text, theme: meta.theme }
      })
    } catch (err: any) {
      this.setData({ submitting: false, error: err.message || '签到失败' })
    }
  },

  goRecords() {
    wx.navigateTo({ url: '/pages/checkin-records/checkin-records' })
  }
})
