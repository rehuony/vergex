import { beforeEach, expect, it } from 'vitest'
import type { Plan, FuturesState } from '@/api/futures'
import { deferredEntries } from '@/utils/futures/futures-plans'
import {
  batchPlans,
  readBatch,
  saveBatch,
  workspaceKey,
} from '@/utils/futures/futures-workspace'

const make = (id: string, kind: string, equity = '1000') =>
  ({
    state: 'draft',
    plan: {
      id,
      idea: { execution: { kind } },
      constraints: { equity },
      expires: new Date(Date.now() + 60000).toISOString(),
    } as Plan,
  }) as FuturesState['records'][string]
beforeEach(() => localStorage.clear())
it('does not assemble a comparison from different historical generations', () => {
  const records = [
    make('new-market', 'market'),
    make('old-limit', 'limit'),
    make('old-twap', 'twap'),
  ]
  expect(batchPlans(records, null).map((p) => p.id)).toEqual(['new-market'])
  expect(
    batchPlans(records, {
      id: 'new',
      ids: ['new-market', 'not-saved'],
      created: 'now',
    }).map((p) => p.id)
  ).toEqual(['new-market'])
})
it('restores only the saved batch for the same user, account and market', () => {
  localStorage.setItem('user_id', 'alice')
  const key = workspaceKey('account-a', 'binance', 'BTCUSDT')
  const batch = { id: 'turn', ids: ['a', 'b'], created: 'now' }
  saveBatch(key, batch)
  expect(readBatch(key)).toEqual(batch)
  expect(readBatch(workspaceKey('account-b', 'binance', 'BTCUSDT'))).toBeNull()
  localStorage.setItem('user_id', 'bob')
  expect(readBatch(workspaceKey('account-a', 'binance', 'BTCUSDT'))).toBeNull()
  localStorage.setItem(key, '{bad')
  expect(readBatch(key)).toBeNull()
})
it('keeps why a way of entering left no plan until it leaves one', () => {
  const check = (kind: string, status: string, reason?: string) => ({
    id: kind,
    name: kind,
    kind,
    status,
    reason,
  })
  const entries = deferredEntries(undefined, [
    check('market', 'deferred', 'Extended'),
    check('ladder', 'rejected', 'Below minimum'),
    check('twap', 'rejected', 'Too thin'),
    check('twap', 'saved'),
  ])
  expect(entries).toEqual([
    { kind: 'market', reason: 'Extended' },
    { kind: 'ladder', reason: 'Below minimum', rejected: true },
  ])
})
it('lists every plan of a generation in execution order', () => {
  const records = [
    make('twap', 'twap'),
    make('limit-high', 'limit'),
    make('limit-low', 'limit'),
    make('market', 'market'),
  ]
  expect(
    batchPlans(records, {
      id: 'turn',
      ids: ['twap', 'limit-high', 'limit-low', 'market'],
      created: 'now',
    }).map((p) => p.id)
  ).toEqual(['market', 'limit-high', 'limit-low', 'twap'])
})
