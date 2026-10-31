import { Link } from 'react-router-dom'
import { ChevronRight, TriangleAlert } from 'lucide-react'
import type { Overview, OverviewAttention } from '@/api/overview'
import { SettingsLink } from '@/components/settings/settings-link'
import { ScrollArea } from '@/components/ui/scroll-area'
import { buildFrontierPath } from '@/constants/route-paths'
import { t, type Language } from '@/i18n/translations'
import { formatRequestError } from '@/utils/request-errors'
import { traderPath } from './overview-links'

const rowClass =
  'flex min-h-11 items-center gap-3 px-4 py-2 text-[13px] hover:bg-vergex-danger/10'

function serviceName(name: string, language: Language) {
  const key = `dashboard.service.${name}`
  const translated = t(key, language)
  return translated === key ? name : translated
}

// One row per kind of problem: ten traders blocked at once are one thing to look
// at, and the row leads to the first of them.
function mergeByKind(items: OverviewAttention[], language: Language) {
  const rows = new Map<string, { item: OverviewAttention; more: number }>()
  items.forEach((item) => {
    const row = rows.get(item.kind)
    if (row) row.more++
    else rows.set(item.kind, { item, more: 0 })
  })
  return [...rows.values()].map(({ item, more }) => ({
    item,
    name: [
      item.name ? serviceName(item.name, language) : '',
      more ? `+${more}` : '',
    ]
      .filter(Boolean)
      .join(' '),
    more,
  }))
}

function Row({
  item,
  name,
  more,
  overview,
  language,
}: {
  item: OverviewAttention
  name: string
  more: number
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
          {t(`dashboard.attention.${item.kind}`, language, { name })}
        </span>
        {item.detail && more === 0 && (
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
    <Link
      to={buildFrontierPath({ plan: item.plan_id, tab: 'positions' })}
      className={rowClass}
    >
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
      <ScrollArea className="max-h-36">
        <ul className="divide-y divide-vergex-danger/20">
          {mergeByKind(overview.attention, language).map((row) => (
            <li key={row.item.kind}>
              <Row {...row} overview={overview} language={language} />
            </li>
          ))}
        </ul>
      </ScrollArea>
    </section>
  )
}
