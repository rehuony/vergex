import {
  FORECAST_INTERVALS,
  type FactorConfig,
  type ForecastConfig,
} from '@/api/forecast'
import {
  cellClass,
  groupHeadClass,
  headClass,
  numberClass,
  StatStrip,
  tableClass,
} from '@/components/common/figures'
import { FieldGrid, NumberField } from '@/components/common/form-fields'
import { SettingRow } from '@/components/common/setting-row'
import { Switch } from '@/components/ui/switch'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'

const CANDLE_FACTORS = [
  'trend',
  'momentum',
  'rsi',
  'bollinger',
  'returns',
  'volume',
  'candle',
  'structure',
  'stochastic',
  'vwap',
  'divergence',
]
const DERIVATIVE_FACTORS = ['funding', 'open_interest', 'long_short']

const factorColumns =
  'grid grid-cols-[minmax(0,1fr)_40px_72px_72px] items-center gap-x-3 sm:grid-cols-[minmax(0,1fr)_56px_112px_112px]'

// A new strategy's rules, as fields. The factor table's heading stays at the top of
// whatever scrolls this.
export function StrategyRules({
  config,
  onChange,
  language,
}: {
  config: ForecastConfig
  onChange: (next: ForecastConfig) => void
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const patch = (next: Partial<ForecastConfig>) =>
    onChange({ ...config, ...next })
  const patchFactor = (name: string, next: Partial<FactorConfig>) =>
    patch({
      factors: {
        ...config.factors,
        [name]: { ...config.factors[name], ...next },
      },
    })

  const factorRow = (name: string) => {
    const factor = config.factors[name]
    const title = label(`factor.${name}`)
    return (
      <div
        key={name}
        className={`${factorColumns} border-t border-vergex-line py-2`}
      >
        <span className="min-w-0 text-sm leading-tight text-vergex-ink">
          {title}
        </span>
        <Switch
          aria-label={title}
          checked={factor.enabled}
          onChange={(enabled) => patchFactor(name, { enabled })}
        />
        <div className="flex">
          <NumberField
            value={factor.trending}
            min={-5}
            max={5}
            step={0.1}
            disabled={!factor.enabled}
            ariaLabel={label('weightOf', {
              name: title,
              regime: label('weightTrending'),
            })}
            onChange={(trending) => patchFactor(name, { trending })}
          />
        </div>
        <div className="flex">
          <NumberField
            value={factor.ranging}
            min={-5}
            max={5}
            step={0.1}
            disabled={!factor.enabled}
            ariaLabel={label('weightOf', {
              name: title,
              regime: label('weightRanging'),
            })}
            onChange={(ranging) => patchFactor(name, { ranging })}
          />
        </div>
      </div>
    )
  }

  const factorGroup = (title: string, names: string[]) => {
    const present = names.filter((name) => config.factors[name])
    if (present.length === 0) return null
    return (
      <>
        <p className="border-t border-vergex-line pb-1 pt-3 text-[11px] font-medium text-vergex-muted">
          {title}
        </p>
        {present.map(factorRow)}
      </>
    )
  }

  return (
    <div className="px-4 pb-4">
      <FieldGrid>
        <SettingRow label={label('adxThreshold')} htmlFor="fc-adx">
          <NumberField
            id="fc-adx"
            value={config.adx_threshold}
            min={5}
            max={60}
            onChange={(adx_threshold) => patch({ adx_threshold })}
          />
        </SettingRow>
        <SettingRow label={label('slope')} htmlFor="fc-slope">
          <NumberField
            id="fc-slope"
            value={config.slope}
            min={0.1}
            max={20}
            step={0.1}
            onChange={(slope) => patch({ slope })}
          />
        </SettingRow>
        <SettingRow label={label('directionThreshold')} htmlFor="fc-direction">
          <NumberField
            id="fc-direction"
            value={Math.round(config.direction_threshold * 100)}
            min={0}
            max={49}
            suffix={label('pointsFrom50')}
            onChange={(points) => patch({ direction_threshold: points / 100 })}
          />
        </SettingRow>
        {FORECAST_INTERVALS.map((interval) => (
          <SettingRow
            key={interval}
            label={label('horizonFor', { interval })}
            htmlFor={`fc-horizon-${interval}`}
          >
            <NumberField
              id={`fc-horizon-${interval}`}
              value={config.horizons[interval]}
              min={1}
              max={96}
              suffix={label('bars')}
              onChange={(bars) =>
                patch({ horizons: { ...config.horizons, [interval]: bars } })
              }
            />
          </SettingRow>
        ))}
      </FieldGrid>

      <div className="relative mt-2 border-t border-vergex-line">
        <div
          className={`${factorColumns} sticky top-0 z-10 bg-vergex-surface py-2 text-xs font-medium text-vergex-muted`}
        >
          <span>{label('factors')}</span>
          <span>{label('on')}</span>
          <span>{label('weightTrending')}</span>
          <span>{label('weightRanging')}</span>
        </div>
        {factorGroup(label('factorsFromCandles'), CANDLE_FACTORS)}
        {factorGroup(label('factorsFromDerivatives'), DERIVATIVE_FACTORS)}
      </div>
    </div>
  )
}

// A factor's weight on a track that runs both ways from zero. The cell has the number, so
// this is left out for assistive technology.
function WeightBar({ weight, largest }: { weight: number; largest: number }) {
  const share = largest > 0 ? (Math.abs(weight) / largest) * 50 : 0
  return (
    <span
      aria-hidden="true"
      className="relative block h-1.5 w-20 rounded-full bg-vergex-surface-muted max-sm:hidden"
    >
      <span
        className="absolute inset-y-0 bg-vergex-ink"
        style={
          weight >= 0
            ? { left: '50%', width: `${share}%` }
            : { right: '50%', width: `${share}%` }
        }
      />
      <span className="absolute -inset-y-1 left-1/2 w-px bg-vergex-muted" />
    </span>
  )
}

// A saved strategy's rules, which cannot change: what it reads as a compact strip, then
// the factors it uses by weight, then those it leaves out.
export function StrategyRulesView({
  config,
  language,
}: {
  config: ForecastConfig
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const used = (names: string[]) =>
    names.filter((name) => config.factors[name]?.enabled)
  const off = [...CANDLE_FACTORS, ...DERIVATIVE_FACTORS].filter(
    (name) => config.factors[name] && !config.factors[name].enabled
  )
  const largest = Math.max(
    0,
    ...Object.values(config.factors)
      .filter((factor) => factor.enabled)
      .flatMap((factor) => [
        Math.abs(factor.trending),
        Math.abs(factor.ranging),
      ])
  )
  const weightCell = (weight: number) => (
    <td className={cn(cellClass, numberClass)}>
      <span className="flex items-center justify-end gap-3">
        <WeightBar weight={weight} largest={largest} />
        <span className="w-8">{weight}</span>
      </span>
    </td>
  )
  const groups = [
    { title: label('factorsFromCandles'), names: used(CANDLE_FACTORS) },
    { title: label('factorsFromDerivatives'), names: used(DERIVATIVE_FACTORS) },
  ].filter((group) => group.names.length > 0)
  const groupHead = (title: string) => (
    <tr>
      <th scope="rowgroup" className={groupHeadClass}>
        {title}
      </th>
      <td className={headClass} />
      <td className={headClass} />
    </tr>
  )

  return (
    <>
      <StatStrip
        small
        className="border-b-0"
        stats={[
          { name: label('adxThreshold'), value: config.adx_threshold },
          { name: label('slope'), value: config.slope },
          {
            name: label('directionThreshold'),
            value: Math.round(config.direction_threshold * 100),
            note: label('pointsFrom50'),
          },
        ]}
      />
      <StatStrip
        small
        className="border-t border-t-vergex-line"
        stats={FORECAST_INTERVALS.map((interval) => ({
          name: label('horizonFor', { interval }),
          value: config.horizons[interval],
          note: label('bars'),
        }))}
      />
      <table className={tableClass}>
        <thead className="sticky top-0 z-10">
          <tr>
            <th className={headClass}>
              {groups[0]?.title ?? label('factors')}
            </th>
            <th className={cn(headClass, 'w-24 text-right sm:w-40')}>
              {label('weightTrending')}
            </th>
            <th className={cn(headClass, 'w-24 text-right sm:w-40')}>
              {label('weightRanging')}
            </th>
          </tr>
        </thead>
        {groups.map((group, index) => (
          <tbody key={group.title}>
            {index > 0 && groupHead(group.title)}
            {group.names.map((name) => (
              <tr key={name}>
                <td className={cn(cellClass, 'text-vergex-ink')}>
                  {label(`factor.${name}`)}
                </td>
                {weightCell(config.factors[name].trending)}
                {weightCell(config.factors[name].ranging)}
              </tr>
            ))}
          </tbody>
        ))}
        {off.length > 0 && (
          <tbody>
            {groupHead(label('factorsOff'))}
            <tr>
              <td
                colSpan={3}
                className={cn(cellClass, 'py-2 text-vergex-muted')}
              >
                {off.map((name) => label(`factor.${name}`)).join(' · ')}
              </td>
            </tr>
          </tbody>
        )}
      </table>
    </>
  )
}
