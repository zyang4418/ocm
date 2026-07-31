import { useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import {
  Button,
  Column,
  Grid,
  InlineNotification,
  PasswordInput,
  Stack,
  TextInput,
  Theme,
  Tile,
} from '@carbon/react'
import { useAuth } from '../auth/AuthContext.jsx'

// Inlined at build/dev-server time from the VITE_ICP_NUMBER env var.
// Empty means no ICP footer is rendered and the page stays unchanged.
const icp = import.meta.env.VITE_ICP_NUMBER || ''

export default function LoginPage() {
  const { user, login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(null)
  const [submitting, setSubmitting] = useState(false)

  if (user) {
    return <Navigate to="/" replace />
  }

  const handleSubmit = async (event) => {
    event.preventDefault()
    if (submitting) return
    setError(null)
    setSubmitting(true)
    try {
      await login(username, password)
      const target = location.state?.from?.pathname || '/'
      navigate(target, { replace: true })
    } catch (err) {
      setError(err.message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Theme theme="g10" className="login-page">
      <Grid fullWidth className="login-page__grid">
        <Column sm={4} md={{ span: 4, offset: 2 }} lg={{ span: 4, offset: 6 }} className="login-page__column">
          <header className="login-page__brand">
            <p className="login-page__product">OCM</p>
            <h1 className="login-page__title">登录</h1>
            <p className="login-page__subtitle">企业运营管理平台</p>
          </header>

          <Tile className="login-page__tile">
            <form onSubmit={handleSubmit} noValidate>
              <Stack gap={6}>
                {error && (
                  <InlineNotification
                    kind="error"
                    title="登录失败"
                    subtitle={error}
                    lowContrast
                    hideCloseButton
                  />
                )}
                <TextInput
                  id="username"
                  labelText="用户名"
                  placeholder="请输入用户名"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  required
                />
                <PasswordInput
                  id="password"
                  labelText="密码"
                  placeholder="请输入密码"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                  showPasswordLabel="显示密码"
                  hidePasswordLabel="隐藏密码"
                  required
                />
                <Button type="submit" className="login-page__submit" disabled={submitting || !username || !password}>
                  {submitting ? '正在登录…' : '登录'}
                </Button>
              </Stack>
            </form>
          </Tile>
        </Column>
      </Grid>
      {icp && (
        <footer className="login-page__footer">
          <a href="https://beian.miit.gov.cn" target="_blank" rel="noreferrer">
            {icp}
          </a>
        </footer>
      )}
    </Theme>
  )
}
