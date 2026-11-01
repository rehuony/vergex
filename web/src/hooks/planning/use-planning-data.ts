import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { type PlanningState, planningAPI } from '@/api/planning'
import { preservePlanningPlans } from '@/utils/planning/planning-presentation'

export interface PlanningConversation {
  id: string
  symbol: string
  exchange_id: string
  agent_model_id: string
  title: string
  updated_at: string
}

// usePlanningData keeps the server's planning state (plans, approvals and
// runs) polled every 4 seconds, plus saved conversations and preferences.
export function usePlanningData(setError: Dispatch<SetStateAction<string>>) {
  const [data, setData] = useState<PlanningState>({
    records: {},
    accounts: {},
    runs: [],
    mode: 'live',
    now: '',
  })
  const [preferences, setPreferences] = useState(''),
    [history, setHistory] = useState<PlanningConversation[]>([])
  const active = useRef(true)
  const reload = useCallback(async () => {
    const next = await planningAPI<PlanningState>('state')
    if (active.current)
      setData((old) =>
        JSON.stringify({ ...old, now: '' }) ===
        JSON.stringify({ ...next, now: '' })
          ? { ...old, now: next.now }
          : preservePlanningPlans(old, next)
      )
  }, [])
  const refreshHistory = useCallback(async () => {
    const rows = await planningAPI<typeof history>('conversations')
    if (active.current) setHistory(rows)
  }, [])
  useEffect(() => {
    active.current = true
    reload().catch((e) => setError(e.message))
    refreshHistory().catch(() => {})
    planningAPI<{ text: string }>('preferences')
      .then((p) => setPreferences(p.text))
      .catch(() => {})
    const timer = window.setInterval(() => reload().catch(() => {}), 4000)
    return () => {
      active.current = false
      clearInterval(timer)
    }
  }, [reload, refreshHistory])
  return {
    data,
    reload,
    history,
    refreshHistory,
    preferences,
    setPreferences,
  }
}
