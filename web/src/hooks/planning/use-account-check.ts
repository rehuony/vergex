import { useCallback, useEffect, useRef, useState } from 'react'
import { planningAPI } from '@/api/planning'
import { defaultMarket } from '@/utils/planning/frontier-accounts'
import type { AccountCheck } from '@/utils/planning/frontier-workspace'

const RECHECK_MS = 30000

// useAccountCheck reads the selected account's balance from the exchange, again
// every 30 seconds while the page is visible. Saved credentials alone prove
// nothing, so only a complete answer counts as verified.
export function useAccountCheck(accountID: string, venue: string) {
  const [check, setCheck] = useState<AccountCheck | null>(null)
  const mounted = useRef(true)
  const request = useRef(0)

  const verify = useCallback(
    async (background = false) => {
      const revision = ++request.current
      if (!accountID) {
        setCheck(null)
        return
      }
      setCheck((previous) =>
        background &&
        previous?.accountID === accountID &&
        previous.status === 'verified'
          ? previous
          : { accountID, status: 'checking' }
      )
      try {
        const next = await planningAPI<{
          equity: string
          available: string
          position: string
          open_orders: number
        }>(
          `account?exchange_id=${encodeURIComponent(accountID)}&symbol=${defaultMarket(venue)}`
        )
        if (
          !Number.isFinite(Number(next.equity)) ||
          !Number.isFinite(Number(next.available)) ||
          next.equity == null ||
          next.available == null ||
          String(next.equity).trim() === '' ||
          String(next.available).trim() === ''
        )
          throw new Error(
            'The exchange did not return a complete balance. Retry verification.'
          )
        if (mounted.current && revision === request.current)
          setCheck({
            ...next,
            accountID,
            status: 'verified',
            checkedAt: Date.now(),
          })
      } catch (e) {
        if (mounted.current && revision === request.current)
          setCheck({
            accountID,
            status: 'unavailable',
            error:
              e instanceof Error ? e.message : 'Could not verify the account.',
          })
      }
    },
    [accountID, venue]
  )

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout>
    const poll = async (background = false) => {
      if (!document.hidden) await verify(background)
      if (!cancelled && accountID)
        timer = setTimeout(() => void poll(true), RECHECK_MS)
    }
    void poll()
    return () => {
      cancelled = true
      clearTimeout(timer)
      request.current++
    }
  }, [verify, accountID])

  return check?.accountID === accountID ? check : null
}
