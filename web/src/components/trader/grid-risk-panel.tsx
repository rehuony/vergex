import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  AlertTriangle,
  Box,
  ChevronDown,
  Shield,
  TrendingUp,
} from 'lucide-react'
import { api } from '@/api'
import { cn } from '@/utils/cn'
import { t, type Language } from '@/i18n'
import { Button } from '@/components/ui/button'

interface GridRiskPanelProps {
  traderId: string
  language?: Language
  refreshInterval?: number // ms, default 5000
}

type Tone = 'good' | 'info' | 'bad' | 'muted'

const toneText: Record<Tone, string> = {
  good: 'text-vergex-success',
  info: 'text-vergex-ink',
  bad: 'text-vergex-danger',
  muted: 'text-vergex-muted',
}
const toneFill: Record<Tone, string> = {
  good: 'bg-vergex-success',
  info: 'bg-vergex-ink',
  bad: 'bg-vergex-danger',
  muted: 'bg-vergex-muted',
}
const toneTint: Record<Tone, string> = {
  good: 'bg-vergex-success/10',
  info: 'bg-vergex-ink-tint',
  bad: 'bg-vergex-danger/10',
  muted: 'bg-vergex-surface-muted',
}

const regimeTone = (regime: string): Tone =>
  (
    ({
      narrow: 'good',
      standard: 'info',
      wide: 'info',
      trending: 'info',
      volatile: 'bad',
    }) as Record<string, Tone>
  )[regime] ?? 'muted'

const breakoutTone = (level: string): Tone =>
  (
    ({ none: 'good', short: 'info', mid: 'info', long: 'bad' }) as Record<
      string,
      Tone
    >
  )[level] ?? 'muted'

const usageTone = (percent: number): Tone =>
  percent < 50 ? 'good' : percent < 80 ? 'info' : 'bad'

const formatPrice = (price: number) => {
  if (price === 0) return '-'
  if (price >= 1000)
    return price.toLocaleString('en-US', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })
  if (price >= 1) return price.toFixed(4)
  return price.toFixed(6)
}

const formatUSD = (value: number) =>
  `$${value.toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 0 })}`

function Block({
  icon,
  title,
  className,
  children,
}: {
  icon: ReactNode
  title: string
  className?: string
  children: ReactNode
}) {
  return (
    <div className={cn('rounded-md bg-vergex-canvas p-3', className)}>
      <p className="mb-2 flex items-center gap-1.5 text-xs font-medium text-vergex-muted">
        {icon}
        {title}
      </p>
      {children}
    </div>
  )
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string
  value: ReactNode
  tone?: Tone
}) {
  return (
    <div>
      <p className="text-vergex-muted">{label}</p>
      <p className={cn('font-mono', tone ? toneText[tone] : 'text-vergex-ink')}>
        {value}
      </p>
    </div>
  )
}

/** Live leverage, position, regime and liquidation state of a grid trader. */
export function GridRiskPanel({
  traderId,
  language = 'en',
  refreshInterval = 5000,
}: GridRiskPanelProps) {
  const [expanded, setExpanded] = useState(false)
  const {
    data: risk,
    error,
    isLoading,
  } = useQuery({
    queryKey: ['grid-risk', traderId],
    queryFn: () => api.getGridRiskInfo(traderId),
    refetchInterval: refreshInterval,
  })
  const text = (key: string) => t(`gridRisk.${key}`, language)
  // Server values such as "volatile" or "up" double as translation keys.
  const label = (value: string, fallback: string) => {
    const known = text(value)
    return known === `gridRisk.${value}` ? text(fallback) : known
  }

  const frame = 'border-b border-vergex-line'
  if (isLoading || error || !risk)
    return (
      <section
        className={cn(
          frame,
          'px-4 py-3 text-xs',
          error ? 'text-vergex-danger' : 'text-vergex-muted'
        )}
      >
        {isLoading
          ? text('loading')
          : error
            ? `${text('error')}: ${error instanceof Error ? error.message : String(error)}`
            : text('notRunning')}
      </section>
    )

  const regime = regimeTone(risk.regime_level)
  const usage = usageTone(risk.position_percent)
  const icon = 'size-3.5'
  return (
    <section className={frame}>
      <Button
        type="button"
        variant="ghost"
        className="h-auto w-full justify-between rounded-none px-4 py-3 text-vergex-ink"
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
      >
        <span className="flex items-center gap-2 text-[13px] font-medium">
          <Shield className="size-4 text-vergex-muted" aria-hidden="true" />
          {text('gridRisk')}
        </span>
        <span className="flex items-center gap-3 text-xs">
          <span
            className={cn(
              'rounded-md px-2 py-0.5',
              toneText[regime],
              toneTint[regime]
            )}
          >
            {label(risk.regime_level, 'standard')}
          </span>
          <span className="font-mono">
            {risk.effective_leverage.toFixed(1)}x
          </span>
          <span className={cn('font-mono', toneText[usage])}>
            {risk.position_percent.toFixed(0)}%
          </span>
          <ChevronDown
            className={cn(
              'size-4 text-vergex-muted transition-transform',
              expanded && 'rotate-180'
            )}
            aria-hidden="true"
          />
        </span>
      </Button>

      {expanded && (
        <div className="grid gap-3 px-4 pb-4 text-xs sm:grid-cols-2">
          <Block
            icon={<TrendingUp className={cn(icon, 'text-vergex-ink')} />}
            title={text('leverageInfo')}
          >
            <div className="grid grid-cols-3 gap-2">
              <Stat
                label={text('currentLeverage')}
                value={`${risk.current_leverage}x`}
              />
              <Stat
                label={text('effectiveLeverage')}
                value={`${risk.effective_leverage.toFixed(2)}x`}
                tone="info"
              />
              <Stat
                label={text('recommendedLeverage')}
                value={`${risk.recommended_leverage}x`}
                tone={
                  risk.current_leverage > risk.recommended_leverage
                    ? 'bad'
                    : 'good'
                }
              />
            </div>
          </Block>

          <Block
            icon={<Activity className={cn(icon, 'text-vergex-ink')} />}
            title={text('positionInfo')}
          >
            <div className="grid grid-cols-3 gap-2">
              <Stat
                label={text('currentPosition')}
                value={formatUSD(risk.current_position)}
              />
              <Stat
                label={text('maxPosition')}
                value={formatUSD(risk.max_position)}
              />
              <Stat
                label={text('positionPercent')}
                value={`${risk.position_percent.toFixed(1)}%`}
                tone={usage}
              />
            </div>
            <div className="mt-2 h-1 overflow-hidden rounded-full bg-vergex-line">
              <div
                className={cn('h-full rounded-full', toneFill[usage])}
                style={{ width: `${Math.min(risk.position_percent, 100)}%` }}
              />
            </div>
          </Block>

          <Block
            icon={<Shield className={cn(icon, 'text-vergex-ink')} />}
            title={text('marketState')}
          >
            <div className="grid grid-cols-2 gap-2">
              <Stat
                label={text('regimeLevel')}
                value={label(risk.regime_level, 'standard')}
                tone={regime}
              />
              <Stat
                label={text('currentPrice')}
                value={formatPrice(risk.current_price)}
              />
              <Stat
                label={text('breakoutLevel')}
                value={label(risk.breakout_level, 'none')}
                tone={breakoutTone(risk.breakout_level)}
              />
              <Stat
                label={text('breakoutDirection')}
                value={
                  risk.breakout_direction
                    ? label(risk.breakout_direction, 'none')
                    : '-'
                }
                tone={
                  risk.breakout_direction === 'up'
                    ? 'good'
                    : risk.breakout_direction === 'down'
                      ? 'bad'
                      : 'muted'
                }
              />
            </div>
          </Block>

          <Block
            icon={<AlertTriangle className={cn(icon, 'text-vergex-danger')} />}
            title={text('liquidationInfo')}
          >
            <div className="grid grid-cols-2 gap-2">
              <Stat
                label={text('liquidationPrice')}
                value={
                  risk.liquidation_price > 0
                    ? formatPrice(risk.liquidation_price)
                    : '-'
                }
                tone="bad"
              />
              <Stat
                label={text('liquidationDistance')}
                value={
                  risk.liquidation_distance > 0
                    ? `${risk.liquidation_distance.toFixed(1)}%`
                    : '-'
                }
                tone="bad"
              />
            </div>
          </Block>

          <Block
            icon={<Box className={cn(icon, 'text-vergex-ink')} />}
            title={text('boxState')}
            className="sm:col-span-2"
          >
            <div className="grid gap-2 sm:grid-cols-3">
              {(
                [
                  ['shortBox', risk.short_box_lower, risk.short_box_upper],
                  ['midBox', risk.mid_box_lower, risk.mid_box_upper],
                  ['longBox', risk.long_box_lower, risk.long_box_upper],
                ] as const
              ).map(([key, lower, upper]) => (
                <div key={key} className="flex justify-between gap-2">
                  <span className="text-vergex-muted">{text(key)}</span>
                  <span className="font-mono text-vergex-ink">
                    {formatPrice(lower)} - {formatPrice(upper)}
                  </span>
                </div>
              ))}
            </div>
          </Block>
        </div>
      )}
    </section>
  )
}
