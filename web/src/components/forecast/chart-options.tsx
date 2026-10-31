import { useState } from 'react'
import { SlidersHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Switch } from '@/components/ui/switch'
import { t, type Language } from '@/i18n/translations'

export interface ChartOptions {
  levels: boolean
  hits: boolean
  projection: boolean
  ema: boolean
  bollinger: boolean
  vwap: boolean
  rsi: boolean
}

const DEFAULTS: ChartOptions = {
  levels: true,
  hits: true,
  projection: true,
  ema: false,
  bollinger: false,
  vwap: false,
  rsi: false,
}

const STORAGE_KEY = 'vergex.forecast.chart'

function load(): ChartOptions {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    return saved ? { ...DEFAULTS, ...JSON.parse(saved) } : DEFAULTS
  } catch {
    return DEFAULTS
  }
}

// What the chart draws, remembered on this browser.
export function useChartOptions() {
  const [options, setOptions] = useState(load)
  const change = (next: Partial<ChartOptions>) => {
    const merged = { ...options, ...next }
    setOptions(merged)
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(merged))
    } catch {
      /* Only a convenience: it just won't be remembered. */
    }
  }
  return [options, change] as const
}

const wantsIndicators = (options: ChartOptions) =>
  options.ema || options.bollinger || options.vwap || options.rsi

export { wantsIndicators }

const OVERLAYS: (keyof ChartOptions)[] = ['levels', 'hits', 'projection']
const INDICATORS: (keyof ChartOptions)[] = ['ema', 'bollinger', 'vwap', 'rsi']

// The menu of what the chart draws, kept out of the toolbar so it stays one line.
export function ChartOptionsMenu({
  options,
  onChange,
  language,
}: {
  options: ChartOptions
  onChange: (next: Partial<ChartOptions>) => void
  language: Language
}) {
  const label = (key: string) => t(`forecast.chart.${key}`, language)
  const row = (name: keyof ChartOptions) => (
    <div
      key={name}
      className="flex min-h-9 items-center justify-between gap-6 px-2 text-sm text-vergex-text"
    >
      {label(name)}
      <Switch
        aria-label={label(name)}
        checked={options[name]}
        onChange={(checked) => onChange({ [name]: checked })}
      />
    </div>
  )
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label={label('menu')}
          className="max-sm:h-10"
        >
          <SlidersHorizontal className="size-4" />
          <span className="max-sm:hidden">{label('menu')}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64 p-2">
        {OVERLAYS.map(row)}
        <p className="mt-1 border-t border-[var(--line)] px-2 pb-1 pt-2 text-xs text-vergex-text-muted">
          {label('indicators')}
        </p>
        {INDICATORS.map(row)}
      </PopoverContent>
    </Popover>
  )
}
