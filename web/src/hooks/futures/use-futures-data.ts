import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
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
// runs) polled every 4 seconds, plus preferences and the account's saved
// conversations.
export function useFuturesData(
  setError: Dispatch<SetStateAction<string>>,
  accountId: string
) {
  const [data, setData] = useState<FuturesState>({
    records: {},
    accounts: {},
    runs: [],
    mode: 'live',
    now: '',
  })
  const [preferences, setPreferences] = useState('')
  const active = useRef(true)
  const client = useQueryClient()
  const historyKey = ['futures-conversations', accountId]
  const { data: history = [], refetch: refetchHistory } = useQuery({
    queryKey: historyKey,
    queryFn: () =>
      futuresAPI<FuturesConversation[]>(
        `conversations?account_id=${encodeURIComponent(accountId)}`
      ),
  })
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
    await refetchHistory()
  }, [refetchHistory])
  const renameConversation = async (id: string, title: string) => {
    const saved = await futuresAPI<{ title: string }>(
      `conversations/${id}`,
      { title },
      'PUT'
    )
    client.setQueryData<FuturesConversation[]>(historyKey, (rows) =>
      rows?.map((row) => (row.id === id ? { ...row, ...saved } : row))
    )
  }
  // The drafts it made that never ran go with it, so the plans are read again.
  const deleteConversation = async (id: string) => {
    await futuresAPI(`conversations/${id}`, undefined, 'DELETE')
    client.setQueryData<FuturesConversation[]>(historyKey, (rows) =>
      rows?.filter((row) => row.id !== id)
    )
    await reload().catch(() => {})
  }
  const deletePlan = useCallback(
    async (id: string) => {
      await futuresAPI(`plans/${encodeURIComponent(id)}`, undefined, 'DELETE')
      await reload()
    },
    [reload]
  )
  useEffect(() => {
    active.current = true
    reload().catch((e) => setError(e.message))
    futuresAPI<{ text: string }>('preferences')
      .then((p) => setPreferences(p.text))
      .catch(() => {})
    const timer = window.setInterval(() => reload().catch(() => {}), 4000)
    return () => {
      active.current = false
      clearInterval(timer)
    }
  }, [reload])
  return {
    data,
    reload,
    history,
    refreshHistory,
    renameConversation,
    deleteConversation,
    deletePlan,
    preferences,
    setPreferences,
  }
}
