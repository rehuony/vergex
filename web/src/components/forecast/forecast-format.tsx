import { ArrowDown, ArrowUp, Minus } from 'lucide-react'
import type { ForecastDirection } from '@/api/forecast'
import { activeLanguage, t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'

export const baseAsset = (symbol: string) => symbol.replace(/USDT$/, '')

export const fmtPrice = (value: number | null | undefined) =>
  value === null || value === undefined || !Number.isFinite(value)
    ? '—'
    : value.toLocaleString('en-US', {
        maximumFractionDigits:
          value >= 1000 ? 1 : value >= 1 ? 3 : value >= 0.01 ? 5 : 8,
      })

export const fmtPct = (value: number, signed = false) =>
  `${signed && value > 0 ? '+' : ''}${value.toFixed(2)}%`

export const fmtTime = (ms: number) =>
  new Date(ms).toLocaleString(activeLanguage(), {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })

export const fmtDate = (ms: number) =>
  new Date(ms).toLocaleDateString(activeLanguage(), {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })

// A score stays a plain number until enough verified forecasts back it as a
// probability.
export const scoreText = (prob: number, calibrated: boolean) => {
  const score = Math.round(prob * 100)
  return calibrated ? `${score}%` : String(score)
}

const directionTone = (direction: ForecastDirection) =>
  direction === 'up'
    ? 'text-vergex-success'
    : direction === 'down'
      ? 'text-vergex-danger'
      : 'text-vergex-text-muted'

export function DirectionIcon({ direction }: { direction: ForecastDirection }) {
  const Icon =
    direction === 'up' ? ArrowUp : direction === 'down' ? ArrowDown : Minus
  return (
    <Icon
      className={cn('size-3.5 shrink-0', directionTone(direction))}
      aria-hidden="true"
    />
  )
}

export function DirectionLabel({
  direction,
  language,
  className,
}: {
  direction: ForecastDirection
  language: Language
  className?: string
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 whitespace-nowrap',
        directionTone(direction),
        className
      )}
    >
      <DirectionIcon direction={direction} />
      {t(`forecast.direction.${direction}`, language)}
    </span>
  )
}
