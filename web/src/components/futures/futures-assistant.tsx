import { useRef, type MutableRefObject, type ReactNode } from 'react'
import { SquarePen } from 'lucide-react'
import { Segmented } from '@/components/common/segmented'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AppSelect } from '@/components/ui/select'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import type { AgentModel } from '@/types'
import { AssistantThread } from './assistant-thread'
import type { ActivityItem } from './futures-activity'
import type { ChatMessage } from './use-futures-assistant'

// What each suggestion puts in the input, to edit or send.
const STARTERS = [
  ['starterPullback', 'starterPullbackPrompt'],
  ['starterCompare', 'starterComparePrompt'],
  ['starterLevels', 'starterLevelsPrompt'],
]

export type AssistantPane = 'chat' | 'plans'

interface FuturesAssistantProps {
  pane: AssistantPane
  onPaneChange: (pane: AssistantPane) => void
  planCount: number
  busy: boolean
  futuresBusy: boolean
  models: AgentModel[]
  modelID: string
  onModelChange: (id: string) => void
  instruction: string
  onInstructionChange: (instruction: string) => void
  onSend: (text?: string, compareStructures?: boolean) => void
  onStop: () => void
  onNewIdea: () => void
  messages: ChatMessage[]
  trace: ActivityItem[]
  hasPlan: boolean
  followRef: MutableRefObject<boolean>
  // The plans and the selected plan's details.
  plans: ReactNode
  // The selected plan's next step, pinned under the plans.
  actions: ReactNode
  // The line that is always there, for what is under way and what went wrong.
  notices: ReactNode
}

// FuturesAssistant sits beside the chart at the same height: the conversation and the
// plans it led to, one at a time, so a plan and the chart it is drawn on are in view
// together. The input stays with the conversation, with what could start one above it until
// one has started.
export function FuturesAssistant({
  pane,
  onPaneChange,
  planCount,
  busy,
  futuresBusy,
  models,
  modelID,
  onModelChange,
  instruction,
  onInstructionChange,
  onSend,
  onStop,
  onNewIdea,
  messages,
  trace,
  hasPlan,
  followRef,
  plans,
  actions,
  notices,
}: FuturesAssistantProps) {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  const input = useRef<HTMLTextAreaElement>(null)
  return (
    <aside
      id="futures-assistant"
      className="flex min-h-0 min-w-0 flex-col overflow-hidden rounded-2xl border border-vergex-line bg-vergex-surface max-lg:h-[80dvh] max-lg:min-h-96"
    >
      <header className="flex shrink-0 items-center gap-2 border-b border-vergex-line p-2">
        <Segmented
          aria-label={label('assistantPanes')}
          value={pane}
          options={[
            { value: 'chat', label: label('chatPane') },
            {
              value: 'plans',
              label: planCount
                ? `${label('plansTab')} ${planCount}`
                : label('plansTab'),
            },
          ]}
          onChange={onPaneChange}
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="ml-auto max-sm:size-10"
          aria-label={label('newIdea')}
          title={label('newIdea')}
          disabled={busy}
          onClick={onNewIdea}
        >
          <SquarePen className="size-4" aria-hidden="true" />
        </Button>
      </header>
      <div hidden={pane !== 'chat'} className="min-h-0 flex-1">
        <AssistantThread
          messages={messages}
          trace={trace}
          futuresBusy={futuresBusy}
          followRef={followRef}
        />
      </div>
      <div hidden={pane !== 'plans'} className="min-h-0 flex-1">
        <ScrollArea className="h-full">{plans}</ScrollArea>
      </div>
      {notices}
      {pane === 'plans' ? (
        actions
      ) : (
        <div className="shrink-0 space-y-2 p-2">
          {messages.length === 0 && (
            <div
              role="group"
              aria-label={label('suggestions')}
              className="space-y-2 px-2"
            >
              <p className="text-xs text-vergex-muted">
                {label(hasPlan ? 'planOnChart' : 'welcome')}
              </p>
              {!hasPlan && (
                <div className="flex flex-wrap gap-1.5">
                  {STARTERS.map(([name, prompt]) => (
                    <Button
                      key={name}
                      type="button"
                      size="sm"
                      className="h-7 px-2.5 text-xs font-normal max-sm:h-10"
                      onClick={() => {
                        onInstructionChange(label(prompt))
                        input.current?.focus()
                      }}
                    >
                      {label(name)}
                    </Button>
                  ))}
                </div>
              )}
            </div>
          )}
          <form
            onSubmit={(event) => {
              event.preventDefault()
              onSend()
            }}
          >
            <div className="rounded-md border border-vergex-line transition-colors focus-within:border-vergex-ink/40">
              <textarea
                ref={input}
                aria-label={label('tradeIdea')}
                className="block max-h-40 min-h-10 w-full resize-none border-0 bg-transparent px-3 py-2 text-[13px] leading-relaxed text-vergex-ink shadow-none outline-none [field-sizing:content] placeholder:text-vergex-faint focus:bg-transparent focus:shadow-none max-sm:text-base"
                value={instruction}
                onChange={(event) => onInstructionChange(event.target.value)}
                placeholder={label('composerHint')}
                maxLength={8000}
                disabled={busy}
                onKeyDown={(event) => {
                  // Enter while composing Chinese picks a word; it does not send.
                  if (
                    event.key === 'Enter' &&
                    !event.shiftKey &&
                    !event.nativeEvent.isComposing
                  ) {
                    event.preventDefault()
                    onSend()
                  }
                }}
              />
              <div className="flex items-center gap-2 p-1.5">
                {models.length > 0 ? (
                  <AppSelect
                    aria-label={label('agentModel')}
                    className="mr-auto h-8 w-auto min-w-0 max-w-[40%] border-transparent px-2 text-[13px] hover:bg-vergex-surface-muted max-sm:h-10"
                    value={modelID}
                    disabled={busy}
                    onChange={onModelChange}
                    options={models.map((model) => ({
                      value: model.id,
                      label: model.customModelName || model.name,
                    }))}
                  />
                ) : (
                  <SettingsLink
                    section="models"
                    className="mr-auto min-w-0 truncate px-2 text-[13px] text-vergex-ink"
                  >
                    {label('addModel')}
                  </SettingsLink>
                )}
                <Button
                  type="button"
                  size="sm"
                  className="shrink-0 max-sm:h-10"
                  disabled={busy}
                  onClick={() =>
                    onSend(instruction.trim() || label('comparePrompt'), true)
                  }
                >
                  {label('generatePlans')}
                </Button>
                {futuresBusy ? (
                  <Button
                    type="button"
                    size="sm"
                    className="shrink-0 max-sm:h-10"
                    onClick={onStop}
                  >
                    {label('stopFutures')}
                  </Button>
                ) : (
                  <Button
                    type="submit"
                    variant="primary"
                    size="sm"
                    className="shrink-0 max-sm:h-10"
                  >
                    {label('send')}
                  </Button>
                )}
              </div>
            </div>
          </form>
        </div>
      )}
    </aside>
  )
}
