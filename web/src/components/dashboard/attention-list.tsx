import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronRight, TriangleAlert } from 'lucide-react'
import type { Overview, OverviewAttention } from '@/api/dashboard'
import { SettingsLink } from '@/components/settings/settings-link'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { buildFuturesPath } from '@/constants/route-paths'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import { formatRequestError } from '@/utils/request-errors'
import { traderPath } from './overview-links'

const cellClass =
  'flex h-full w-full flex-col justify-center gap-1 px-4 py-3 text-left'

const rowClass =
  'flex min-h-11 items-center gap-3 px-4 py-2 text-[13px] hover:bg-vergex-surface-muted hover:duration-0'

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
      <span className="min-w-0 flex-1">
        <span className="font-medium text-vergex-ink">
          {t(`dashboard.attention.${item.kind}`, language, { name })}
        </span>
        {item.detail && more === 0 && (
          <span
            className="block truncate text-xs text-vergex-muted"
            title={item.detail}
          >
            {formatRequestError(item.detail)}
          </span>
        )}
      </span>
      <ChevronRight
        className="size-4 shrink-0 text-vergex-muted"
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
      <SettingsLink section="accounts" className={rowClass}>
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
      to={buildFuturesPath({ plan: item.plan_id, tab: 'positions' })}
      className={rowClass}
    >
      {body}
    </Link>
  )
}

// AttentionList is the last of the Dashboard's figures, always there, saying whether
// anything needs the user. With problems it opens a list of them, each row leading to
// where to fix it. Nothing on the page moves either way.
export function AttentionList({
  overview,
  language,
}: {
  /** Undefined while it is first read. */
  overview: Overview | undefined
  language: Language
}) {
  const [open, setOpen] = useState(false)
  const count = overview?.attention.length ?? 0
  const title = t('dashboard.needsAttention', language)
  const name = (
    <span className="block whitespace-nowrap text-xs text-vergex-muted">
      {title}
    </span>
  )
  if (!overview || count === 0) {
    return (
      <div className={cellClass}>
        {name}
        <span className="text-[13px] text-vergex-ink">
          {overview ? (
            t('dashboard.attentionNone', language)
          ) : (
            <Skeleton className="w-12" />
          )}
        </span>
      </div>
    )
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        className={cn(
          cellClass,
          'hover:bg-vergex-surface-muted hover:duration-0 data-[state=open]:bg-vergex-surface-muted'
        )}
      >
        {name}
        <span className="inline-flex items-center gap-1 whitespace-nowrap text-[13px] font-normal tabular-nums text-vergex-warning">
          <TriangleAlert className="size-3 shrink-0" aria-hidden="true" />
          {t('dashboard.attentionCount', language, { count })}
        </span>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        collisionPadding={16}
        aria-label={title}
        className="w-[360px] max-w-[calc(100vw-32px)] overflow-hidden"
      >
        <ScrollArea className="max-h-72">
          {/* A row opens a page or Settings, which belongs in front of this. */}
          <ul
            className="divide-y divide-vergex-line"
            onClick={() => setOpen(false)}
          >
            {mergeByKind(overview.attention, language).map((row) => (
              <li key={row.item.kind}>
                <Row {...row} overview={overview} language={language} />
              </li>
            ))}
          </ul>
        </ScrollArea>
      </PopoverContent>
    </Popover>
  )
}
