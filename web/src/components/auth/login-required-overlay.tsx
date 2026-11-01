import { Check, LogIn } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { Dialog } from '@/components/common/dialog'
import { Button } from '@/components/ui/button'

interface LoginRequiredOverlayProps {
  isOpen: boolean
  onClose: () => void
  featureName?: string
}

/** Shown when a signed-out visitor opens a page that needs an account. */
export function LoginRequiredOverlay({
  isOpen,
  onClose,
  featureName,
}: LoginRequiredOverlayProps) {
  const { language } = useLanguage()
  if (!isOpen) return null

  const tr = (key: string, params?: Record<string, string | number>) =>
    t(`loginRequired.${key}`, language, params)

  return (
    <Dialog
      title={tr('title')}
      description={
        featureName
          ? tr('subtitleWithFeature', { featureName })
          : tr('subtitleDefault')
      }
      icon={
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--brand-soft)] text-vergex-brand">
          <LogIn className="h-[18px] w-[18px]" />
        </div>
      }
      size="sm"
      onClose={onClose}
      footer={
        <>
          <Button type="button" onClick={onClose}>
            {tr('notNow')}
          </Button>
          <Button asChild variant="primary">
            <Link to="/login" onClick={onClose}>
              {tr('loginButton')}
            </Link>
          </Button>
        </>
      }
    >
      <p className="text-[13px] leading-relaxed text-vergex-text-muted">
        {tr('description')}
      </p>
      <ul className="mt-4 space-y-2 text-[13px] text-vergex-text">
        {['benefit1', 'benefit2', 'benefit3'].map((key) => (
          <li key={key} className="flex items-center gap-2">
            <Check className="h-4 w-4 shrink-0 text-vergex-success" />
            {tr(key)}
          </li>
        ))}
      </ul>
    </Dialog>
  )
}
