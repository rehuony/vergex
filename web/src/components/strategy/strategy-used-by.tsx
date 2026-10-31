import { Link } from 'react-router-dom'
import { buildTraderPath } from '@/constants/route-paths'
import { t, type Language } from '@/i18n/translations'
import type { TraderInfo } from '@/types'
import { getTraderSlug } from '@/utils/trader-slug'

// How many traders are named in the row before the rest become a count.
const NAMED_TRADERS = 3

// The traders running on a strategy, each linking to its page. Saving a strategy
// changes what they do, so this stays beside the form.
export function StrategyUsedBy({
  traders,
  language,
}: {
  traders: TraderInfo[]
  language: Language
}) {
  if (traders.length === 0) {
    return (
      <p className="text-sm text-vergex-text-muted">
        {t('strategy.noTraders', language)}
      </p>
    )
  }
  const hidden = traders.length - NAMED_TRADERS
  return (
    <p className="truncate text-sm text-vergex-text-muted">
      {traders.slice(0, NAMED_TRADERS).map((trader, index) => (
        <span key={trader.trader_id}>
          {index > 0 && ', '}
          <Link
            to={buildTraderPath(getTraderSlug(trader))}
            title={trader.trader_name}
            className="text-vergex-text hover:underline"
          >
            {trader.trader_name}
          </Link>
        </span>
      ))}
      {hidden > 0 &&
        ` ${t('strategy.moreTraders', language, { count: hidden })}`}
    </p>
  )
}
