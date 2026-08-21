import { useEffect, useRef, useState, type FormEvent, type MouseEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import {
  Button,
  Checkbox,
  Dropdown,
  InlineNotification,
  OverflowMenu,
  OverflowMenuItem,
  PasswordInput,
  TextInput,
} from '@carbon/react'
import { ArrowRight, Document, Edit, Translate } from '@carbon/icons-react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import useLanguage from '../i18n/useLanguage'
import logoUrl from '../assets/logo.png'
import wechatIconUrl from '../assets/wechat-icon.png'

// Inlined at build/dev-server time from the VITE_ICP_NUMBER env var.
// Empty means no ICP footer is rendered and the page stays unchanged.
const icp = import.meta.env.VITE_ICP_NUMBER || ''
// External docs site; when unset the Docs nav link is hidden.
const docsUrl = import.meta.env.VITE_DOCS_URL || ''
const REMEMBER_KEY = 'ocm.remembered-id'

const displayOnly = (event: MouseEvent) => event.preventDefault()

// Dropdown option shape for the sign-in realm picker.
interface RealmItem {
  id: string
  label: string
}

function WeChatIcon({ className }: { className?: string }) {
  return (
    <img
      src={wechatIconUrl}
      alt=""
      aria-hidden="true"
      className={className}
      style={{ display: 'block', height: '20px', width: '20px', objectFit: 'contain' }}
    />
  )
}

// Two-step sign-in: 'id' collects the account, 'password' unlocks it.
type Step = 'id' | 'password'

export default function LoginPage() {
  const { t } = useTranslation('login')
  const { languages, setLanguage } = useLanguage()
  const { user, login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const realmItems: RealmItem[] = [{ id: 'Email', label: t('email') }]

  const [step, setStep] = useState<Step>('id')
  const [username, setUsername] = useState(() => localStorage.getItem(REMEMBER_KEY) || '')
  const [password, setPassword] = useState('')
  const [rememberId, setRememberId] = useState(() => Boolean(localStorage.getItem(REMEMBER_KEY)))
  const [idError, setIdError] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const passwordRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (step === 'password') {
      passwordRef.current?.focus()
    }
  }, [step])

  if (user) {
    return <Navigate to="/" replace />
  }

  const handleContinue = (event: FormEvent) => {
    event.preventDefault()
    if (!username.trim()) {
      setIdError(t('enterId'))
      return
    }
    setIdError('')
    setError(null)
    setStep('password')
  }

  const handleBackToId = () => {
    setError(null)
    setPasswordError('')
    setStep('id')
  }

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (submitting) return
    if (!password) {
      setPasswordError(t('enterPassword'))
      return
    }
    setPasswordError('')
    setError(null)
    setSubmitting(true)
    try {
      if (rememberId) {
        localStorage.setItem(REMEMBER_KEY, username.trim())
      } else {
        localStorage.removeItem(REMEMBER_KEY)
      }
      await login(username.trim(), password)
      const state = location.state as { from?: { pathname?: string } } | null
      const target = state?.from?.pathname || '/'
      navigate(target, { replace: true })
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  // The WeChat SSO button is decorative until the mini-program flow lands;
  // renderIcon must return a component, so wrap the icon element.
  const wechatRenderIcon = () => <WeChatIcon />

  return (
    <div className="login">
      <header className="login__header">
        <a href="#" className="login__brand" onClick={displayOnly}>
          <span className="login__brand-name">{t('brand')}</span>
        </a>
        <nav className="login__nav" aria-label={t('docs')}>
          {docsUrl && (
            <a href={docsUrl} className="login__nav-item" target="_blank" rel="noreferrer">
              <span className="login__nav-icon"><Document size={20} /></span>
              <span className="login__nav-text">{t('docs')}</span>
            </a>
          )}
          <OverflowMenu
            className="login__nav-item login__lang"
            renderIcon={Translate}
            aria-label={t('aria.languageSwitcher', { ns: 'common' })}
            iconDescription={t('aria.languageSwitcher', { ns: 'common' })}
            align="bottom-end"
            flipped
          >
            {languages.map((lng) => (
              <OverflowMenuItem
                key={lng}
                itemText={t(`language.${lng}`, { ns: 'common' })}
                isDelete={false}
                onClick={() => setLanguage(lng)}
              />
            ))}
          </OverflowMenu>
        </nav>
      </header>

      <div className="login__container">
        <div className="login__panel">
          <div className="login__panel-main">
            <img src={logoUrl} className="login__logo" alt="" aria-hidden="true" />
            <h1 className="login__title">
              {t('titlePrefix')}
              <span className="login__title-cloud">{t('titleCloud')}</span>
            </h1>
            <p className="login__create-account">
              {t('createAccountPrefix')}{' '}
              <a href="#" onClick={displayOnly}>{t('createAccountLink')}</a>
            </p>

            <div className="login__rows">
              <form
                className={`login__id-row${step === 'password' ? ' login__id-row--out' : ''}`}
                onSubmit={handleContinue}
                noValidate
                aria-hidden={step === 'password'}
              >
                <div className="login__label">{t('signInWith')}</div>
                <div className="login__id-inputs">
                  <div className="login__realm">
                    <Dropdown
                      id="realm"
                      label={t('signInWith')}
                      // DropdownProps requires titleText (it renders a visible
                      // <label> when non-empty); pass an empty string — the
                      // page already renders its own .login__label above.
                      titleText=""
                      aria-label={t('signInWith')}
                      items={realmItems}
                      selectedItem={realmItems[0]}
                      size="lg"
                      type="default"
                    />
                  </div>
                  <div className="login__userid">
                    <TextInput
                      id="userid"
                      labelText={t('email')}
                      hideLabel
                      placeholder={t('usernamePlaceholder')}
                      value={username}
                      onChange={(e) => {
                        setUsername(e.target.value)
                        if (idError) setIdError('')
                      }}
                      invalid={Boolean(idError)}
                      invalidText={idError}
                      autoComplete="username"
                      size="lg"
                      autoFocus
                      tabIndex={step === 'password' ? -1 : 0}
                    />
                  </div>
                </div>
                <div className="login__button-row">
                  <Button type="submit" kind="primary" size="lg" renderIcon={ArrowRight} className="login__primary-btn">
                    {t('continue')}
                  </Button>
                </div>
                <div className="login__aux">
                  <a href="#" className="login__link" onClick={displayOnly} tabIndex={step === 'password' ? -1 : 0}>
                    {t('forgotId')}
                  </a>
                  <Checkbox
                    id="remember-id"
                    labelText={t('rememberId')}
                    checked={rememberId}
                    onChange={(_, { checked }) => setRememberId(checked)}
                    tabIndex={step === 'password' ? -1 : 0}
                  />
                </div>
                <div className="login__separator">{t('or')}</div>
                <Button
                  kind="tertiary"
                  size="lg"
                  renderIcon={wechatRenderIcon}
                  className="login__sso-btn"
                  onClick={displayOnly}
                  tabIndex={step === 'password' ? -1 : 0}
                >
                  {t('continueWithWeChat')}
                </Button>
              </form>

              <form
                className={`login__password-row${step === 'password' ? ' login__password-row--in' : ''}`}
                onSubmit={handleSubmit}
                noValidate
                aria-hidden={step === 'id'}
              >
                {error && (
                  <InlineNotification
                    kind="error"
                    title={t('errorTitle')}
                    subtitle={error}
                    lowContrast
                    hideCloseButton
                    className="login__error"
                  />
                )}
                <button type="button" className="login__id-display" onClick={handleBackToId}>
                  <Edit size={16} />
                  <span>{username}</span>
                </button>
                <PasswordInput
                  id="password"
                  labelText={t('password')}
                  placeholder={t('passwordPlaceholder')}
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value)
                    if (passwordError) setPasswordError('')
                  }}
                  invalid={Boolean(passwordError)}
                  invalidText={passwordError}
                  autoComplete="current-password"
                  showPasswordLabel={t('showPassword')}
                  hidePasswordLabel={t('hidePassword')}
                  size="lg"
                  ref={passwordRef}
                  tabIndex={step === 'id' ? -1 : 0}
                />
                <div className="login__button-row">
                  <Button
                    type="submit"
                    kind="primary"
                    size="lg"
                    renderIcon={ArrowRight}
                    className="login__primary-btn"
                    disabled={submitting}
                  >
                    {submitting ? t('loggingIn') : t('logIn')}
                  </Button>
                </div>
                <div className="login__aux login__aux--password">
                  <a href="#" className="login__link" onClick={displayOnly} tabIndex={step === 'id' ? -1 : 0}>
                    {t('forgotPassword')}
                  </a>
                </div>
              </form>
            </div>
          </div>

          <div className="login__copyright">
            {icp && (
              <>
                {' '}
                <a href="https://beian.miit.gov.cn" target="_blank" rel="noreferrer">
                  {icp}
                </a>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
