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
  type PlanningChart,
  type PlanningState,
  streamPlanning,
} from '@/api/planning'
import { formatRequestError } from '@/utils/request-errors'
import type { PlanBatch } from '@/utils/planning/frontier-workspace'
import {
  deferredEntries,
  executionRank,
  mergeChecks,
} from '@/utils/planning/frontier-plans'
import { useLanguage } from '@/contexts/language-context'
import { recordActivity, type ActivityItem } from './planning-activity'
import type { PlanCheck } from '@/utils/planning/frontier-plans'

export interface ChatMessage {
  role: 'user' | 'assistant' | 'error'
  content: string
}

interface PlanningAssistantOptions {
  busy: boolean
  setBusy: Dispatch<SetStateAction<boolean>>
  modelID: string
  exchangeID: string
  venue: string
  symbol: string
  chart: MutableRefObject<PlanningChart | null>
  current: PlanningState['records'][string] | undefined
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

// usePlanningAssistant runs the Frontier assistant conversation: one planning
// turn at a time, streamed into messages, activity and entry checks.
export function usePlanningAssistant({
  busy,
  setBusy,
  modelID,
  exchangeID,
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
}: PlanningAssistantOptions) {
  const { language } = useLanguage()
  const [messages, setMessages] = useState<ChatMessage[]>([]),
    [instruction, setInstruction] = useState(''),
    [planningBusy, setPlanningBusy] = useState(false),
    [status, setStatus] = useState('')
  const [trace, setTrace] = useState<ActivityItem[]>([])
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
      setError(
        'The chart is still switching markets. Try again once it is ready.'
      )
      return
    }
    // A locked trade is immutable, but it must not lock the whole workspace.
    const separateDraft = Boolean(current && current.state !== 'draft')
    const revisionID =
      !compareStructures && current?.state === 'draft' ? selected : undefined
    const historyMessages = separateDraft
      ? []
      : messages.filter((m) => m.role !== 'error').slice(-16)
    if (separateDraft) {
      session.current = crypto.randomUUID()
      setSelected('')
      setCompare([])
      setMessages([])
    }
    setBusy(true)
    setPlanningBusy(true)
    const generation: PlanBatch = {
      id: crypto.randomUUID(),
      ids: [],
      created: new Date().toISOString(),
    }
    setError('')
    setOrderNotice('')
    setInstruction('')
    resetTurn()
    chatFollow.current = true
    setStatus('Reading current market context…')
    setMessages((m) => [
      ...(separateDraft ? [] : m),
      { role: 'user', content: text },
    ])
    const controller = new AbortController()
    abort.current = controller
    let streamStarted = false
    let waiting = false
    let preferredPlan: Plan | undefined
    try {
      await streamPlanning(
        {
          exchange_id: exchangeID,
          agent_model_id: modelID,
          symbol,
          venue,
          interval: context.interval,
          visible_from: context.visible_from,
          visible_to: context.visible_to,
          message: text,
          plan_id: revisionID,
          execution_kind: 'auto',
          compare_structures: compareStructures,
          session_id: session.current,
          turn_id: generation.id,
          history: historyMessages,
          constraints: { ...limits, leverage },
          auto_size: Boolean(exchangeID),
        },
        controller.signal,
        (event) => {
          streamStarted = true
          setTrace((items) => recordActivity(items, event, undefined, language))
          if (event.type === 'planning_sizing')
            setOrderNotice(String(event.data.message))
          if (event.type === 'assistant')
            setMessages((m) => [
              ...m,
              { role: 'assistant', content: String(event.data.message) },
            ])
          if (event.type === 'error') {
            const message = formatRequestError(String(event.data.message), 'en')
            setError(message)
          }
          if (event.type === 'status') setStatus(String(event.data.message))
          if (event.type === 'iteration')
            setStatus(
              `Planning round ${event.data.round} of ${event.data.max_rounds}`
            )
          if (event.type === 'outcome' && Number(event.data.drafts) === 0) {
            setStatus(
              'Planning completed without a validated draft. Review the explanation.'
            )
            if (waiting && event.data.stop_reason === 'completed') {
              rememberBatch({ ...generation, waiting: true }, batchKey)
              setSelected('')
              setCompare([])
            }
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
                ...new Set([...generation.ids, ...plans.map((p) => p.id)]),
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
        const message = formatRequestError(e, 'en')
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
      setPlanningBusy(false)
      setStatus(
        controller.signal.aborted ? 'Planning stopped' : 'Planning finished'
      )
      await reload().catch(() => {})
      await refreshHistory().catch(() => {})
    }
  }
  return {
    messages,
    setMessages,
    instruction,
    setInstruction,
    planningBusy,
    status,
    setStatus,
    trace,
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
