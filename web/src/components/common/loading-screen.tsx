import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'

export function LoadingScreen() {
  const { language } = useLanguage()

  return (
    <div className="flex min-h-screen items-center justify-center bg-vergex-bg">
      <div className="text-center">
        <img
          src="/icons/vergex-flow.svg"
          alt="Vergex Logo"
          className="w-16 h-16 mx-auto mb-4 animate-pulse"
        />
        <p className="text-vergex-text">{t('loading', language)}</p>
      </div>
    </div>
  )
}
