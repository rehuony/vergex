import React, { useEffect, useId, useState } from 'react'
import { useAuth } from '@/contexts/auth-context'
import { invalidateSystemConfig } from '@/api/system'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import {
  AuthField,
  AuthMessage,
  AuthPassword,
  AuthSurface,
  authInputClass,
} from '@/components/auth/auth-surface'
import { Button } from '@/components/ui/button'

export function SetupPage() {
  const { language } = useLanguage()
  const { register } = useAuth()
  const id = useId()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  // Clean up any stale auth state on setup page load
  useEffect(() => {
    localStorage.removeItem('auth_token')
    localStorage.removeItem('auth_user')
    localStorage.removeItem('user_id')
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (password.length < 8) {
      setError(t('setup.passwordError', language))
      return
    }
    setLoading(true)
    const result = await register(email, password)
    setLoading(false)
    if (result.success) {
      invalidateSystemConfig()
    } else {
      setError(result.message || t('setup.failed', language))
    }
  }

  return (
    <AuthSurface
      title={t('setup.welcome', language)}
      intro={t('setup.subtitle', language)}
    >
      <form onSubmit={handleSubmit}>
        <div className="space-y-4">
          <AuthField label={t('email', language)} htmlFor={`${id}-email`}>
            <input
              id={`${id}-email`}
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className={authInputClass}
              placeholder={t('setup.emailPlaceholder', language)}
              required
              autoFocus
            />
          </AuthField>

          <AuthField label={t('password', language)} htmlFor={`${id}-password`}>
            <AuthPassword
              id={`${id}-password`}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={t('setup.passwordPlaceholder', language)}
              required
            />
          </AuthField>
        </div>

        <AuthMessage error>{error}</AuthMessage>

        <Button
          variant="primary"
          size="lg"
          className="mt-2 w-full"
          type="submit"
          disabled={loading}
        >
          {loading
            ? t('setup.submitting', language)
            : t('setup.submit', language)}
        </Button>
      </form>
    </AuthSurface>
  )
}
