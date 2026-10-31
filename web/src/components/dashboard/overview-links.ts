import type { OverviewTrader } from '@/api/overview'
import { buildTraderPath, ROUTES } from '@/constants/route-paths'
import { getTraderSlug } from '@/utils/trader-slug'

// Where the dashboard sends each row: the page where it can be acted on.
export function traderPath(
  traders: OverviewTrader[],
  traderId: string | undefined,
  section = ''
) {
  const trader = traders.find((item) => item.trader_id === traderId)
  if (!trader) return ROUTES.planning
  const base = buildTraderPath(getTraderSlug(trader))
  return section ? `${base}/${section}` : base
}
