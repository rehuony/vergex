import { useNavigate } from 'react-router-dom'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { Button } from '@/components/ui/button'

export function RegistrationDisabled() {
  const { language } = useLanguage()
  const navigate = useNavigate()

  const handleBackToLogin = () => {
    navigate('/login')
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-vergex-bg text-vergex-text">
      <div className="text-center max-w-md px-6">
        <img
          src="/icons/vergex-flow.svg"
          alt="Vergex Logo"
          className="w-16 h-16 mx-auto mb-4"
        />
        <h1 className="text-2xl font-semibold mb-3">
          {t('registrationClosed', language)}
        </h1>
        <p className="text-sm text-vergex-text-muted">
          {t('registrationClosedMessage', language)}
        </p>
        <Button variant="primary" className="mt-6" onClick={handleBackToLogin}>
          {t('backToLogin', language)}
        </Button>
      </div>
    </div>
  )
}
