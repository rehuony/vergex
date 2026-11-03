import { expect, it } from 'vitest'
import { validationSummary } from '@/utils/futures/futures-presentation'

it('explains rounded ratio failures without implying an equal ratio failed', () => {
  expect(validationSummary('net reward/risk 1.999999 is below minimum 2')).toBe(
    'Net R:R is just below 2.00 : 1 after costs'
  )
  expect(validationSummary('net reward/risk 1.246912 is below minimum 2')).toBe(
    'Net R:R 1.25 : 1 is below the required 2.00 : 1 after costs'
  )
  expect(validationSummary('invalid stop/target ordering')).toBe(
    'invalid stop/target ordering'
  )
})

it('keeps immutable chart plan references when only execution timestamps refresh', async () => {
  const { preserveFuturesPlans } =
    await import('@/utils/futures/futures-presentation')
  const previous = {
    records: {
      plan: { plan: { id: 'plan', hash: 'fixed' }, status: 'approved' },
    },
    runs: [
      {
        id: 'run',
        plan: { id: 'plan', hash: 'fixed' },
        status: 'needs_attention',
        updated_at: 'before',
      },
    ],
    accounts: {},
    mode: 'live',
    now: 'before',
  } as unknown as import('@/api/futures').FuturesState
  const incoming = JSON.parse(JSON.stringify(previous))
  incoming.now = 'after'
  incoming.runs[0].updated_at = 'after'
  const result = preserveFuturesPlans(previous, incoming)
  expect(result.records).toBe(previous.records)
  expect(result.runs[0].plan).toBe(previous.runs[0].plan)
  expect(result.runs[0].updated_at).toBe('after')
  incoming.records.plan.status = 'completed'
  expect(preserveFuturesPlans(previous, incoming).records).not.toBe(
    previous.records
  )
})
