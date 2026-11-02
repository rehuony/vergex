import React, { useState, useEffect, useId } from 'react'
import { Loader2 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { ROUTES } from '@/constants/route-paths'
import {
  AuthField,
  AuthMessage,
  AuthPassword,
  AuthSurface,
  authInputClass,
} from '@/components/auth/auth-surface'
import { Button } from '@/components/ui/button'

const forgotClass =
  'text-xs font-normal text-vergex-muted hover:text-vergex-ink'

export function LoginPage() {
  const { language } = useLanguage()
  const { login } = useAuth()
  const navigate = useNavigate()
  const id = useId()
  const messageId = `${id}-message`
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [expired, setExpired] = useState(false)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (sessionStorage.getItem('from401') === 'true') {
      setExpired(true)
      sessionStorage.removeItem('from401')
    }
  }, [])

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    const result = await login(email, password)
    setLoading(false)
    if (!result.success) {
      // Wrong credentials get the translated message; anything else (such as
      // too many attempts) shows what the server said.
      setError(
        result.status === 401
          ? t('invalidCredentials', language)
          : result.message || t('loginFailed', language)
      )
    }
  }

  const message = error || (expired ? t('sessionExpired', language) : '')
  const invalid = error !== ''

  return (
    <AuthSurface title={t('signIn', language)}>
      <form onSubmit={handleLogin}>
        <div className="space-y-4">
          <AuthField label={t('email', language)} htmlFor={`${id}-email`}>
            <input
              id={`${id}-email`}
              type="email"
              name="email"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value)
                setError('')
              }}
              className={authInputClass}
              placeholder="you@example.com"
              required
              autoFocus
              autoComplete="username"
              aria-invalid={invalid}
              aria-describedby={invalid ? messageId : undefined}
            />
          </AuthField>

          <AuthField
            label={t('password', language)}
            htmlFor={`${id}-password`}
            action={
              <Button
                variant="link"
                className={forgotClass}
                type="button"
                onClick={() => navigate(ROUTES.resetPassword)}
              >
                {t('forgotPassword', language)}
              </Button>
            }
          >
            <AuthPassword
              id={`${id}-password`}
              name="password"
              value={password}
              onChange={(e) => {
                setPassword(e.target.value)
                setError('')
              }}
              placeholder="••••••••"
              required
              autoComplete="current-password"
              aria-invalid={invalid}
              aria-describedby={invalid ? messageId : undefined}
            />
          </AuthField>
        </div>

        <AuthMessage id={messageId} error={invalid}>
          {message}
        </AuthMessage>

        <Button
          variant="primary"
          size="lg"
          className="mt-2 w-full"
          type="submit"
          disabled={loading}
        >
          {loading && <Loader2 size={16} className="animate-spin" />}
          {loading ? t('loggingIn', language) : t('signIn', language)}
        </Button>
      </form>

      <div className="mt-6 flex justify-center">
        <Button
          variant="link"
          className={forgotClass}
          type="button"
          onClick={() => navigate(ROUTES.resetUser)}
        >
          {t('forgotLogin', language)}
        </Button>
      </div>
    </AuthSurface>
  )
}
