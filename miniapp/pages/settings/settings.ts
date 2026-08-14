import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { request } from '../../utils/request'

/**
 * 系统参数配置(仅 '*' 管理员)。三节表单与 web 端 SettingsPage 一致:
 * 掩码密钥留空不上送新值(PUT 整表照发,后端按 flags 处理);
 * 校验镜像服务端,不合法时禁用保存。
 */

interface MailForm {
  enabled: boolean
  host: string
  port: string
  username: string
  password: string
  passwordSet: boolean
  fromName: string
  fromAddress: string
  encryption: string
}

interface StorageForm {
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  accessKey: string
  secretKey: string
  secretKeySet: boolean
  useSsl: boolean
  usePathStyle: boolean
  publicBaseUrl: string
}

interface AiForm {
  enabled: boolean
  baseUrl: string
  model: string
  apiKey: string
  apiKeySet: boolean
}

const emptyMail: MailForm = {
  enabled: false, host: '', port: '465', username: '', password: '', passwordSet: false,
  fromName: '', fromAddress: '', encryption: 'ssl'
}

const emptyStorage: StorageForm = {
  enabled: false, endpoint: '', region: '', bucket: '', accessKey: '', secretKey: '',
  secretKeySet: false, useSsl: true, usePathStyle: true, publicBaseUrl: ''
}

const emptyAi: AiForm = { enabled: false, baseUrl: '', model: '', apiKey: '', apiKeySet: false }

const ENCRYPTIONS = ['ssl', 'starttls', 'none']
const ENCRYPTION_LABELS = ['SSL', 'STARTTLS', '无']

Page({
  data: {
    allowed: false,
    loaded: false,
    pageError: '',
    mail: { ...emptyMail },
    storage: { ...emptyStorage },
    ai: { ...emptyAi },
    encryptionLabels: ENCRYPTION_LABELS,
    mailEncryptionIndex: 0,
    mailSaving: false,
    mailError: '',
    storageSaving: false,
    storageError: '',
    aiSaving: false,
    aiError: ''
  },

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ allowed: can('*') })
    if (!this.data.allowed) return
    try {
      const [mail, storage, ai] = await Promise.all([
        request<MailForm>({ path: '/api/settings/email' }),
        request<StorageForm>({ path: '/api/settings/storage' }),
        request<AiForm>({ path: '/api/settings/ai' })
      ])
      this.setData({
        mail: { ...emptyMail, ...mail, port: String(mail.port != null ? mail.port : 465) },
        storage: { ...emptyStorage, ...storage },
        ai: { ...emptyAi, ...ai },
        mailEncryptionIndex: Math.max(0, ENCRYPTIONS.indexOf(mail.encryption)),
        loaded: true
      })
    } catch (err: any) {
      this.setData({ pageError: (err && err.message) || '加载失败' })
    }
  },

  // ---- 通用输入 ----

  onMailInput(e: WechatMiniprogram.Input) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`mail.${field}`]: e.detail.value })
  },

  onMailToggle(e: WechatMiniprogram.SwitchChange) {
    this.setData({ 'mail.enabled': Boolean(e.detail.value) })
  },

  onMailEncryptionChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    this.setData({ mailEncryptionIndex: idx, 'mail.encryption': ENCRYPTIONS[idx] })
  },

  onStorageInput(e: WechatMiniprogram.Input) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`storage.${field}`]: e.detail.value })
  },

  onStorageToggle(e: WechatMiniprogram.SwitchChange) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`storage.${field}`]: Boolean(e.detail.value) })
  },

  onAiInput(e: WechatMiniprogram.Input) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`ai.${field}`]: e.detail.value })
  },

  onAiToggle(e: WechatMiniprogram.SwitchChange) {
    this.setData({ 'ai.enabled': Boolean(e.detail.value) })
  },

  // ---- 校验(镜像 web) ----

  mailValid(): boolean {
    const f = this.data.mail
    if (!f.enabled) return true
    const port = Number(f.port)
    return (
      f.host.trim() !== '' &&
      f.username.trim() !== '' &&
      f.fromAddress.trim() !== '' &&
      f.fromAddress.includes('@') &&
      port >= 1 &&
      port <= 65535
    )
  },

  storageValid(): boolean {
    const f = this.data.storage
    if (!f.enabled) return true
    return (
      f.endpoint.trim() !== '' &&
      !f.endpoint.includes('://') &&
      f.bucket.trim() !== '' &&
      f.accessKey.trim() !== ''
    )
  },

  aiValid(): boolean {
    const f = this.data.ai
    if (!f.enabled) return true
    const baseUrl = f.baseUrl.trim()
    return (
      (baseUrl.startsWith('http://') || baseUrl.startsWith('https://')) &&
      f.model.trim() !== '' &&
      (f.apiKey.trim() !== '' || f.apiKeySet)
    )
  },

  // ---- 保存 ----

  async saveMail() {
    const f = this.data.mail
    const body = {
      enabled: f.enabled,
      host: f.host.trim(),
      port: Number(f.port) || 0,
      username: f.username.trim(),
      password: f.password, // 留空=保持已存密钥(后端按 passwordSet 处理)
      fromName: f.fromName.trim(),
      fromAddress: f.fromAddress.trim(),
      encryption: f.encryption
    }
    this.setData({ mailSaving: true, mailError: '' })
    try {
      const data = await request<MailForm>({ path: '/api/settings/email', method: 'PUT', data: body })
      this.setData({ mail: { ...this.data.mail, ...data, port: String(data.port) }, mailEncryptionIndex: Math.max(0, ENCRYPTIONS.indexOf(data.encryption)) })
      wx.showToast({ title: '邮件服务配置已保存', icon: 'success' })
    } catch (err: any) {
      this.setData({ mailError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ mailSaving: false })
    }
  },

  async saveStorage() {
    const f = this.data.storage
    const body = {
      enabled: f.enabled,
      endpoint: f.endpoint.trim(),
      region: f.region.trim(),
      bucket: f.bucket.trim(),
      accessKey: f.accessKey.trim(),
      secretKey: f.secretKey, // 留空=保持已存密钥
      useSsl: f.useSsl,
      usePathStyle: f.usePathStyle,
      publicBaseUrl: f.publicBaseUrl.trim()
    }
    this.setData({ storageSaving: true, storageError: '' })
    try {
      const data = await request<StorageForm>({ path: '/api/settings/storage', method: 'PUT', data: body })
      this.setData({ storage: { ...this.data.storage, ...data } })
      wx.showToast({ title: '对象存储配置已保存', icon: 'success' })
    } catch (err: any) {
      this.setData({ storageError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ storageSaving: false })
    }
  },

  async saveAi() {
    const f = this.data.ai
    const body = {
      enabled: f.enabled,
      baseUrl: f.baseUrl.trim(),
      model: f.model.trim(),
      apiKey: f.apiKey // 留空=保持已存密钥
    }
    this.setData({ aiSaving: true, aiError: '' })
    try {
      const data = await request<AiForm>({ path: '/api/settings/ai', method: 'PUT', data: body })
      this.setData({ ai: { ...this.data.ai, ...data } })
      wx.showToast({ title: 'AI 助手配置已保存', icon: 'success' })
    } catch (err: any) {
      this.setData({ aiError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ aiSaving: false })
    }
  }
})
