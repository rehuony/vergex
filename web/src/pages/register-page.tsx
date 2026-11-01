import React, { useEffect, useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import PasswordChecklist from 'react-password-checklist'
import { toast } from 'sonner'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { getSystemConfig } from '@/api/system-config'
import { BrandLogo } from '@/components/common/brand-logo'
import { AuthSurface } from '@/components/auth/auth-surface'
import { RegistrationDisabled } from '@/components/auth/registration-disabled'
import { Button } from '@/components/ui/button'

export function RegisterPage() {
  const { language } = useLanguage()
  const { register } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [betaCode, setBetaCode] = useState('')
  const [betaMode, setBetaMode] = useState(false)
  const [registrationEnabled, setRegistrationEnabled] = useState(true)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [passwordValid, setPasswordValid] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)

  useEffect(() => {
    getSystemConfig()
      .then((config) => {
        setBetaMode(config.beta_mode || false)
        setRegistrationEnabled(config.initialized === false)
      })
      .catch((err) => {
        console.error('Failed to fetch system config:', err)
      })
  }, [])

  if (!registrationEnabled) {
    return <RegistrationDisabled />
  }

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    if (!passwordValid) {
      setError(t('passwordNotMeetRequirements', language))
      return
    }

    if (betaMode && !betaCode.trim()) {
      setError(t('betaCodeRequired', language))
      return
    }

    setLoading(true)
    try {
      const result = await register(
        email,
        password,
        betaCode.trim() || undefined
      )

      if (!result.success) {
        const msg = result.message || t('registrationFailed', language)
        setError(msg)
        toast.error(msg)
      }
      // success path is handled in AuthContext (auto login + navigation)
    } catch (e) {
      console.error('Registration error:', e)
      const errorMsg =
        e instanceof Error ? e.message : t('registrationFailed', language)
      setError(errorMsg)
      toast.error(errorMsg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthSurface className="min-h-screen flex items-center justify-center py-12 font-sans vergex-auth">
      <div className="w-full max-w-lg relative z-10 px-6">
        <div className="mb-8 text-center">
          <div className="flex justify-center mb-6">
            <BrandLogo />
          </div>
          <h1 className="text-3xl font-bold tracking-tighter text-vergex-text mb-2">
            {t('registerTitle', language)}
          </h1>
          <p className="text-vergex-text-muted text-sm">
            {t('registerIntro', language)}
          </p>
        </div>

        <div className="bg-vergex-bg-lighter backdrop-blur-md border border-[var(--line)] rounded-lg overflow-hidden shadow-lg relative group">
          <div className="p-6 md:p-8 relative">
            <form onSubmit={handleRegister} className="space-y-5">
              <div>
                <label className="block text-xs tracking-normal text-vergex-text-muted mb-1.5 ml-1 font-bold">
                  {t('email', language)}
                </label>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full bg-vergex-bg border border-[var(--line)] rounded-lg px-4 py-3 text-sm focus:border-vergex-brand focus:ring-1 focus:ring-vergex-brand/50 outline-hidden transition-all placeholder-vergex-text-muted text-vergex-text font-sans"
                  placeholder="you@example.com"
                  required
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs tracking-normal text-vergex-text-muted mb-1.5 ml-1 font-bold">
                    {t('password', language)}
                  </label>
                  <div className="relative">
                    <input
                      type={showPassword ? 'text' : 'password'}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      className="w-full bg-vergex-bg border border-[var(--line)] rounded-lg px-4 py-3 text-sm focus:border-vergex-brand focus:ring-1 focus:ring-vergex-brand/50 outline-hidden transition-all placeholder-vergex-text-muted text-vergex-text font-sans pr-10"
                      placeholder="••••••••"
                      required
                    />
                    <Button
                      variant="ghost"
                      size="icon"
                      className="absolute right-1.5 top-1/2 size-8 -translate-y-1/2"
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                    >
                      {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                    </Button>
                  </div>
                </div>

                <div>
                  <label className="block text-xs tracking-normal text-vergex-text-muted mb-1.5 ml-1 font-bold">
                    {t('confirmPassword', language)}
                  </label>
                  <div className="relative">
                    <input
                      type={showConfirmPassword ? 'text' : 'password'}
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      className="w-full bg-vergex-bg border border-[var(--line)] rounded-lg px-4 py-3 text-sm focus:border-vergex-brand focus:ring-1 focus:ring-vergex-brand/50 outline-hidden transition-all placeholder-vergex-text-muted text-vergex-text font-sans pr-10"
                      placeholder="••••••••"
                      required
                    />
                    <Button
                      variant="ghost"
                      size="icon"
                      className="absolute right-1.5 top-1/2 size-8 -translate-y-1/2"
                      type="button"
                      onClick={() =>
                        setShowConfirmPassword(!showConfirmPassword)
                      }
                    >
                      {showConfirmPassword ? (
                        <EyeOff size={16} />
                      ) : (
                        <Eye size={16} />
                      )}
                    </Button>
                  </div>
                </div>
              </div>

              <div className="bg-vergex-bg-deeper p-3 rounded-lg border border-[var(--line)]">
                <div className="text-[10px] tracking-normal text-vergex-text-muted mb-2 font-bold flex items-center gap-2">
                  <div className="w-1 h-1 rounded-full bg-vergex-text-muted"></div>
                  {t('passwordRequirements', language)}
                </div>
                <div className="text-xs font-sans text-vergex-text-muted">
                  <PasswordChecklist
                    rules={[
                      'minLength',
                      'capital',
                      'lowercase',
                      'number',
                      'specialChar',
                      'match',
                    ]}
                    minLength={8}
                    value={password}
                    valueAgain={confirmPassword}
                    messages={{
                      minLength: t('passwordRuleMinLength', language),
                      capital: t('passwordRuleUppercase', language),
                      lowercase: t('passwordRuleLowercase', language),
                      number: t('passwordRuleNumber', language),
                      specialChar: t('passwordRuleSpecial', language),
                      match: t('passwordRuleMatch', language),
                    }}
                    className="grid grid-cols-2 gap-x-4 gap-y-1"
                    onChange={(isValid) => setPasswordValid(isValid)}
                    iconSize={10}
                  />
                </div>
              </div>

              {betaMode && (
                <div>
                  <label className="block text-xs tracking-normal text-vergex-brand mb-1.5 ml-1 font-bold">
                    {t('betaCodeLabel', language)}
                  </label>
                  <input
                    type="text"
                    value={betaCode}
                    onChange={(e) =>
                      setBetaCode(
                        e.target.value.replace(/[^a-z0-9]/gi, '').toLowerCase()
                      )
                    }
                    className="w-full bg-vergex-bg border border-[var(--line)] rounded-lg px-4 py-3 text-sm focus:border-vergex-brand focus:ring-1 focus:ring-vergex-brand/50 outline-hidden transition-all placeholder-vergex-text-muted text-vergex-text font-sans tracking-widest"
                    placeholder="XXXXXX"
                    maxLength={6}
                    required={betaMode}
                  />
                </div>
              )}

              {error && (
                <div className="text-xs bg-vergex-danger/10 border border-vergex-danger/30 text-vergex-danger px-3 py-2 rounded-lg font-sans">
                  {error}
                </div>
              )}

              <Button
                variant="primary"
                size="lg"
                className="group mt-4 w-full"
                type="submit"
                disabled={
                  loading || (betaMode && !betaCode.trim()) || !passwordValid
                }
              >
                {loading ? (
                  <span className="animate-pulse">
                    {t('registering', language)}
                  </span>
                ) : (
                  t('registerButton', language)
                )}
              </Button>
            </form>
          </div>
        </div>

        <div className="text-center mt-8 space-y-4">
          <p className="text-xs font-sans text-vergex-text-muted">
            {t('hasAccount', language)}{' '}
            <Button
              variant="link"
              className="ml-1 text-xs"
              onClick={() => navigate('/login')}
            >
              {t('signIn', language)}
            </Button>
          </p>
        </div>
      </div>
    </AuthSurface>
  )
}
