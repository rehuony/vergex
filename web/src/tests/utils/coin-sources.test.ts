import { describe, expect, it } from 'vitest'
import {
  COIN_SOURCE_TYPES,
  RANKED_SOURCES,
  coinSourceLabel,
} from '@/utils/coin-sources'

describe('coin source labels', () => {
  it('labels every supported source and falls back to the raw value', () => {
    expect(coinSourceLabel('oi_top', 'en')).toBe('OI increase')
    expect(coinSourceLabel('vergex_signal', 'en')).toBe('vergex_signal')
    for (const type of COIN_SOURCE_TYPES) {
      expect(coinSourceLabel(type, 'en')).not.toContain('traderSettings.')
    }
  })

  it('lists every ranking source exactly once', () => {
    const ranked = RANKED_SOURCES.map((s) => s.type).sort()
    const all = COIN_SOURCE_TYPES.filter(
      (t) => t !== 'static' && t !== 'mixed'
    ).sort()
    expect(ranked).toEqual(all)
  })
})
