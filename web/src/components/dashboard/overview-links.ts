import type { OverviewTrader } from '@/api/dashboard'
import { buildTraderPath, ROUTES } from '@/constants/route-paths'

// Where the dashboard sends each row: the page where it can be acted on.
export function traderPath(
  traders: OverviewTrader[],
  traderId: string | undefined,
  section = ''
) {
  const trader = traders.find((item) => item.trader_id === traderId)
  if (!trader) return ROUTES.futures
  return buildTraderPath(trader.trader_id, section)
}
