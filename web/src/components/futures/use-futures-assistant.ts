import {
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from 'react'
import {
  type Constraints,
  type Drawing,
  type Plan,
  type FuturesChart,
  type FuturesState,
  streamFutures,
} from '@/api/futures'
import { eventErrorText, formatRequestError } from '@/utils/request-errors'
import { readBatch, type PlanBatch } from '@/utils/futures/futures-workspace'
import {
  deferredEntries,
  executionRank,
  mergeChecks,
} from '@/utils/futures/futures-plans'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { recordActivity, type ActivityItem } from './futures-activity'
import type { PlanCheck } from '@/utils/futures/futures-plans'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

interface FuturesAssistantOptions {
  busy: boolean
  setBusy: Dispatch<SetStateAction<boolean>>
  modelID: string
  modelName: string
  accountId: string
  venue: string
  symbol: string
  chart: MutableRefObject<FuturesChart | null>
  current: FuturesState['records'][string] | undefined
  selected: string
  setSelected: Dispatch<SetStateAction<string>>
  setCompare: Dispatch<SetStateAction<string[]>>
  limits: Constraints
  setLimits: Dispatch<SetStateAction<Constraints>>
  leverage: number
  batchKey: string
  rememberBatch: (value: PlanBatch, key?: string) => void
  reload: () => Promise<void>
  refreshHistory: () => Promise<void>
  setError: Dispatch<SetStateAction<string>>
  setOrderNotice: Dispatch<SetStateAction<string>>
  // Called instead of planning when no model is set up yet.
  onNeedModel: (text: string, compare: boolean) => void
}

// useFuturesAssistant runs the Futures assistant conversation: one planning
// turn at a time, streamed into messages, activity and entry checks.
export function useFuturesAssistant({
  busy,
  setBusy,
  modelID,
  modelName,
  accountId,
  venue,
  symbol,
  chart,
  current,
  selected,
  setSelected,
  setCompare,
  limits,
  setLimits,
  leverage,
  batchKey,
  rememberBatch,
  reload,
  refreshHistory,
  setError,
  setOrderNotice,
  onNeedModel,
}: FuturesAssistantOptions) {
  const { language } = useLanguage()
  const [messages, setMessages] = useState<ChatMessage[]>([]),
    [instruction, setInstruction] = useState(''),
    [futuresBusy, setFuturesBusy] = useState(false)
  const [trace, setTrace] = useState<ActivityItem[]>([])
  // The reply as the model writes it, until it is finished.
  const [streaming, setStreaming] = useState('')
  const [checks, setChecks] = useState<PlanCheck[]>([])
  const [turnPlanIDs, setTurnPlanIDs] = useState<string[]>([])
  const abort = useRef<AbortController | null>(null),
    session = useRef<string>(crypto.randomUUID()),
    chatFollow = useRef(true)
  useEffect(() => () => abort.current?.abort(), [])
  function resetTurn() {
    setChecks([])
    setTurnPlanIDs([])
    setTrace([])
  }
  async function send(text = instruction, compareStructures = false) {
    if (busy || !text.trim()) return
    if (!modelID) {
      setInstruction(text)
      onNeedModel(text, compareStructures)
      return
    }
    const context = chart.current?.context()
    if (!context || context.venue !== venue || context.symbol !== symbol) {
      setError(t('futures.chartSwitching', language))
      return
    }
    // A locked trade is immutable, but it must not lock the whole workspace.
    const separateDraft = Boolean(current && current.state !== 'draft')
    const revisionID =
      !compareStructures && current?.state === 'draft' ? selected : undefined
    const historyMessages = separateDraft ? [] : messages.slice(-16)
    if (separateDraft) {
      session.current = crypto.randomUUID()
      setSelected('')
      setCompare([])
      setMessages([])
    }
    setBusy(true)
    setFuturesBusy(true)
    // Changing one plan leaves the rest of its generation where they were.
    const previous = revisionID ? readBatch(batchKey) : null
    const kept = previous?.ids.filter((id) => id !== revisionID) ?? []
    const generation: PlanBatch = {
      id: crypto.randomUUID(),
      ids: [],
      created: new Date().toISOString(),
      deferred: previous?.deferred,
    }
    setError('')
    setOrderNotice('')
    setInstruction('')
    resetTurn()
    chatFollow.current = true
    setMessages((m) => [
      ...(separateDraft ? [] : m),
      { role: 'user', content: text },
    ])
    const controller = new AbortController()
    abort.current = controller
    let streamStarted = false
    let waiting = false
    let preferredPlan: Plan | undefined
    let partial = ''
    const setPartial = (text: string) => {
      partial = text
      setStreaming(text)
    }
    try {
      await streamFutures(
        {
          account_id: accountId,
          agent_model_id: modelID,
          agent_model_name: modelName || undefined,
          symbol,
          venue,
          interval: context.interval,
          visible_from: context.visible_from,
          visible_to: context.visible_to,
          message: text,
          language,
          plan_id: revisionID,
          execution_kind: 'auto',
          compare_structures: compareStructures,
          session_id: session.current,
          turn_id: generation.id,
          history: historyMessages,
          constraints: { ...limits, leverage },
          auto_size: Boolean(accountId),
        },
        controller.signal,
        (event) => {
          streamStarted = true
          setTrace((items) => recordActivity(items, event, undefined, language))
          if (event.type === 'assistant_delta')
            setPartial(partial + String(event.data.text))
          // What a round wrote before moving on to a tool is not its answer.
          if (
            event.type === 'iteration' ||
            event.type === 'tool_start' ||
            event.type === 'assistant' ||
            event.type === 'error'
          )
            setPartial('')
          if (event.type === 'assistant')
            setMessages((m) => [
              ...m,
              { role: 'assistant', content: String(event.data.message) },
            ])
          if (event.type === 'error') {
            setError(eventErrorText(event.data, language))
          }
          if (
            event.type === 'outcome' &&
            Number(event.data.drafts) === 0 &&
            waiting &&
            event.data.stop_reason === 'completed'
          ) {
            rememberBatch(
              { ...generation, ids: kept, waiting: kept.length === 0 },
              batchKey
            )
            setSelected('')
            setCompare([])
          }
          if (event.type === 'candidate_result') {
            const incoming = [event.data as unknown as PlanCheck]
            setChecks((previous) => mergeChecks(previous, incoming))
          }
          if (event.type === 'plans') {
            waiting = event.data.waiting === true
            if (Array.isArray(event.data.checks)) {
              const incoming = event.data.checks as PlanCheck[]
              setChecks((previous) => mergeChecks(previous, incoming))
              generation.deferred = deferredEntries(
                generation.deferred,
                incoming
              )
            }
            const plans = (event.data.plans as Plan[])
              ?.slice()
              .sort(
                (a, b) =>
                  executionRank(a.idea.execution.kind) -
                  executionRank(b.idea.execution.kind)
              )
            if (plans?.length) {
              generation.ids = [
                ...new Set([
                  ...kept,
                  ...generation.ids,
                  ...plans.map((p) => p.id),
                ]),
              ]
              rememberBatch({ ...generation }, batchKey)
              setLimits(plans[0].constraints)
              setTurnPlanIDs((ids) => [
                ...new Set([...ids, ...plans.map((plan) => plan.id)]),
              ])
              const rank = (p: Plan) => executionRank(p.idea.execution.kind)
              if (!preferredPlan || rank(plans[0]) <= rank(preferredPlan))
                preferredPlan = plans[0]
              setSelected(preferredPlan.id)

              setCompare([])
            } else if (generation.ids.length) {
              rememberBatch({ ...generation }, batchKey)
            }
            reload().catch(() => {})
          }
          if (event.type === 'drawings') {
            const drawings = event.data.drawings as Drawing[]
            chart.current
              ?.draw(drawings, 'agent', context.interval)
              .catch((e) => setError(e.message))
          }
        }
      )
    } catch (e) {
      if (!controller.signal.aborted) {
        if (!streamStarted) setMessages(separateDraft ? [] : messages)
        const message = formatRequestError(e, language)
        setError(message)
        setTrace((items) =>
          recordActivity(
            items,
            { type: 'error', data: { message } },
            undefined,
            language
          )
        )
      }
    } finally {
      // Stopping keeps what was written so far.
      if (partial) {
        const kept = partial
        setMessages((m) => [...m, { role: 'assistant', content: kept }])
      }
      setPartial('')
      if (controller.signal.aborted)
        setTrace((items) =>
          recordActivity(
            items,
            { type: 'cancelled', data: {} },
            undefined,
            language
          )
        )
      setBusy(false)
      setFuturesBusy(false)
      await reload().catch(() => {})
      await refreshHistory().catch(() => {})
    }
  }
  return {
    messages,
    setMessages,
    instruction,
    setInstruction,
    futuresBusy,
    trace,
    streaming,
    setTrace,
    checks,
    turnPlanIDs,
    session,
    abort,
    chatFollow,
    resetTurn,
    send,
  }
}
