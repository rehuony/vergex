import { Link, useNavigate } from 'react-router-dom'
import { t, type Language } from '@/i18n/translations'
import {
  getExchangeDisplayName,
  getModelDisplayName,
} from '@/constants/model-providers'
import { buildTraderPath } from '@/constants/route-paths'
import { Panel, PanelState } from '@/components/common/panel'
import { SettingsLink } from '@/components/settings/settings-link'
import type { Exchange, TraderInfo, TraderMetrics } from '@/types'
import { cn } from '@/utils/cn'
import { getTraderSlug } from '@/utils/trader-slug'
import { fmtPct, fmtUsd } from './trader-format'

const headClass = 'px-4 py-3 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-4 py-3 whitespace-nowrap'
const numberClass = 'w-28 text-right tabular-nums'

interface TradersListProps {
  traders?: TraderInfo[]
  metrics?: Record<string, TraderMetrics>
  failed: boolean
  exchanges: Exchange[]
  modelCount: number
  exchangeCount: number
  language: Language
}

// The trader table. A row opens that trader, where it is started, stopped,
// edited and deleted. Numbers come from stored data.
export function TradersList({
  traders,
  metrics,
  failed,
  exchanges,
  modelCount,
  exchangeCount,
  language,
}: TradersListProps) {
  const navigate = useNavigate()
  const label = (key: string) => t(`traderList.${key}`, language)

  if (!traders) {
    return (
      <Panel>
        {failed ? (
          <PanelState role="alert">{label('loadFailed')}</PanelState>
        ) : (
          <PanelState role="status">{t('loading', language)}</PanelState>
        )}
      </Panel>
    )
  }

  if (traders.length === 0) {
    return (
      <Panel>
        <PanelState role="status">
          <p className="font-medium text-vergex-text">
            {t('noTraders', language)}
          </p>
          <p>{t('createFirstTrader', language)}</p>
          {modelCount === 0 && (
            <SettingsLink section="models" className="text-vergex-brand">
              {t('configureModelsFirst', language)}
            </SettingsLink>
          )}
          {exchangeCount === 0 && (
            <SettingsLink section="exchanges" className="text-vergex-brand">
              {t('configureExchangesFirst', language)}
            </SettingsLink>
          )}
        </PanelState>
      </Panel>
    )
  }

  return (
    <Panel className="overflow-hidden">
      <table className="w-full text-left text-[13px]">
        <thead>
          <tr>
            <th className={headClass}>{label('name')}</th>
            <th className={cn(headClass, numberClass)}>{label('equity')}</th>
            <th className={cn(headClass, numberClass)}>{label('return')}</th>
            <th className={cn(headClass, numberClass, 'max-md:hidden')}>
              {label('winRate')}
            </th>
            <th className={cn(headClass, numberClass, 'max-md:hidden')}>
              {label('drawdown')}
            </th>
            <th className={cn(headClass, numberClass, 'max-md:hidden')}>
              {label('trades')}
            </th>
          </tr>
        </thead>
        <tbody>
          {traders.map((trader) => {
            const row = metrics?.[trader.trader_id]
            const path = buildTraderPath(getTraderSlug(trader))
            const change = row?.return_pct
            return (
              <tr
                key={trader.trader_id}
                className="cursor-pointer"
                onClick={() => navigate(path)}
              >
                <td className={cn(cellClass, 'max-w-0 w-full')}>
                  <div className="flex min-w-0 items-center gap-2">
                    <span
                      className={cn(
                        'size-2 shrink-0 rounded-full',
                        trader.is_running
                          ? 'bg-vergex-success'
                          : 'bg-[var(--line-strong)]'
                      )}
                      role="img"
                      aria-label={
                        trader.is_running ? label('running') : label('stopped')
                      }
                    />
                    <Link
                      to={path}
                      className="truncate text-sm font-medium text-vergex-text hover:underline"
                      title={trader.trader_name}
                      onClick={(event) => event.stopPropagation()}
                    >
                      {trader.trader_name}
                    </Link>
                  </div>
                  <div className="mt-0.5 truncate pl-4 text-xs text-vergex-text-muted">
                    {getModelDisplayName(
                      trader.ai_model.split('_').pop() || trader.ai_model
                    )}{' '}
                    · {getExchangeDisplayName(trader.exchange_id, exchanges)}
                  </div>
                </td>
                <td className={cn(cellClass, numberClass)}>
                  {fmtUsd(row?.equity)}
                </td>
                <td
                  className={cn(
                    cellClass,
                    numberClass,
                    change == null
                      ? ''
                      : change >= 0
                        ? 'text-vergex-success'
                        : 'text-vergex-danger'
                  )}
                >
                  {fmtPct(change)}
                </td>
                <td className={cn(cellClass, numberClass, 'max-md:hidden')}>
                  {row?.win_rate == null ? '—' : `${row.win_rate.toFixed(1)}%`}
                </td>
                <td className={cn(cellClass, numberClass, 'max-md:hidden')}>
                  {row?.max_drawdown_pct == null
                    ? '—'
                    : `${row.max_drawdown_pct.toFixed(1)}%`}
                </td>
                <td className={cn(cellClass, numberClass, 'max-md:hidden')}>
                  {row?.total_trades ?? '—'}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </Panel>
  )
}
