import { Loader2 } from 'lucide-react'
import type {
  AssistantConfirmation,
  AssistantConfirmItem,
} from '@/api/assistant'
import { SettingBadge } from '@/components/common/setting-row'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'

const ACTIONS = new Set([
  'create',
  'start',
  'stop',
  'delete',
  'update',
  'close',
  'delete_conversation',
  'delete_conversations',
  'delete_plan',
  'delete_plans',
])

// What waits for an answer, pinned above the input so it cannot scroll away. Its parts come
// from the server, never the model, so what is confirmed is what is read here. Each action
// is one row: what it does to what, whether that is real money, and the rest under it.
export function ConfirmCard({
  confirmation,
  answering,
  onAnswer,
}: {
  confirmation: AssistantConfirmation
  answering: boolean
  onAnswer: (confirmed: boolean) => void
}) {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`assistant.${key}`, language, params)
  const items: AssistantConfirmItem[] = confirmation.items?.length
    ? confirmation.items
    : confirmation.lines.map((line) => ({ target: line }))
  const only = items.length === 1 ? items[0] : null
  // The button says what it does: the one action by name, or all of them.
  const okText =
    only?.action && ACTIONS.has(only.action)
      ? label(`action.${only.action}`)
      : items.length > 1
        ? label('runAll', { count: items.length })
        : label('confirm')
  return (
    // About three rows show at once, and the rest scroll inside, so the conversation keeps
    // most of the window.
    <section
      aria-label={label('waiting')}
      className="mx-2 mb-1.5 flex max-h-[min(160px,35%)] min-h-0 shrink-0 animate-menu-in flex-col rounded-md border border-vergex-line bg-vergex-surface text-[13px] leading-snug text-vergex-ink"
    >
      <ScrollArea
        className="min-h-0 flex-1"
        viewportClassName="overscroll-contain"
      >
        <ul className="px-3">
          {items.map((item, index) => {
            const details = item.details?.join(' · ')
            const full = [item.target, item.account, details]
              .filter(Boolean)
              .join(' · ')
            return (
              <li
                key={index}
                title={full}
                className="flex min-w-0 items-center gap-2 border-b border-vergex-line py-1.5 last:border-b-0"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate">
                    {item.action && ACTIONS.has(item.action) && (
                      <span className="text-vergex-muted">
                        {label(`action.${item.action}`)}{' '}
                      </span>
                    )}
                    <span className="font-medium">{item.target}</span>
                    {item.account && (
                      <span className="text-vergex-muted">
                        {' · '}
                        {item.account}
                      </span>
                    )}
                  </p>
                  {details && (
                    <p className="line-clamp-2 break-words text-xs text-vergex-muted">
                      {details}
                    </p>
                  )}
                </div>
                {item.money === 'real' && (
                  <SettingBadge tone="warning">
                    {label('realMoney')}
                  </SettingBadge>
                )}
                {item.money === 'testnet' && (
                  <SettingBadge>{label('testnet')}</SettingBadge>
                )}
              </li>
            )
          })}
        </ul>
      </ScrollArea>
      <div className="flex shrink-0 items-center gap-2 border-t border-vergex-line py-1.5 pl-3 pr-1.5">
        <h3 className="min-w-0 flex-1 truncate text-[11px] font-medium text-vergex-muted">
          {items.length > 1
            ? label('waitingCount', { count: items.length })
            : label('waiting')}
        </h3>
        <Button
          type="button"
          size="sm"
          className="max-sm:h-10"
          onClick={() => onAnswer(false)}
        >
          {label('cancel')}
        </Button>
        <Button
          type="button"
          variant={confirmation.destructive ? 'destructive' : 'primary'}
          size="sm"
          className="max-w-[60%] max-sm:h-10"
          onClick={() => onAnswer(true)}
        >
          {answering && (
            <Loader2
              className="size-3.5 animate-spin motion-reduce:animate-none"
              aria-hidden="true"
            />
          )}
          <span className="truncate">{okText}</span>
        </Button>
      </div>
    </section>
  )
}
