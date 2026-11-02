import { useState, type Dispatch, type SetStateAction } from 'react'
import {
  type Constraints,
  type Idea,
  type LiveRun,
  type Plan,
  futuresAPI,
} from '@/api/futures'
import { nativeMarket } from '@/utils/futures/futures-account'
import type { AccountObservation } from '@/utils/futures/futures-workspace'
import { ladderLevels, runSettled } from '@/utils/futures/futures-plans'

interface FuturesTradesOptions {
  busy: boolean
  setBusy: Dispatch<SetStateAction<boolean>>
  setError: Dispatch<SetStateAction<string>>
  setOrderNotice: Dispatch<SetStateAction<string>>
  accountId: string
  venue: string
  symbol: string
  leverage: number
  limits: Constraints
  setLimits: Dispatch<SetStateAction<Constraints>>
  observation: AccountObservation | null
  clock: number
  blockingRun: LiveRun | undefined
  reload: () => Promise<void>
  setSelected: Dispatch<SetStateAction<string>>
  setCompare: Dispatch<SetStateAction<string[]>>
  onShowAccount: () => void
  // A plan was compiled afresh for its review.
  onPlanSized: (old: string, sized: string) => void
}

// useFuturesTrades owns the steps from a plan to live orders: sizing a plan
// against the account for review, approval and execution, actions on a
// running trade, and the manual entry builder.
export function useFuturesTrades({
  busy,
  setBusy,
  setError,
  setOrderNotice,
  accountId,
  venue,
  symbol,
  leverage,
  limits,
  setLimits,
  observation,
  clock,
  blockingRun,
  reload,
  setSelected,
  setCompare,
  onShowAccount,
  onPlanSized,
}: FuturesTradesOptions) {
  const [tradeCheckError, setTradeCheckError] = useState<{
    runID: string
    message: string
  } | null>(null)
  const [reviewReplacement, setReviewReplacement] = useState<LiveRun | null>(
    null
  )
  const [review, setReview] = useState<Plan | null>(null),
    [builder, setBuilder] = useState(false),
    [builderIdea, setBuilderIdea] = useState<Idea>({
      name: 'Custom plan',
      side: 'long',
      entry: '',
      stop: '',
      target: '',
      rationale:
        'User-defined conditional prices, subject to mathematical validation; neither approved nor executed',
      execution: { kind: 'limit' },
    }),
    [levels, setLevels] = useState('')
  async function reviewWithAccount(plan: Plan) {
    if (busy) return
    setBusy(true)
    setError('')
    setOrderNotice('')
    setTradeCheckError(null)
    setReviewReplacement(null)
    let checkingPreviousTrade = false
    let replacement: LiveRun | null = null
    try {
      const existingPosition =
        observation?.accountID === accountId &&
        Math.abs(clock - observation.checkedAt) < 30000
          ? observation.positions.find(
              (p) =>
                nativeMarket(p.symbol, venue) ===
                nativeMarket(blockingRun?.plan.snapshot.symbol || symbol, venue)
            )
          : undefined
      if (blockingRun && existingPosition) {
        onShowAccount()
        return
      }
      if (blockingRun?.state === 'approved') {
        replacement = blockingRun
      } else if (blockingRun) {
        checkingPreviousTrade = true
        const checked = await futuresAPI<LiveRun>('check-status', {
          id: blockingRun.id,
        })
        await reload()
        if (checked.state === 'approved') {
          replacement = checked
        } else if (!runSettled(checked.state)) {
          throw new Error(
            'The previous trade is still active. Review Positions and orders before placing another'
          )
        }
        checkingPreviousTrade = false
      }
      const sized = await futuresAPI<Plan>('compile', {
        account_id: accountId,
        venue,
        symbol,
        idea: plan.idea,
        constraints: {
          ...plan.constraints,
          leverage,
          min_reward_risk: limits.min_reward_risk,
        },
        auto_size: true,
      })
      await reload()
      // A refreshed account size is a new review, never a replacement in an older comparison set.
      onPlanSized(plan.id, sized.id)
      setSelected(sized.id)
      setCompare([])
      setLimits(sized.constraints)
      setReviewReplacement(replacement)
      setReview(sized)
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e)
      if (checkingPreviousTrade && blockingRun) {
        setTradeCheckError({ runID: blockingRun.id, message })
        await reload().catch(() => {})
      } else setError(message)
    } finally {
      setBusy(false)
    }
  }
  async function approveAndExecute() {
    if (!review || busy) return
    const locked = review
    setBusy(true)
    setError('')
    setOrderNotice('')
    try {
      const approved = await futuresAPI<LiveRun>('approve', {
        plan_id: locked.id,
        hash: locked.hash,
        confirmed: true,
        ...(reviewReplacement
          ? {
              replace_approval_id: reviewReplacement.id,
              replace_approval_hash: reviewReplacement.approval_hash,
            }
          : {}),
      })
      setReview(null)
      await futuresAPI('execute', {
        id: approved.id,
        hash: approved.approval_hash,
      })
      await reload()
    } catch (e) {
      setReview(null)
      setError(e instanceof Error ? e.message : String(e))
      await reload().catch(() => {})
    } finally {
      setBusy(false)
    }
  }
  async function checkPreviousTrade(r: LiveRun) {
    setBusy(true)
    setError('')
    setOrderNotice('')
    setTradeCheckError(null)
    try {
      const checked = await futuresAPI<LiveRun>('check-status', { id: r.id })
      await reload()
      setOrderNotice(
        checked.state === 'completed'
          ? 'Previous trade is closed. Your account is ready for a new order. Review a plan when you are ready'
          : ''
      )
    } catch (e) {
      setTradeCheckError({
        runID: r.id,
        message: e instanceof Error ? e.message : String(e),
      })
      await reload().catch(() => {})
    } finally {
      setBusy(false)
    }
  }
  async function runAction(
    action: 'execute' | 'stop' | 'reconcile',
    r: LiveRun
  ) {
    setBusy(true)
    setError('')
    setOrderNotice('')
    try {
      await futuresAPI(action, { id: r.id, hash: r.approval_hash })
      await reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  async function saveBuilder() {
    setBusy(true)
    setError('')
    setReviewReplacement(blockingRun?.state === 'approved' ? blockingRun : null)
    try {
      const idea = { ...builderIdea, execution: { ...builderIdea.execution } }
      if (idea.execution.kind === 'ladder') {
        idea.execution.levels = ladderLevels(levels)
      }
      const p = await futuresAPI<Plan>('compile', {
        account_id: accountId,
        venue,
        symbol,
        idea,
        constraints: { ...limits, leverage },
        auto_size: Boolean(accountId),
      })
      setBuilder(false)
      await reload()
      setSelected(p.id)
      setCompare([])
      setReview(p)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  return {
    tradeCheckError,
    setTradeCheckError,
    reviewReplacement,
    review,
    setReview,
    builder,
    setBuilder,
    builderIdea,
    setBuilderIdea,
    levels,
    setLevels,
    reviewWithAccount,
    approveAndExecute,
    checkPreviousTrade,
    runAction,
    saveBuilder,
  }
}
