import type { ReactNode } from 'react'
import { Loader2, X } from 'lucide-react'
import { formatRequestError } from '@/utils/request-errors'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import type { ActivityItem } from './futures-activity'

interface FuturesNoticesProps {
  busy: boolean
  // What is under way, while something is.
  progress: string
  notice: string
  error: string
  // An earlier trade reserves the account: what says so, and explains the block.
  blocker: ReactNode
  trace: ActivityItem[]
  onDismissError: () => void
}

// FuturesNotices is one line above the assistant's input, there only while it has something
// to say: the latest error, what is under way, a trade that reserves the account, or the
// latest order notice. What sits below it stays put, and the conversation above gives up the
// room. An error the blocker explains or the assistant's activity shows stays out of it, and
// one too long for the line opens in full.
export function FuturesNotices({
  busy,
  progress,
  notice,
  error,
  blocker,
  trace,
  onDismissError,
}: FuturesNoticesProps) {
  const { language } = useLanguage()
  const shownError =
    error &&
    !(
      blocker &&
      (error.includes('already has an approved or unresolved plan') ||
        error.includes('An earlier trade is reserving this account'))
    ) &&
    !trace.some((item) => item.state === 'failed' && item.detail === error)
      ? formatRequestError(error, 'en')
      : ''
  const line = shownError ? (
    <div role="alert" className="flex min-w-0 flex-1 items-center gap-1">
      <Popover>
        <PopoverTrigger asChild>
          <button
            type="button"
            className="min-w-0 flex-1 truncate text-left text-vergex-danger"
          >
            {shownError}
          </button>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-[min(360px,calc(100vw-32px))] whitespace-pre-wrap break-words p-3 text-[13px] leading-relaxed text-vergex-danger"
        >
          {shownError}
        </PopoverContent>
      </Popover>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-6 shrink-0 text-vergex-danger hover:text-vergex-danger"
        onClick={onDismissError}
        aria-label={t('futures.dismiss', language)}
      >
        <X className="size-3.5" aria-hidden="true" />
      </Button>
    </div>
  ) : busy ? (
    <>
      <Loader2
        className="size-3.5 shrink-0 animate-spin motion-reduce:animate-none"
        aria-hidden="true"
      />
      <span className="min-w-0 truncate" title={progress}>
        {progress}
      </span>
    </>
  ) : (
    blocker ||
    (notice && (
      <span className="min-w-0 truncate" title={notice}>
        {notice}
      </span>
    ))
  )
  return (
    // The live region stays while the line comes and goes, so what appears is read out.
    <div role="status" aria-live="polite" className="shrink-0">
      {line && (
        <div className="flex min-h-8 items-center gap-2 border-t border-vergex-line px-4 text-xs text-vergex-muted max-sm:min-h-10">
          {line}
        </div>
      )}
    </div>
  )
}
