import { useEffect, useRef, useState } from 'react'
import { Loader2, MessageSquare, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'
import { AssistantWindow } from './assistant-window'
import { useAssistant } from './use-assistant'

// The assistant's way in, always in the same corner. It sits clear of what pages pin at
// their bottom right: nothing there comes closer than 40px from either edge, or on a phone
// 48px from the bottom once the page has scrolled to its end. It stays under the top bar and
// a dialog's overlay, which is how a dialog keeps the page to itself.
export function AssistantDock() {
  const { language } = useLanguage()
  const [open, setOpen] = useState(false)
  const assistant = useAssistant(open)
  const icon = useRef<HTMLButtonElement>(null)
  const wasOpen = useRef(false)
  useEffect(() => {
    if (wasOpen.current && !open) icon.current?.focus()
    wasOpen.current = open
  }, [open])
  const waiting = Boolean(assistant.confirm)
  return (
    <>
      {open && (
        <AssistantWindow assistant={assistant} onClose={() => setOpen(false)} />
      )}
      <Button
        ref={icon}
        type="button"
        variant="ghost"
        size="icon"
        aria-label={t(open ? 'assistant.close' : 'assistant.open', language)}
        aria-expanded={open}
        title={
          !open && waiting
            ? t('assistant.waiting', language)
            : !open && assistant.unread
              ? t('assistant.unread', language)
              : undefined
        }
        onClick={() => setOpen(!open)}
        className={cn(
          'fixed bottom-[max(8px,env(safe-area-inset-bottom))] right-2 z-40 size-8 border border-vergex-line bg-vergex-surface text-vergex-ink shadow-(--shadow-float) hover:bg-vergex-surface-muted max-sm:size-10',
          // A phone's window fills the screen and has a close button of its own.
          open && 'max-sm:hidden'
        )}
      >
        {open ? (
          <X className="size-4" aria-hidden="true" />
        ) : assistant.busy ? (
          <Loader2
            className="size-4 animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        ) : (
          <MessageSquare className="size-4" aria-hidden="true" />
        )}
        {!open && (waiting || assistant.unread) && (
          <span
            aria-hidden="true"
            className={cn(
              'absolute right-0 top-0 size-1.5 rounded-full',
              waiting ? 'bg-vergex-warning' : 'bg-vergex-ink'
            )}
          />
        )}
      </Button>
    </>
  )
}
