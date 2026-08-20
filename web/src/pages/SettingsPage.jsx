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
  RadioButton,
  RadioButtonGroup,
  Select,
  SelectItem,
  TextInput,
  Toggle,
} from '@carbon/react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import { THEME_PREFERENCES, useTheme } from '../theme/ThemeContext'

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

// Appearance is a client-side preference (localStorage), unlike the server
// settings sections below, so it applies instantly and is available to every
// user - not just admins.
function AppearanceSection() {
  const { t } = useTranslation('settings')
  const { preference, setPreference } = useTheme()

  return (
    <section className="settings-page__section">
      <h2 className="settings-page__section-heading">{t('appearance.heading')}</h2>
      <p className="settings-page__section-hint">{t('appearance.hint')}</p>
      <RadioButtonGroup
        legendText={t('appearance.label')}
        name="theme-preference"
        valueSelected={preference}
        onChange={(value) => setPreference(value)}
      >
        {THEME_PREFERENCES.map((pref) => (
          <RadioButton
            key={pref}
            id={`theme-pref-${pref}`}
            labelText={t(`theme.${pref}`, { ns: 'common' })}
            value={pref}
          />
        ))}
      </RadioButtonGroup>
    </section>
  )
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
  const { t } = useTranslation('settings')
  const s = useSettings('/api/settings/email', mailDefaults, t('mail.savedNotice'))
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
      <h2 className="settings-page__section-heading">{t('mail.heading')}</h2>
      <p className="settings-page__section-hint">{t('mail.hint')}</p>
      {s.error && (
        <InlineNotification kind="error" lowContrast title={t('error.action')} subtitle={s.error} />
      )}
      {s.notice && <InlineNotification kind="success" lowContrast title={s.notice} />}
      <Toggle
        id="mail-enabled"
        labelText={t('mail.enabledLabel')}
        toggled={form.enabled}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, enabled: checked })}
      />
      <TextInput
        id="mail-host"
        labelText={t('mail.host')}
        placeholder="smtp.example.com"
        value={form.host}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, host: e.target.value })}
      />
      <NumberInput
        id="mail-port"
        label={t('mail.port')}
        min={1}
        max={65535}
        value={form.port}
        disabled={disabled || !s.loaded}
        invalidText={t('mail.portInvalid')}
        onChange={(e, { value }) => setForm({ ...form, port: Number(value) })}
      />
      <TextInput
        id="mail-username"
        labelText={t('mail.username')}
        value={form.username}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, username: e.target.value })}
      />
      <PasswordInput
        id="mail-password"
        labelText={t('mail.password')}
        placeholder={form.passwordSet ? t('placeholder.set') : t('placeholder.unset')}
        value={form.password}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, password: e.target.value })}
        showPasswordLabel={t('password.show', { ns: 'common' })}
        hidePasswordLabel={t('password.hide', { ns: 'common' })}
      />
      <TextInput
        id="mail-from-name"
        labelText={t('mail.fromName')}
        placeholder={t('mail.fromNamePlaceholder')}
        value={form.fromName}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, fromName: e.target.value })}
      />
      <TextInput
        id="mail-from-address"
        labelText={t('mail.fromAddress')}
        placeholder="notify@example.com"
        value={form.fromAddress}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, fromAddress: e.target.value })}
      />
      <Select
        id="mail-encryption"
        labelText={t('mail.encryption')}
        value={form.encryption}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, encryption: e.target.value })}
      >
        <SelectItem value="ssl" text={t('mail.encryptionSsl')} />
        <SelectItem value="starttls" text={t('mail.encryptionStarttls')} />
        <SelectItem value="none" text={t('mail.encryptionNone')} />
      </Select>
      {!disabled && (
        <Button size="md" disabled={s.saving || !s.loaded || !valid} onClick={s.save}>
          {t('saveButton')}
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
  const { t } = useTranslation('settings')
  const s = useSettings('/api/settings/storage', storageDefaults, t('storage.savedNotice'))
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
      <h2 className="settings-page__section-heading">{t('storage.heading')}</h2>
      <p className="settings-page__section-hint">{t('storage.hint')}</p>
      {s.error && (
        <InlineNotification kind="error" lowContrast title={t('error.action')} subtitle={s.error} />
      )}
      {s.notice && <InlineNotification kind="success" lowContrast title={s.notice} />}
      <Toggle
        id="storage-enabled"
        labelText={t('storage.enabledLabel')}
        toggled={form.enabled}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, enabled: checked })}
      />
      <TextInput
        id="storage-endpoint"
        labelText={t('storage.endpoint')}
        placeholder="cos.ap-guangzhou.myqcloud.com"
        value={form.endpoint}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, endpoint: e.target.value })}
      />
      <TextInput
        id="storage-region"
        labelText={t('storage.region')}
        placeholder="ap-guangzhou"
        value={form.region}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, region: e.target.value })}
      />
      <TextInput
        id="storage-bucket"
        labelText={t('storage.bucket')}
        value={form.bucket}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, bucket: e.target.value })}
      />
      <TextInput
        id="storage-access-key"
        labelText={t('storage.accessKey')}
        value={form.accessKey}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, accessKey: e.target.value })}
      />
      <PasswordInput
        id="storage-secret-key"
        labelText={t('storage.secretKey')}
        placeholder={form.secretKeySet ? t('placeholder.set') : t('placeholder.unset')}
        value={form.secretKey}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, secretKey: e.target.value })}
        showPasswordLabel={t('secret.show')}
        hidePasswordLabel={t('secret.hide')}
      />
      <Toggle
        id="storage-use-ssl"
        labelText={t('storage.useSsl')}
        toggled={form.useSsl}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, useSsl: checked })}
      />
      <Toggle
        id="storage-use-path-style"
        labelText={t('storage.usePathStyle')}
        toggled={form.usePathStyle}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, usePathStyle: checked })}
      />
      <TextInput
        id="storage-public-base-url"
        labelText={t('storage.publicBaseUrl')}
        placeholder="https://cdn.example.com"
        value={form.publicBaseUrl}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, publicBaseUrl: e.target.value })}
      />
      {!disabled && (
        <Button size="md" disabled={s.saving || !s.loaded || !valid} onClick={s.save}>
          {t('saveButton')}
        </Button>
      )}
    </section>
  )
}

function isHttpUrl(value) {
  try {
    const u = new URL(value)
    return u.protocol === 'http:' || u.protocol === 'https:'
  } catch {
    return false
  }
}

const aiDefaults = {
  enabled: false,
  baseUrl: '',
  model: '',
  apiKey: '',
  apiKeySet: false,
}

function AiSection({ disabled }) {
  const { t } = useTranslation('settings')
  const s = useSettings('/api/settings/ai', aiDefaults, t('ai.savedNotice'))
  const { form, setForm } = s
  // Mirrors the server-side validation for enabled services so the save
  // button stays disabled on invalid input.
  const valid =
    !form.enabled ||
    (isHttpUrl(form.baseUrl.trim()) &&
      form.model.trim() !== '' &&
      (form.apiKey.trim() !== '' || form.apiKeySet))

  return (
    <section className="settings-page__section">
      <h2 className="settings-page__section-heading">{t('ai.heading')}</h2>
      <p className="settings-page__section-hint">{t('ai.hint')}</p>
      {s.error && (
        <InlineNotification kind="error" lowContrast title={t('error.action')} subtitle={s.error} />
      )}
      {s.notice && <InlineNotification kind="success" lowContrast title={s.notice} />}
      <Toggle
        id="ai-enabled"
        labelText={t('ai.enabledLabel')}
        toggled={form.enabled}
        disabled={disabled || !s.loaded}
        onToggle={(checked) => setForm({ ...form, enabled: checked })}
      />
      <TextInput
        id="ai-base-url"
        labelText={t('ai.baseUrl')}
        placeholder="https://api.openai.com/v1"
        value={form.baseUrl}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, baseUrl: e.target.value })}
      />
      <TextInput
        id="ai-model"
        labelText={t('ai.model')}
        placeholder="gpt-4o-mini"
        value={form.model}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, model: e.target.value })}
      />
      <PasswordInput
        id="ai-api-key"
        labelText={t('ai.apiKey')}
        placeholder={form.apiKeySet ? t('placeholder.set') : t('placeholder.unset')}
        value={form.apiKey}
        disabled={disabled || !s.loaded}
        onChange={(e) => setForm({ ...form, apiKey: e.target.value })}
        showPasswordLabel={t('secret.show')}
        hidePasswordLabel={t('secret.hide')}
      />
      {!disabled && (
        <Button size="md" disabled={s.saving || !s.loaded || !valid} onClick={s.save}>
          {t('saveButton')}
        </Button>
      )}
    </section>
  )
}

export default function SettingsPage() {
  const { t } = useTranslation('settings')
  const { can } = useAuth()
  const navigate = useNavigate()
  const isAdmin = can('*')

  return (
    <div className="settings-page">
      <Grid fullWidth>
        <Column sm={4} md={8} lg={16}>
          <Breadcrumb aria-label={t('aria.breadcrumb', { ns: 'common' })}>
            <BreadcrumbItem
              href="/"
              onClick={(e) => {
                e.preventDefault()
                navigate('/')
              }}
            >
              {t('breadcrumb.home')}
            </BreadcrumbItem>
            <BreadcrumbItem isCurrentPage>{t('breadcrumb.current')}</BreadcrumbItem>
          </Breadcrumb>
          <h1 className="settings-page__heading">{t('title')}</h1>
          <p className="settings-page__subtitle">{t('subtitle')}</p>
          <AppearanceSection />
          <MailSection disabled={!isAdmin} />
          <StorageSection disabled={!isAdmin} />
          <AiSection disabled={!isAdmin} />
        </Column>
      </Grid>
    </div>
  )
}
