import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import { getAuthHeaders, handleJSONResponse } from '@/api/helpers'
import {
  accountPositions,
  type AccountPosition,
} from '@/utils/futures/futures-account'
import type { AccountObservation } from '@/utils/futures/futures-workspace'

export interface OpenOrder {
  order_id: string
  symbol: string
  side: string
  type: string
  price: number
  stop_price: number
  quantity: number
  status: string
}

const POLL_MS = 10000
const ORDER_READ_WAIT_MS = 30000

// useAccountWatch keeps reading the exchange for the account in view: positions on
// every market, open orders on this one, and whether a trader runs on it. It lives
// with the page, since the order blocker and reviews rely on what it observes
// whichever section is showing. It only reads.
export function useAccountWatch(accountID: string, symbol: string) {
  const [snapshot, setSnapshot] = useState<{
    accountID: string
    positions: AccountPosition[]
    at: string
  } | null>(null)
  const [orders, setOrders] = useState<{
    accountID: string
    symbol: string
    rows: OpenOrder[]
  } | null>(null)
  const [binding, setBinding] = useState<{
    id: string
    accountID: string
    running: boolean
  } | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [ordersFailed, setOrdersFailed] = useState(false)
  const generation = useRef(0)
  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (!accountID || inFlight.current) return
    const version = generation.current
    inFlight.current = true
    setLoading(true)
    try {
      const traders = (await api.getTraders(true)).filter(
        (t) => t.account_id === accountID
      )
      const traderID = traders[0]?.trader_id
      if (version !== generation.current) return
      setBinding(
        traderID
          ? {
              id: traderID,
              accountID,
              running: traders.some((t) => t.is_running),
            }
          : null
      )
      const values = accountPositions(
        await api.getAccountPositions(accountID, true)
      )
      if (version !== generation.current) return
      setSnapshot({
        accountID,
        positions: values,
        at: new Date().toISOString(),
      })
      setError('')
      const orderRequest = new AbortController()
      const orderDeadline = setTimeout(
        () => orderRequest.abort(),
        ORDER_READ_WAIT_MS
      )
      try {
        const query = new URLSearchParams({ symbol })
        const path = `/api/accounts/${encodeURIComponent(accountID)}/open-orders?`
        const response = await fetch(path + query, {
          headers: getAuthHeaders(),
          signal: orderRequest.signal,
        })
        const rows = await handleJSONResponse<OpenOrder[]>(response)
        if (
          !Array.isArray(rows) ||
          rows.some(
            (o) =>
              !o ||
              typeof o.order_id !== 'string' ||
              !o.order_id ||
              typeof o.type !== 'string' ||
              !Number.isFinite(Number(o.quantity))
          )
        )
          throw new Error('Incomplete order response')
        if (version !== generation.current) return
        setOrders({ accountID, symbol, rows })
        setOrdersFailed(false)
      } catch {
        if (version === generation.current) {
          setOrders(null)
          setOrdersFailed(true)
        }
      } finally {
        clearTimeout(orderDeadline)
      }
    } catch (e) {
      if (version === generation.current)
        setError(e instanceof Error ? e.message : String(e))
    } finally {
      if (version === generation.current) {
        setLoading(false)
        inFlight.current = false
      }
    }
  }, [accountID, symbol])

  useEffect(() => {
    generation.current++
    inFlight.current = false
    setSnapshot((old) => (old?.accountID === accountID ? old : null))
    setOrders(null)
    setError('')
    setBinding((old) => (old?.accountID === accountID ? old : null))
    let cancelled = false
    let timer: ReturnType<typeof setTimeout>
    const poll = async () => {
      if (!document.hidden) await refresh()
      if (!cancelled) timer = setTimeout(poll, POLL_MS)
    }
    void poll()
    return () => {
      cancelled = true
      clearTimeout(timer)
      generation.current++
    }
  }, [refresh, accountID])

  const observation = useMemo<AccountObservation | null>(
    () =>
      !error && snapshot?.accountID === accountID
        ? {
            accountID,
            symbol,
            positions: snapshot.positions,
            orders:
              !ordersFailed &&
              orders?.accountID === accountID &&
              orders.symbol === symbol
                ? orders.rows
                : null,
            checkedAt: Date.parse(snapshot.at),
          }
        : null,
    [accountID, symbol, snapshot, orders, error, ordersFailed]
  )

  return {
    snapshot,
    orders,
    binding,
    loading,
    error,
    ordersFailed,
    refresh,
    observation,
  }
}

export type AccountWatch = ReturnType<typeof useAccountWatch>
