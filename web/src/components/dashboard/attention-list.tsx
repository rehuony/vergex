import { Link } from 'react-router-dom'
import { ChevronRight, TriangleAlert } from 'lucide-react'
import type { Overview, OverviewAttention } from '@/api/overview'
import { SettingsLink } from '@/components/settings/settings-link'
import { ROUTES } from '@/constants/route-paths'
import { t, type Language } from '@/i18n/translations'
import { formatRequestError } from '@/utils/request-errors'
import { traderPath } from './overview-links'

const rowClass =
  'flex min-h-11 items-center gap-3 px-4 py-2 text-[13px] hover:bg-vergex-danger/10'

function Row({
  item,
  overview,
  language,
}: {
  item: OverviewAttention
  overview: Overview
  language: Language
}) {
  const body = (
    <>
      <TriangleAlert
        className="size-4 shrink-0 text-vergex-danger"
        aria-hidden="true"
      />
      <span className="min-w-0 flex-1">
        <span className="font-medium text-vergex-text">
          {t(`dashboard.attention.${item.kind}`, language, {
            name: item.name
              ? t(`dashboard.service.${item.name}`, language) ===
                `dashboard.service.${item.name}`
                ? item.name
                : t(`dashboard.service.${item.name}`, language)
              : '',
          })}
        </span>
        {item.detail && (
          <span
            className="block truncate text-xs text-vergex-text-muted"
            title={item.detail}
          >
            {formatRequestError(item.detail)}
          </span>
        )}
      </span>
      <ChevronRight
        className="size-4 shrink-0 text-vergex-text-muted"
        aria-hidden="true"
      />
    </>
  )
  if (item.trader_id) {
    return (
      <Link
        to={traderPath(overview.traders, item.trader_id)}
        className={rowClass}
      >
        {body}
      </Link>
    )
  }
  if (item.kind.startsWith('account_')) {
    return (
      <SettingsLink section="exchanges" className={rowClass}>
        {body}
      </SettingsLink>
    )
  }
  if (item.kind === 'service_failed' && item.name === 'telegram') {
    return (
      <SettingsLink section="telegram" className={rowClass}>
        {body}
      </SettingsLink>
    )
  }
  return (
    <Link to={ROUTES.planning} className={rowClass}>
      {body}
    </Link>
  )
}

// AttentionList names what needs the user, each row leading to where to fix it.
// It takes no space when all is well.
export function AttentionList({
  overview,
  language,
}: {
  overview: Overview
  language: Language
}) {
  if (overview.attention.length === 0) return null
  return (
    <section
      aria-label={t('dashboard.needsAttention', language)}
      className="overflow-hidden rounded-xl border border-vergex-danger/30 bg-vergex-danger/5"
    >
      <ul className="divide-y divide-vergex-danger/20">
        {overview.attention.map((item, index) => (
          <li key={`${item.kind}-${item.name}-${index}`}>
            <Row item={item} overview={overview} language={language} />
          </li>
        ))}
      </ul>
    </section>
  )
}
