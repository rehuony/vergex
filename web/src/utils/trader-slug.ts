import type { TraderInfo } from '@/types'

export function getTraderSlug(
  trader: Pick<TraderInfo, 'trader_id' | 'trader_name'>
) {
  const idPrefix = trader.trader_id.slice(0, 4)
  return `${trader.trader_name}-${idPrefix}`
}

export function findTraderBySlug(slug: string, traderList: TraderInfo[]) {
  const lastDashIndex = slug.lastIndexOf('-')
  if (lastDashIndex === -1) {
    return traderList.find((trader) => trader.trader_name === slug)
  }

  const name = slug.slice(0, lastDashIndex)
  const idPrefix = slug.slice(lastDashIndex + 1)
  return traderList.find(
    (trader) =>
      trader.trader_name === name && trader.trader_id.startsWith(idPrefix)
  )
}
