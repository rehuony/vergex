import type { PlanBatch } from './futures-workspace'
import { expired, type LiveRun, type FuturesState } from '@/api/futures'

export interface PlanCheck {
  id: string
  kind: string
  name: string
  status: string
  reason?: string
  plan_id?: string
}

// Futures lists and compares entry structures in this order.
const executionOrder = ['market', 'limit', 'ladder', 'twap']

export function executionRank(kind: string) {
  return executionOrder.indexOf(kind)
}

// marketRecords keeps the plans saved for one account and market, newest
// first and in execution order within one generation.
export function marketRecords(
  data: FuturesState,
  accountID: string,
  symbol: string,
  venue: string
) {
  return Object.values(data.records)
    .filter(
      (r) =>
        data.accounts[r.plan.id] === accountID &&
        r.plan.snapshot.symbol === symbol &&
        r.plan.snapshot.venue === venue
    )
    .sort(
      (a, b) =>
        Date.parse(b.plan.created) - Date.parse(a.plan.created) ||
        executionRank(a.plan.idea.execution.kind) -
          executionRank(b.plan.idea.execution.kind)
    )
}

// A run in one of these states reserves the account for its trade.
const reservingStates = [
  'approved',
  'starting',
  'running',
  'position_open',
  'stopping',
  'needs_attention',
]

// findBlockingRun returns the account's unsettled trade, ignoring approvals
// that expired before any order was placed.
export function findBlockingRun(
  runs: LiveRun[],
  accountID: string,
  clock: number
) {
  return runs.find(
    (r) =>
      r.account_id === accountID &&
      reservingStates.includes(r.state) &&
      !(r.state === 'approved' && expired(r.plan, clock))
  )
}

export function runSettled(state: string) {
  return ['completed', 'cancelled', 'expired'].includes(state)
}

// mergeChecks replaces checks by id and keeps their first-seen order.
export function mergeChecks(previous: PlanCheck[], incoming: PlanCheck[]) {
  const merged = new Map(previous.map((check) => [check.id, check]))
  incoming.forEach((check) => merged.set(check.id, check))
  return [...merged.values()]
}

// deferredEntries records the structures the assistant deferred until entry
// conditions are met or could not use, and drops those it later saved or reused.
export function deferredEntries(
  deferred: PlanBatch['deferred'],
  checks: PlanCheck[]
) {
  const entries = new Map((deferred || []).map((entry) => [entry.kind, entry]))
  for (const check of checks) {
    if (check.status === 'deferred' && check.reason)
      entries.set(check.kind, {
        kind: check.kind,
        reason: check.reason,
      })
    else if (check.status === 'rejected' && check.reason)
      entries.set(check.kind, {
        kind: check.kind,
        reason: check.reason,
        rejected: true,
      })
    else if (check.status === 'saved' || check.status === 'reused')
      entries.delete(check.kind)
  }
  return [...entries.values()]
}

// ladderLevels reads one "price weight" pair per line.
export function ladderLevels(text: string) {
  return text
    .split('\n')
    .filter(Boolean)
    .map((line) => {
      const [p, w] = line.trim().split(/\s+/)
      return { price: p, weight: w }
    })
}
