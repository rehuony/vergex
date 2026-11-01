import { BrandLogo } from '@/components/common/brand-logo'
import React, { useState, useEffect, useId } from 'react'
import { Eye, EyeOff, Loader2 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import {
  useSupportedExchanges,
  useSupportedModels,
} from '@/hooks/use-connections'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'
import { ROUTES } from '@/constants/route-paths'
import { AuthSurface } from '@/components/auth/auth-surface'
import { dialogInputClass } from '@/components/common/dialog'
import { Button } from '@/components/ui/button'

const labelClass = 'text-[13px] font-medium text-vergex-text'
// On the bare page the dialog border is too faint to outline the fields.
const inputClass = cn(
  dialogInputClass,
  'border-[var(--line-strong)] hover:border-vergex-text-muted/50 aria-[invalid=true]:border-vergex-danger'
)
const forgotClass =
  'text-xs font-normal text-vergex-text-muted hover:text-vergex-text'

export function LoginPage() {
  const { language } = useLanguage()
  const { login } = useAuth()
  const navigate = useNavigate()
  const id = useId()
  const messageId = `${id}-message`
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const [expired, setExpired] = useState(false)
  const [loading, setLoading] = useState(false)
  // The counts come from the server, so they follow what it supports.
  const { data: exchanges } = useSupportedExchanges()
  const { data: models } = useSupportedModels()

  useEffect(() => {
    localStorage.removeItem('auth_token')
    localStorage.removeItem('auth_user')
    localStorage.removeItem('user_id')
  }, [])

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

  // One line above the button holds a failed sign-in or an expired session,
  // so a message never pushes the form around.
  const message = error || (expired ? t('sessionExpired', language) : '')
  const invalid = error !== ''

  return (
    <AuthSurface className="vergex-auth">
      {/* Self-contained centering grid — works regardless of parent flex setup */}
      <main className="flex-1 grid lg:grid-cols-2">
        {/* ───────── LEFT: brand panel (desktop only) ───────── */}
        <section className="hidden lg:flex flex-col justify-between p-12 xl:p-16 relative overflow-hidden">
          {/* A faint chart grid, fading out from behind the headline */}
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 [background-image:linear-gradient(var(--line)_1px,transparent_1px),linear-gradient(90deg,var(--line)_1px,transparent_1px)] [background-size:48px_48px] [mask-image:radial-gradient(ellipse_at_30%_50%,black_10%,transparent_70%)]"
          />
          {/* Soft brand background */}
          <div className="absolute -left-32 top-1/3 w-md h-112 bg-vergex-brand/10 rounded-full blur-3xl pointer-events-none" />
          <div className="absolute -right-16 bottom-0 w-72 h-72 bg-vergex-accent/8 rounded-full blur-3xl pointer-events-none" />

          {/* Brand mark */}
          <div className="flex items-center gap-3 relative">
            <BrandLogo />
          </div>

          {/* Headline */}
          <div className="relative max-w-lg">
            <div className="inline-flex items-center gap-2 mb-7 px-3 py-1 rounded-full border border-[var(--line)] bg-vergex-brand/6">
              <div className="w-1.5 h-1.5 rounded-full bg-vergex-brand" />
              <span className="text-xs font-medium text-vergex-brand">
                {t('authCoverBadge', language)}
              </span>
            </div>
            <h2 className="text-4xl xl:text-5xl font-bold tracking-tight text-vergex-text leading-[1.05]">
              {t('authCoverTitleLead', language)}
              <br />
              <span className="text-vergex-brand">
                {t('authCoverTitleAccent', language)}
              </span>
            </h2>
            <p className="mt-5 text-vergex-text-muted text-base leading-relaxed max-w-md">
              {t('authCoverBody', language)}
            </p>
          </div>

          {/* Stats strip */}
          <div className="relative grid grid-cols-3 gap-8 max-w-md">
            <Stat
              value={exchanges ? String(exchanges.length) : '—'}
              label={t('authStatExchanges', language)}
            />
            <Stat
              value={models ? String(models.length) : '—'}
              label={t('authStatModels', language)}
            />
            <Stat value="24/7" label={t('authStatAlwaysOn', language)} />
          </div>
        </section>

        {/* ───────── RIGHT: form panel ───────── */}
        <section className="flex flex-col relative bg-[var(--surface)] lg:border-l lg:border-[var(--line)]">
          {/* Without the cover panel, its logo takes the same corner here */}
          <header className="flex px-6 pt-6 sm:px-12 sm:pt-12 lg:hidden">
            <BrandLogo small />
          </header>

          <div className="flex flex-1 items-center justify-center p-6 sm:p-12">
            <div className="w-full max-w-sm">
              <h1 className="mb-8 text-[26px] sm:text-3xl font-bold leading-tight tracking-tight text-vergex-text">
                {t('signIn', language)}
              </h1>

              {/* Form */}
              <form onSubmit={handleLogin}>
                <div className="space-y-5">
                  <div>
                    <label
                      htmlFor={`${id}-email`}
                      className={cn(labelClass, 'mb-1.5 block')}
                    >
                      {t('email', language)}
                    </label>
                    <input
                      id={`${id}-email`}
                      type="email"
                      name="email"
                      value={email}
                      onChange={(e) => {
                        setEmail(e.target.value)
                        setError('')
                      }}
                      className={inputClass}
                      placeholder="you@example.com"
                      required
                      autoFocus
                      autoComplete="username"
                      aria-invalid={invalid}
                      aria-describedby={invalid ? messageId : undefined}
                    />
                  </div>

                  <div>
                    <div className="mb-1.5 flex items-center justify-between">
                      <label htmlFor={`${id}-password`} className={labelClass}>
                        {t('password', language)}
                      </label>
                      <Button
                        variant="link"
                        className={forgotClass}
                        type="button"
                        onClick={() => navigate(ROUTES.resetPassword)}
                      >
                        {t('forgotPassword', language)}
                      </Button>
                    </div>
                    <div className="relative">
                      <input
                        id={`${id}-password`}
                        type={showPassword ? 'text' : 'password'}
                        name="password"
                        value={password}
                        onChange={(e) => {
                          setPassword(e.target.value)
                          setError('')
                        }}
                        className={cn(inputClass, 'pr-10')}
                        placeholder="••••••••"
                        required
                        autoComplete="current-password"
                        aria-invalid={invalid}
                        aria-describedby={invalid ? messageId : undefined}
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        className="absolute right-1 top-1/2 size-8 -translate-y-1/2"
                        type="button"
                        onClick={() => setShowPassword(!showPassword)}
                        aria-label={t(
                          showPassword ? 'hidePassword' : 'showPassword',
                          language
                        )}
                        aria-pressed={showPassword}
                      >
                        {showPassword ? (
                          <EyeOff size={16} />
                        ) : (
                          <Eye size={16} />
                        )}
                      </Button>
                    </div>
                  </div>
                </div>

                <p
                  id={messageId}
                  aria-live="polite"
                  title={message || undefined}
                  className={cn(
                    'mt-1.5 h-5 truncate text-xs leading-5',
                    error ? 'text-vergex-danger' : 'text-vergex-text-muted'
                  )}
                >
                  {message}
                </p>

                <Button
                  variant="primary"
                  className="mt-1.5 w-full"
                  type="submit"
                  disabled={loading}
                >
                  {loading && <Loader2 size={16} className="animate-spin" />}
                  {loading ? t('loggingIn', language) : t('signIn', language)}
                </Button>
              </form>

              {/* Footer */}
              <div className="mt-10 flex items-center justify-between border-t border-[var(--line)] pt-5">
                {/* A status, marked like the cover badge so it is not taken for a link */}
                <span className="inline-flex items-center gap-2 text-xs text-vergex-text-muted">
                  <span
                    aria-hidden="true"
                    className="size-1.5 rounded-full bg-vergex-brand"
                  />
                  {t('localWorkspace', language)}
                </span>
                <Button
                  variant="link"
                  className={forgotClass}
                  type="button"
                  onClick={() => navigate(ROUTES.resetAccount)}
                >
                  {t('forgotAccount', language)}
                </Button>
              </div>
            </div>
          </div>
        </section>
      </main>
    </AuthSurface>
  )
}

function Stat({ value, label }: { value: string; label: string }) {
  return (
    <div>
      <div className="font-sans text-2xl xl:text-3xl font-bold text-vergex-text">
        {value}
      </div>
      <div className="mt-1 text-xs text-vergex-text-muted">{label}</div>
    </div>
  )
}
