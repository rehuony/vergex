import {
  defaultLimits,
  type Constraints,
  type Plan,
  type FuturesState,
} from '@/api/futures'
import type { AccountPosition } from './futures-account'

export interface AccountObservation {
  accountID: string
  symbol: string
  positions: AccountPosition[]
  orders:
    | { type: string; status: string; quantity: number; side: string }[]
    | null
  checkedAt: number
}

const LIMITS_KEY = 'vergex.futures.limits'
const SAVED_LIMITS = [
  'equity',
  'max_risk',
  'max_notional',
  'max_risk_pct',
  'min_reward_risk',
  'fee_bps',
  'slippage_bps',
] as const

// The limits set by hand, kept on this browser. What the server sized for a plan stays out
// of it, so an account's numbers are never carried over to another.
export function savedLimits(): Constraints {
  const limits = { ...defaultLimits }
  try {
    const saved = JSON.parse(localStorage.getItem(LIMITS_KEY) || '{}')
    for (const key of SAVED_LIMITS) {
      const value = saved?.[key]
      if (typeof value === 'string' && Number(value) >= 0) limits[key] = value
    }
  } catch {
    /* Only a convenience: planning starts from the defaults. */
  }
  return limits
}

export function saveLimits(limits: Constraints) {
  try {
    localStorage.setItem(
      LIMITS_KEY,
      JSON.stringify(
        Object.fromEntries(SAVED_LIMITS.map((key) => [key, limits[key]]))
      )
    )
  } catch {
    /* Only a convenience: the next visit starts from the defaults. */
  }
}

export interface PlanBatch {
  id: string
  ids: string[]
  created: string
  waiting?: boolean
  // The ways of entering that left no plan, and why: put off, or tried and refused.
  deferred?: { kind: string; reason: string; rejected?: boolean }[]
}

export function workspaceKey(account: string, venue: string, symbol: string) {
  // Scope browser presentation preferences to the signed-in user as well as market.
  let user = ''
  try {
    user = localStorage.getItem('user_id') || ''
  } catch {
    /* Private browsing may disable storage. */
  }
  return ['futures-batch-v1', user, account, venue, symbol].join('/')
}

export function readBatch(key: string): PlanBatch | null {
  try {
    const batch = JSON.parse(localStorage.getItem(key) || 'null')
    return batch &&
      typeof batch.id === 'string' &&
      typeof batch.created === 'string' &&
      (batch.waiting === undefined || typeof batch.waiting === 'boolean') &&
      (batch.deferred === undefined ||
        (Array.isArray(batch.deferred) &&
          batch.deferred.length <= 4 &&
          batch.deferred.every(
            (entry: { kind?: unknown; reason?: unknown } | null) =>
              entry &&
              ['market', 'limit', 'ladder', 'twap'].includes(
                String(entry.kind)
              ) &&
              typeof entry.reason === 'string' &&
              entry.reason.length <= 2000
          ))) &&
      Array.isArray(batch.ids) &&
      batch.ids.every((id: unknown) => typeof id === 'string')
      ? batch
      : null
  } catch {
    return null
  }
}

export function saveBatch(key: string, batch: PlanBatch) {
  try {
    localStorage.setItem(key, JSON.stringify(batch))
  } catch {
    /* Storage may be unavailable. */
  }
}

// Never infer a generation batch from time proximity or entry type. With no batch
// remembered on this browser, show one saved idea instead of mixing rounds.
export function batchPlans(
  records: FuturesState['records'][string][],
  batch: PlanBatch | null
): Plan[] {
  const drafts = records.filter((record) => record.status === 'draft')
  if (!batch) return drafts.length ? [drafts[0].plan] : []
  const found = batch.ids.flatMap((id) => {
    const record = drafts.find((row) => row.plan.id === id)
    return record ? [record.plan] : []
  })
  return ['market', 'limit', 'ladder', 'twap'].flatMap((kind) =>
    found.filter((plan) => plan.idea.execution.kind === kind)
  )
}

export interface AccountCheck {
  accountID: string
  status: 'checking' | 'verified' | 'unavailable'
  equity?: string
  available?: string
  position?: string
  open_orders?: number
  error?: string
  checkedAt?: number
}
