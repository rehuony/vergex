import { useRef, type MutableRefObject, type ReactNode } from 'react'
import { SquarePen } from 'lucide-react'
import { ChatComposer } from '@/components/common/chat-composer'
import { ChatThread, type ChatMessage } from '@/components/common/chat-thread'
import { ModelPicker } from '@/components/common/model-picker'
import { Segmented } from '@/components/common/segmented'
import { TurnSteps, type ActivityItem } from '@/components/common/turn-steps'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import type { AgentModel } from '@/types'

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
  modelName: string
  // Another of that connection's models, or blank for the one it is set to.
  onModelChange: (id: string, name?: string) => void
  instruction: string
  onInstructionChange: (instruction: string) => void
  onSend: (text?: string, compareStructures?: boolean) => void
  onStop: () => void
  onNewIdea: () => void
  messages: ChatMessage[]
  trace: ActivityItem[]
  // The reply as the model writes it.
  streaming: string
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
  modelName,
  onModelChange,
  instruction,
  onInstructionChange,
  onSend,
  onStop,
  onNewIdea,
  messages,
  trace,
  streaming,
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
        <ChatThread
          messages={messages}
          steps={<TurnSteps items={trace} busy={futuresBusy} />}
          streaming={streaming}
          changes={[trace]}
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
          {messages.length === 0 && !hasPlan && (
            <div
              role="group"
              aria-label={label('suggestions')}
              className="flex flex-wrap gap-1.5 px-2"
            >
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
          <ChatComposer
            label={label('tradeIdea')}
            placeholder={label('composerHint')}
            value={instruction}
            onChange={onInstructionChange}
            onSubmit={() => onSend()}
            disabled={busy}
            inputRef={input}
            controls={
              <>
                {models.length > 0 ? (
                  <ModelPicker
                    label={label('agentModel')}
                    models={models}
                    modelID={modelID}
                    modelName={modelName}
                    disabled={busy}
                    onChange={onModelChange}
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
              </>
            }
          />
        </div>
      )}
    </aside>
  )
}
