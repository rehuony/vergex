import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { type FuturesState, futuresAPI } from '@/api/futures'
import { preserveFuturesPlans } from '@/utils/futures/futures-presentation'

export interface FuturesConversation {
  id: string
  symbol: string
  account_id: string
  agent_model_id: string
  title: string
  updated_at: string
}

// useFuturesData keeps the server's planning state (plans, approvals and
// runs) polled every 4 seconds, plus saved conversations and preferences.
export function useFuturesData(setError: Dispatch<SetStateAction<string>>) {
  const [data, setData] = useState<FuturesState>({
    records: {},
    accounts: {},
    runs: [],
    mode: 'live',
    now: '',
  })
  const [preferences, setPreferences] = useState(''),
    [history, setHistory] = useState<FuturesConversation[]>([])
  const active = useRef(true)
  const reload = useCallback(async () => {
    const next = await futuresAPI<FuturesState>('state')
    if (active.current)
      setData((old) =>
        JSON.stringify({ ...old, now: '' }) ===
        JSON.stringify({ ...next, now: '' })
          ? { ...old, now: next.now }
          : preserveFuturesPlans(old, next)
      )
  }, [])
  const refreshHistory = useCallback(async () => {
    const rows = await futuresAPI<typeof history>('conversations')
    if (active.current) setHistory(rows)
  }, [])
  useEffect(() => {
    active.current = true
    reload().catch((e) => setError(e.message))
    refreshHistory().catch(() => {})
    futuresAPI<{ text: string }>('preferences')
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
