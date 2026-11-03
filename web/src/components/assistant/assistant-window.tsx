import { useEffect, useRef, useState } from 'react'
import {
  ChevronLeft,
  History,
  RotateCcw,
  SquarePen,
  Trash2,
  X,
} from 'lucide-react'
import { ChatComposer } from '@/components/common/chat-composer'
import { ChatThread } from '@/components/common/chat-thread'
import { ModelPicker } from '@/components/common/model-picker'
import { confirmToast } from '@/components/common/notify'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { TurnSteps } from '@/components/common/turn-steps'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useLanguage } from '@/contexts/language-context'
import { useAssistantPageName } from '@/hooks/use-assistant-page'
import { t } from '@/i18n'
import { lockPageScroll } from '@/utils/page-scroll'
import { ActionResults } from './action-results'
import { ConfirmCard } from './confirm-card'
import { ConversationList } from './conversation-list'
import { ProposalCard } from './proposal-card'
import { ReplyLinks } from './reply-links'
import type { Assistant } from './use-assistant'

const PHONE = '(max-width: 639px)'

// What could start a conversation on each page. Each has a short name and what it puts in
// the input, under suggest.<page> in the copy.
const SUGGESTIONS: Record<string, string[]> = {
  dashboard: ['attention', 'today'],
  forecast_signals: ['call', 'record'],
  forecast_strategies: ['record', 'weakest'],
  forecast_backtests: ['read', 'chance'],
  forecast_settings: ['collecting'],
  futures: ['compare', 'risks'],
  traders: ['doing', 'quiet'],
  traders_overview: ['best', 'attention'],
}
const ELSEWHERE = 'dashboard'

// The assistant's window. It floats beside the page, which goes on working under it, so it
// is no dialog: it keeps no focus and locks nothing. On a phone there is no room beside the
// page, so it is a sheet from the top bar to the bottom of the screen, as the menu is.
// It is an aside rather than role="dialog", which is how Dialog finds the topmost dialog.
export function AssistantWindow({
  assistant,
  onClose,
}: {
  assistant: Assistant
  onClose: () => void
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`assistant.${key}`, language)
  const [view, setView] = useState<'chat' | 'list'>('chat')
  const input = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    input.current?.focus()
    if (window.matchMedia?.(PHONE).matches) return lockPageScroll()
  }, [])

  const pageName = useAssistantPageName()
  const suggestionsOf = SUGGESTIONS[pageName] ? pageName : ELSEWHERE
  const { thread, messages, models, confirm } = assistant
  const iconButton = 'size-8 shrink-0 max-sm:size-10'
  // Going to a page from a phone's window means leaving it for that page.
  const onLeave = () => {
    if (window.matchMedia?.(PHONE).matches) onClose()
  }
  const closeButton = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={label('close')}
      title={label('close')}
      className={`${iconButton} sm:hidden`}
      onClick={onClose}
    >
      <X className="size-4" aria-hidden="true" />
    </Button>
  )
  const newButton = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={label('newConversation')}
      title={label('newConversation')}
      className={iconButton}
      onClick={() => {
        if (assistant.newConversation()) {
          setView('chat')
          input.current?.focus()
        }
      }}
    >
      <SquarePen className="size-4" aria-hidden="true" />
    </Button>
  )
  const clearButton = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={label('clear')}
      title={label('clear')}
      className={iconButton}
      onClick={() => {
        // With nothing to clear there is nothing to ask about.
        if (assistant.conversations.data?.length === 0) return
        void confirmToast(label('clearMessage'), {
          title: label('clear'),
          okText: label('clear'),
          cancelText: label('cancel'),
          tone: 'danger',
        }).then((yes) => {
          if (yes) void assistant.clearConversations()
        })
      }}
    >
      <Trash2 className="size-4" aria-hidden="true" />
    </Button>
  )
  const started =
    messages.length > 0 ||
    assistant.busy ||
    Boolean(confirm) ||
    assistant.proposals.length > 0

  return (
    <aside
      aria-label={label('regionLabel')}
      onKeyDown={(event) => {
        // A list open inside the window marks the Escape it used to close.
        if (event.key === 'Escape' && !event.defaultPrevented) onClose()
      }}
      className="fixed bottom-12 right-[var(--page-gutter)] z-40 flex h-[min(640px,calc(100dvh-136px))] w-[400px] origin-bottom-right animate-menu-in flex-col overflow-hidden rounded-2xl border border-vergex-line bg-vergex-surface shadow-(--shadow-overlay) max-sm:inset-x-0 max-sm:bottom-0 max-sm:top-[72px] max-sm:h-auto max-sm:w-auto max-sm:rounded-none max-sm:border-x-0 max-sm:border-b-0 max-sm:shadow-none"
    >
      {view === 'list' ? (
        <>
          <header className="flex h-12 shrink-0 items-center gap-1 border-b border-vergex-line px-2">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={label('backToConversation')}
              title={label('backToConversation')}
              className={iconButton}
              onClick={() => setView('chat')}
            >
              <ChevronLeft className="size-4" aria-hidden="true" />
            </Button>
            <h2 className="min-w-0 flex-1 truncate text-sm font-semibold text-vergex-ink">
              {label('conversations')}
            </h2>
            {clearButton}
            {newButton}
            {closeButton}
          </header>
          <ConversationList
            assistant={assistant}
            onOpened={() => setView('chat')}
          />
          <TurnError assistant={assistant} />
        </>
      ) : (
        <>
          <header className="flex h-12 shrink-0 items-center gap-1 border-b border-vergex-line pl-4 pr-2">
            <h2
              className="min-w-0 flex-1 truncate text-sm font-semibold text-vergex-ink"
              title={assistant.title}
            >
              {started ? assistant.title : label('title')}
            </h2>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={label('conversations')}
              title={label('conversations')}
              className={iconButton}
              onClick={() => setView('list')}
            >
              <History className="size-4" aria-hidden="true" />
            </Button>
            {newButton}
            {closeButton}
          </header>
          <div className="min-h-0 flex-1">
            {thread.isError && !thread.data ? (
              <PanelState role="alert" className="h-full">
                <span>{label('threadFailed')}</span>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => void thread.refetch()}
                >
                  {label('retry')}
                </Button>
              </PanelState>
            ) : !thread.data ? (
              <PanelLoading />
            ) : started ? (
              <ChatThread
                messages={messages}
                steps={
                  <TurnSteps items={assistant.trace} busy={assistant.busy} />
                }
                streaming={assistant.streaming}
                renderReply={(message) =>
                  message.results?.length ? (
                    <ActionResults results={message.results} />
                  ) : undefined
                }
                changes={[
                  assistant.trace,
                  confirm,
                  assistant.links,
                  assistant.proposals,
                ]}
                followRef={assistant.follow}
                after={
                  <>
                    {assistant.proposals.map((proposal) => (
                      <ProposalCard
                        key={proposal.form}
                        proposal={proposal}
                        onFill={() => {
                          // The form is what to look at now.
                          if (assistant.fillProposal(proposal)) onClose()
                        }}
                        onDismiss={() => assistant.dismissProposal(proposal)}
                        onFollow={onLeave}
                      />
                    ))}
                    <ReplyLinks links={assistant.links} onFollow={onLeave} />
                  </>
                }
              />
            ) : (
              <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
                <p className="text-sm text-vergex-muted">
                  {label(models.length ? 'empty' : 'noModel')}
                </p>
                {models.length > 0 ? (
                  <div
                    role="group"
                    className="flex flex-wrap justify-center gap-1.5"
                  >
                    {SUGGESTIONS[suggestionsOf].map((name) => {
                      const key = `suggest.${suggestionsOf}.${name}`
                      return (
                        <Button
                          key={key}
                          type="button"
                          size="sm"
                          className="h-7 px-2.5 text-xs font-normal max-sm:h-10"
                          onClick={() => {
                            assistant.setInstruction(label(`${key}Prompt`))
                            input.current?.focus()
                          }}
                        >
                          {label(key)}
                        </Button>
                      )
                    })}
                  </div>
                ) : (
                  <SettingsLink
                    section="models"
                    className="text-[13px] text-vergex-ink underline-offset-4 hover:underline"
                  >
                    {label('addModel')}
                  </SettingsLink>
                )}
              </div>
            )}
          </div>
          <TurnError assistant={assistant} />
          {confirm && (
            <ConfirmCard
              confirmation={confirm}
              answering={assistant.answering}
              onAnswer={(confirmed) => void assistant.answer(confirmed)}
            />
          )}
          <div className="shrink-0 p-2 pb-[max(8px,env(safe-area-inset-bottom))]">
            <ChatComposer
              label={label('message')}
              placeholder={label(
                confirm ? 'placeholderWaiting' : 'placeholder'
              )}
              value={assistant.instruction}
              onChange={assistant.setInstruction}
              onSubmit={() => void assistant.send()}
              inputRef={input}
              controls={
                <>
                  {models.length > 0 ? (
                    <ModelPicker
                      label={label('model')}
                      models={models}
                      modelID={assistant.modelID}
                      modelName={assistant.modelName}
                      onChange={assistant.chooseModel}
                    />
                  ) : (
                    <SettingsLink
                      section="models"
                      className="mr-auto min-w-0 truncate px-2 text-[13px] text-vergex-ink"
                    >
                      {label('addModel')}
                    </SettingsLink>
                  )}
                  {assistant.busy && !assistant.answering ? (
                    <Button
                      type="button"
                      size="sm"
                      className="shrink-0 max-sm:h-10"
                      onClick={assistant.stop}
                    >
                      {label('stop')}
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
        </>
      )}
    </aside>
  )
}

// One line floating above the input while something went wrong, opening in full and taking
// no room, so nothing moves when it comes or goes.
function TurnError({ assistant }: { assistant: Assistant }) {
  const { language } = useLanguage()
  const label = (key: string) => t(`assistant.${key}`, language)
  const { error, messages } = assistant
  const asked = messages[messages.length - 1]?.role === 'user'
  return (
    <div
      role="status"
      aria-live="polite"
      className="relative z-10 h-0 shrink-0"
    >
      {error && (
        <div
          role="alert"
          className="absolute inset-x-2 bottom-2 flex min-h-8 animate-menu-in items-center gap-1 rounded-2xl border border-vergex-line bg-vergex-surface pl-3 pr-1 text-xs shadow-(--shadow-float) max-sm:min-h-10"
        >
          <Popover>
            <PopoverTrigger asChild>
              <button
                type="button"
                className="min-w-0 flex-1 truncate text-left text-vergex-danger"
              >
                {error}
              </button>
            </PopoverTrigger>
            <PopoverContent
              align="start"
              className="w-[min(360px,calc(100vw-32px))] whitespace-pre-wrap break-words p-3 text-[13px] leading-relaxed text-vergex-danger"
            >
              {error}
            </PopoverContent>
          </Popover>
          {asked && !assistant.busy && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-6 shrink-0 text-vergex-danger hover:text-vergex-danger max-sm:size-10"
              aria-label={label('retry')}
              title={label('retry')}
              onClick={assistant.retry}
            >
              <RotateCcw className="size-3.5" aria-hidden="true" />
            </Button>
          )}
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-6 shrink-0 text-vergex-danger hover:text-vergex-danger max-sm:size-10"
            aria-label={label('dismiss')}
            title={label('dismiss')}
            onClick={assistant.dismissError}
          >
            <X className="size-3.5" aria-hidden="true" />
          </Button>
        </div>
      )}
    </div>
  )
}
