import { useEffect, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  Grid,
  InlineNotification,
  NumberInput,
  PasswordInput,
  Select,
  SelectItem,
  TextInput,
  Toggle,
} from '@carbon/react'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'

// useSettings loads one settings endpoint into a form, saves via PUT, and
// re-applies the masked response over the form (so a stored secret shows as
// passwordSet rather than its value).
function useSettings(path, defaults, successText) {
  const { token } = useAuth()
  const [form, setForm] = useState(defaults)
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    apiFetch(path, { token })
      .then((data) => {
        setForm({ ...defaults, ...data })
        setLoaded(true)
      })
      .catch((err) => setError(err.message))
  }, [path, token])

  const save = () => {
    setSaving(true)
    setNotice('')
    apiFetch(path, { method: 'PUT', body: form, token })
      .then((data) => {
        setForm({ ...form, ...data })
        setNotice(successText)
      })
      .catch((err) => setError(err.message))
      .finally(() => setSaving(false))
  }
  return { form, setForm, loaded, saving, error, notice, save }
}

const mailDefaults = {
  enabled: false,
  host: '',
  port: 465,
  username: '',
  password: '',
  passwordSet: false,
  fromName: '',
  fromAddress: '',
  encryption: 'ssl',
}

function MailSection({ disabled }) {
  const s = useSettings('/api/settings/email', mailDefaults, '邮件服务配置已保存')
  const { form, setForm } = s
  // Mirrors the server-side validation for enabled services so the save
  // button stays disabled on invalid input.
  const valid =
    !form.enabled ||
    (form.host.trim() !== '' &&
      form.username.trim() !== '' &&
      form.fromAddress.trim() !== '' &&
      form.fromAddress.includes('@') &&
      form.port >= 1 &&
      form.port <= 65535)

  return (
    <section className="settings-page__section">
      <h2 className="settings-page__section-heading">邮件服务</h2>
      <p className="settings-page__section-hint">
        SMTP 发信配置。当前仅保存配置，尚未接入实际发送流程。
      </p>
      {s.error && (
        <InlineNotification kind="error" lowContrast title="操作失败" subtitle={s.error} />
      )}
      {s.notice && <InlineNotification kind="success" lowContrast title={s.notice} />}
      <Toggle
        id="mail-enabled"
        labelText="启用邮件服务"
        toggled={form.enabled}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, enabled: checked })}
      />
      <TextInput
        id="mail-host"
        labelText="SMTP 服务器"
        placeholder="smtp.example.com"
        value={form.host}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, host: e.target.value })}
      />
      <NumberInput
        id="mail-port"
        label="端口"
        min={1}
        max={65535}
        value={form.port}
        disabled={disabled || !s.loaded}
        invalidText="端口需在 1–65535 之间"
        onChange={(e, { value }) => setForm({ ...form, port: Number(value) })}
      />
      <TextInput
        id="mail-username"
        labelText="用户名"
        value={form.username}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, username: e.target.value })}
      />
      <PasswordInput
        id="mail-password"
        labelText="密码"
        placeholder={form.passwordSet ? '已设置，留空保持不变' : '未设置'}
        value={form.password}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, password: e.target.value })}
        showPasswordLabel="显示密码"
        hidePasswordLabel="隐藏密码"
      />
      <TextInput
        id="mail-from-name"
        labelText="发件人名称"
        placeholder="OCM 通知"
        value={form.fromName}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, fromName: e.target.value })}
      />
      <TextInput
        id="mail-from-address"
        labelText="发件人邮箱"
        placeholder="notify@example.com"
        value={form.fromAddress}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, fromAddress: e.target.value })}
      />
      <Select
        id="mail-encryption"
        labelText="加密方式"
        value={form.encryption}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, encryption: e.target.value })}
      >
        <SelectItem value="ssl" text="SSL" />
        <SelectItem value="starttls" text="STARTTLS" />
        <SelectItem value="none" text="不加密" />
      </Select>
      {!disabled && (
        <Button size="md" disabled={s.saving || !s.loaded || !valid} onClick={s.save}>
          保存配置
        </Button>
      )}
    </section>
  )
}

const storageDefaults = {
  enabled: false,
  endpoint: '',
  region: '',
  bucket: '',
  accessKey: '',
  secretKey: '',
  secretKeySet: false,
  useSsl: true,
  usePathStyle: true,
  publicBaseUrl: '',
}

function StorageSection({ disabled }) {
  const s = useSettings('/api/settings/storage', storageDefaults, '对象存储配置已保存')
  const { form, setForm } = s
  // Mirrors the server-side validation for enabled services so the save
  // button stays disabled on invalid input.
  const valid =
    !form.enabled ||
    (form.endpoint.trim() !== '' &&
      !form.endpoint.includes('://') &&
      form.bucket.trim() !== '' &&
      form.accessKey.trim() !== '')

  return (
    <section className="settings-page__section">
      <h2 className="settings-page__section-heading">对象存储</h2>
      <p className="settings-page__section-hint">
        S3 兼容存储（腾讯云 COS、阿里云 OSS、MinIO 等）配置。当前仅保存配置，尚未接入实际上传流程。
      </p>
      {s.error && (
        <InlineNotification kind="error" lowContrast title="操作失败" subtitle={s.error} />
      )}
      {s.notice && <InlineNotification kind="success" lowContrast title={s.notice} />}
      <Toggle
        id="storage-enabled"
        labelText="启用对象存储"
        toggled={form.enabled}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, enabled: checked })}
      />
      <TextInput
        id="storage-endpoint"
        labelText="服务地址 (Endpoint)"
        placeholder="cos.ap-guangzhou.myqcloud.com"
        value={form.endpoint}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, endpoint: e.target.value })}
      />
      <TextInput
        id="storage-region"
        labelText="地域 (Region)"
        placeholder="ap-guangzhou"
        value={form.region}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, region: e.target.value })}
      />
      <TextInput
        id="storage-bucket"
        labelText="存储桶 (Bucket)"
        value={form.bucket}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, bucket: e.target.value })}
      />
      <TextInput
        id="storage-access-key"
        labelText="AccessKey"
        value={form.accessKey}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, accessKey: e.target.value })}
      />
      <PasswordInput
        id="storage-secret-key"
        labelText="SecretKey"
        placeholder={form.secretKeySet ? '已设置，留空保持不变' : '未设置'}
        value={form.secretKey}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, secretKey: e.target.value })}
        showPasswordLabel="显示密钥"
        hidePasswordLabel="隐藏密钥"
      />
      <Toggle
        id="storage-use-ssl"
        labelText="使用 HTTPS"
        toggled={form.useSsl}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, useSsl: checked })}
      />
      <Toggle
        id="storage-use-path-style"
        labelText="Path-Style 访问（兼容 MinIO 等自托管）"
        toggled={form.usePathStyle}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, usePathStyle: checked })}
      />
      <TextInput
        id="storage-public-base-url"
        labelText="公开访问地址（可选）"
        placeholder="https://cdn.example.com"
        value={form.publicBaseUrl}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, publicBaseUrl: e.target.value })}
      />
      {!disabled && (
        <Button size="md" disabled={s.saving || !s.loaded || !valid} onClick={s.save}>
          保存配置
        </Button>
      )}
    </section>
  )
}

export default function SettingsPage() {
  const { can } = useAuth()
  const isAdmin = can('*')

  return (
    <div className="settings-page">
      <Grid fullWidth>
        <Column sm={4} md={8} lg={16}>
          <Breadcrumb>
            <BreadcrumbItem href="/">首页</BreadcrumbItem>
            <BreadcrumbItem isCurrentPage>参数配置</BreadcrumbItem>
          </Breadcrumb>
          <h1 className="settings-page__heading">参数配置</h1>
          <p className="settings-page__subtitle">
            邮件与对象存储服务配置，仅系统管理员可查看和修改。
          </p>
          <MailSection disabled={!isAdmin} />
          <StorageSection disabled={!isAdmin} />
        </Column>
      </Grid>
    </div>
  )
}
