import { useCallback, useEffect, useRef, useState } from 'react'
import { futuresAPI } from '@/api/futures'
import { defaultMarket } from '@/utils/futures/futures-accounts'
import type { AccountCheck } from '@/utils/futures/futures-workspace'

const RECHECK_MS = 30000
const RECHECK_IN_VIEW_MS = 10000

// useAccountCheck reads the selected account's balance from the exchange, again
// every 30 seconds while the page is visible, or every 10 while the account is in
// view. Saved credentials alone prove nothing, so only a complete answer counts as
// verified.
export function useAccountCheck(
  accountID: string,
  venue: string,
  inView = false
) {
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
        const next = await futuresAPI<{
          equity: string
          available: string
          position: string
          open_orders: number
        }>(
          `account?account_id=${encodeURIComponent(accountID)}&symbol=${defaultMarket(venue)}`
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
            'The exchange did not return a complete balance. Retry verification'
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
              e instanceof Error ? e.message : 'Could not verify the account',
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
        timer = setTimeout(
          () => void poll(true),
          inView ? RECHECK_IN_VIEW_MS : RECHECK_MS
        )
    }
    void poll()
    return () => {
      cancelled = true
      clearTimeout(timer)
      request.current++
    }
  }, [verify, accountID, inView])

  return check?.accountID === accountID ? check : null
}
