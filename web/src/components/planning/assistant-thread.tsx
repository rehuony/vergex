import { useEffect, useRef, type MutableRefObject } from 'react'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'
import { PlanningActivity, type ActivityItem } from './planning-activity'
import type { ChatMessage } from './use-planning-assistant'

interface AssistantThreadProps {
  messages: ChatMessage[]
  trace: ActivityItem[]
  planningBusy: boolean
  hasPlan: boolean
  // Whether the thread keeps scrolling to the newest message.
  followRef: MutableRefObject<boolean>
  onSuggest: (instruction: string) => void
}

const STARTERS: [string, string][] = [
  ['starterPullback', 'Find a pullback entry with clear risk and exits.'],
  [
    'starterCompare',
    'Compare market, single-limit, price-ladder and TWAP entries for this market.',
  ],
  ['starterLevels', 'Mark support and resistance, without creating orders.'],
]

// AssistantThread is the assistant conversation and the current turn's
// activity. It follows new messages unless the reader scrolled up.
export function AssistantThread({
  messages,
  trace,
  planningBusy,
  hasPlan,
  followRef,
  onSuggest,
}: AssistantThreadProps) {
  const { language } = useLanguage()
  const label = (key: string) => t(`frontier.${key}`, language)
  const box = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (box.current && followRef.current)
      box.current.scrollTop =
        messages.length || trace.length ? box.current.scrollHeight : 0
  }, [messages, trace])
  return (
    <ScrollArea
      className="min-h-40 flex-1"
      viewportRef={box}
      onViewportScroll={() => {
        if (box.current)
          followRef.current =
            box.current.scrollHeight -
              box.current.scrollTop -
              box.current.clientHeight <
            80
      }}
    >
      <div className="flex flex-col gap-3 p-3" id="frontier-chat-panel">
        {messages.length === 0 && !hasPlan && (
          <div
            className="flex flex-col gap-2"
            aria-label={label('suggestions')}
          >
            <p className="text-[13px] text-vergex-text-muted">
              {label('welcome')}
            </p>
            {STARTERS.map(([key, instruction]) => (
              <Button
                key={key}
                type="button"
                className="h-auto min-h-10 w-full justify-start whitespace-normal px-3 py-2 text-left text-[13px] font-normal"
                onClick={() => onSuggest(instruction)}
              >
                {label(key)}
              </Button>
            ))}
          </div>
        )}
        {messages.length === 0 && hasPlan && (
          <p className="text-[13px] text-vergex-text-muted">
            {label('planOnChart')}
          </p>
        )}
        {messages
          .filter(
            (m, i) =>
              m.role !== 'user' ||
              i === 0 ||
              messages[i - 1].role !== 'user' ||
              messages[i - 1].content !== m.content
          )
          .map((m, i) => (
            <div
              key={i}
              className={cn(
                'text-[13px] leading-relaxed break-words whitespace-pre-wrap',
                m.role === 'user'
                  ? 'self-end rounded-xl rounded-br-sm bg-[var(--brand-soft)] px-3 py-2 text-vergex-text'
                  : 'text-vergex-text'
              )}
            >
              {m.content}
            </div>
          ))}
        <PlanningActivity items={trace} busy={planningBusy} />
      </div>
    </ScrollArea>
  )
}
