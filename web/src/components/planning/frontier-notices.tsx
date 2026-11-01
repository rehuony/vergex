import { X } from 'lucide-react'
import { formatRequestError } from '@/utils/request-errors'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import type { ActivityItem } from './planning-activity'

interface FrontierNoticesProps {
  notice: string
  error: string
  // An earlier trade reserves the account; its banner explains the block.
  blocked: boolean
  trace: ActivityItem[]
  onDismissError: () => void
}

// FrontierNotices shows the latest order notice or error above the workspace,
// unless the order blocker or the assistant activity already shows it.
export function FrontierNotices({
  notice,
  error,
  blocked,
  trace,
  onDismissError,
}: FrontierNoticesProps) {
  const { language } = useLanguage()
  return (
    <>
      {notice && !blocked && !error && (
        <p className="frontier-order-notice" role="status">
          {notice}
        </p>
      )}
      {error &&
        !(
          blocked &&
          (error.includes('already has an approved or unresolved plan') ||
            error.includes('An earlier trade is reserving this account'))
        ) &&
        !trace.some(
          (item) => item.state === 'failed' && item.detail === error
        ) && (
          <div className="planning-error" role="alert">
            {formatRequestError(error, 'en')}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-7 shrink-0 text-vergex-danger hover:bg-vergex-danger/10 hover:text-vergex-danger"
              onClick={onDismissError}
              aria-label={t('frontier.dismiss', language)}
            >
              <X size={14} />
            </Button>
          </div>
        )}
    </>
  )
}
