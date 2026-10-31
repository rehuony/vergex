import type { MutableRefObject, ReactNode } from 'react'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import type { AIModel } from '@/types'
import { AssistantThread } from './assistant-thread'
import type { ActivityItem } from './planning-activity'
import type { ChatMessage } from './use-planning-assistant'

const COMPARE_PROMPT =
  'Compare four plans in this order: market entry, single limit, weighted ladder and TWAP for this market.'
const LEVELS_PROMPT =
  'Mark potential support and resistance on the current chart. Explain assumptions without generating trade plans yet.'

interface FrontierAssistantProps {
  busy: boolean
  planningBusy: boolean
  status: string
  models: AIModel[]
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
  // The selected plan's next step, shown above the composer.
  review: ReactNode
}

// FrontierAssistant sits beside the chart at the same height: the model, the
// conversation in its own scroll area, and the composer pinned below it.
export function FrontierAssistant({
  busy,
  planningBusy,
  status,
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
  review,
}: FrontierAssistantProps) {
  const { language } = useLanguage()
  const label = (key: string) => t(`frontier.${key}`, language)
  return (
    <aside
      id="frontier-assistant"
      className="flex min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)] max-lg:h-[80dvh] max-lg:min-h-96"
    >
      <header className="flex items-center gap-2 border-b border-[var(--line)] p-2">
        {models.length > 0 ? (
          <AppSelect
            aria-label={label('aiModel')}
            className="h-9 min-w-0 flex-1 max-sm:h-10"
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
            className="min-w-0 flex-1 truncate px-1 text-[13px] text-vergex-brand"
          >
            {label('addModel')}
          </SettingsLink>
        )}
        <Button
          type="button"
          size="sm"
          className="max-sm:h-10"
          disabled={busy}
          onClick={onNewIdea}
        >
          {label('newIdea')}
        </Button>
      </header>
      <AssistantThread
        messages={messages}
        trace={trace}
        planningBusy={planningBusy}
        hasPlan={hasPlan}
        followRef={followRef}
        onSuggest={onInstructionChange}
      />
      {busy && (
        <div
          className="flex items-center gap-2 border-t border-[var(--line)] px-3 py-2 text-xs text-vergex-text-muted"
          role="status"
          aria-live="polite"
        >
          <span className="planning-spinner" />
          <span className="min-w-0 truncate">
            {planningBusy
              ? status || label('preparingPlans')
              : label('validatingOrder')}
          </span>
        </div>
      )}
      {review}
      <form
        className="border-t border-[var(--line)] p-2"
        onSubmit={(event) => {
          event.preventDefault()
          onSend()
        }}
      >
        <textarea
          aria-label={label('tradeIdea')}
          className="block min-h-16 w-full resize-none rounded-lg border-0 bg-transparent p-2 text-[13px] text-vergex-text outline-none placeholder:text-vergex-text-muted focus:bg-transparent focus:shadow-none max-sm:text-base"
          value={instruction}
          onChange={(event) => onInstructionChange(event.target.value)}
          placeholder={label('composerHint')}
          maxLength={8000}
          disabled={busy}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            size="sm"
            className="max-sm:h-10"
            disabled={busy}
            onClick={() => onSend(LEVELS_PROMPT)}
          >
            {label('supportResistance')}
          </Button>
          <span className="ml-auto flex gap-2">
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              disabled={busy}
              onClick={() => onSend(instruction.trim() || COMPARE_PROMPT, true)}
            >
              {label('generatePlans')}
            </Button>
            {planningBusy ? (
              <Button
                type="button"
                size="sm"
                className="max-sm:h-10"
                onClick={onStop}
              >
                {label('stopPlanning')}
              </Button>
            ) : (
              <Button
                type="submit"
                variant="primary"
                size="sm"
                className="max-sm:h-10"
                disabled={busy || !instruction.trim()}
              >
                {label('send')}
              </Button>
            )}
          </span>
        </div>
      </form>
    </aside>
  )
}
