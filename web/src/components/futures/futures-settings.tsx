import { useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { getAuthHeaders, handleJSONResponse } from '@/api/helpers'
import { type Constraints, futuresAPI } from '@/api/futures'
import { NumberField, textAreaClass } from '@/components/common/form-fields'
import { Panel, PanelState } from '@/components/common/panel'
import { SettingRow } from '@/components/common/setting-row'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useFuturesLabel } from './use-futures-label'

interface Calculator {
  id: string
  description: string
  hash: string
  status: string
  reports?: { name: string; passed: boolean }[]
}

// What sets a plan's size and costs, in the order they are read. With an account connected
// it sets the budget itself, so the first three only apply without one.
const BUDGET = ['equity', 'max_risk', 'max_notional'] as const
const ASSUMPTIONS = ['min_reward_risk', 'fee_bps', 'slippage_bps'] as const

// FuturesSettings is what planning works from: the limits, kept on this browser as they
// change, and the discussion preferences, saved on leaving the field. Beside them are the
// entry calculators the current conversation's model has built, read only.
export function FuturesSettings({
  active,
  limits,
  onLimitsChange,
  accountSized,
  preferences,
  onPreferencesChange,
  sessionID,
}: {
  active: boolean
  limits: Constraints
  onLimitsChange: (limits: Constraints) => void
  accountSized: boolean
  preferences: string
  onPreferencesChange: (value: string) => void
  sessionID: string
}) {
  const label = useFuturesLabel('tools')
  const { data: calculators, error } = useQuery({
    queryKey: ['futures-calculators', sessionID],
    queryFn: () =>
      futuresAPI<Calculator[]>(
        'entry-plugins?session_id=' + encodeURIComponent(sessionID)
      ),
    enabled: active && Boolean(sessionID),
    retry: false,
  })
  const whenFocused = useRef('')
  const savePreferences = async () => {
    if (preferences === whenFocused.current) return
    try {
      const response = await fetch('/api/futures/preferences', {
        method: 'PUT',
        headers: getAuthHeaders(),
        body: JSON.stringify({ text: preferences }),
      })
      await handleJSONResponse(response)
      toast.success(label('preferencesSaved'))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
    }
  }
  const fields = accountSized ? ASSUMPTIONS : [...BUDGET, ...ASSUMPTIONS]
  return (
    <div className="grid min-w-0 gap-4 lg:h-[max(520px,calc(100dvh-181px))] lg:grid-cols-[minmax(0,1fr)_320px]">
      <Panel
        title={label('riskLimits')}
        className="flex min-h-0 flex-col overflow-hidden"
      >
        <div className="flex min-h-0 flex-1 flex-col border-t border-vergex-line px-4">
          {fields.map((key) => (
            <SettingRow
              key={key}
              label={label(`limit.${key}`)}
              htmlFor={`futures-limit-${key}`}
            >
              <NumberField
                id={`futures-limit-${key}`}
                value={Number(limits[key])}
                min={0}
                suffix={
                  key === 'fee_bps' || key === 'slippage_bps'
                    ? 'bps'
                    : key === 'min_reward_risk'
                      ? label('noMinimum')
                      : undefined
                }
                onChange={(next) =>
                  onLimitsChange({ ...limits, [key]: String(next) })
                }
              />
            </SettingRow>
          ))}
          <SettingRow
            top
            label={label('preferences')}
            htmlFor="futures-preferences"
            className="lg:min-h-0 lg:flex-1 lg:grid-rows-[minmax(0,1fr)] lg:[&>div]:h-full lg:[&>div]:items-stretch"
          >
            <textarea
              id="futures-preferences"
              className={`${textAreaClass} lg:h-full lg:max-h-none lg:[field-sizing:fixed]`}
              value={preferences}
              maxLength={1600}
              onChange={(event) => onPreferencesChange(event.target.value)}
              onFocus={() => {
                whenFocused.current = preferences
              }}
              onBlur={() => void savePreferences()}
            />
          </SettingRow>
        </div>
      </Panel>
      <Panel
        title={label('calculators')}
        className="flex min-h-0 flex-col overflow-hidden"
      >
        <ScrollArea className="min-h-0 flex-1 rounded-b-2xl border-t border-vergex-line">
          {error ? (
            <PanelState role="alert">{label('calculatorsFailed')}</PanelState>
          ) : !calculators?.length ? (
            <PanelState role="status">{label('noCalculators')}</PanelState>
          ) : (
            <ul>
              {calculators.map((calculator) => {
                const reports = calculator.reports ?? []
                return (
                  <li
                    key={calculator.hash}
                    className="border-b border-vergex-line px-4 py-2.5"
                  >
                    <p className="truncate text-[13px] font-medium text-vergex-ink">
                      {calculator.id}
                    </p>
                    <p className="text-xs text-vergex-muted">
                      {calculator.description}
                    </p>
                    <p className="mono mt-1 text-[11px] text-vergex-muted">
                      {calculator.status} ·{' '}
                      {label('calculatorTests', {
                        passed: reports.filter((r) => r.passed).length,
                        total: reports.length,
                      })}
                    </p>
                  </li>
                )
              })}
            </ul>
          )}
        </ScrollArea>
      </Panel>
    </div>
  )
}
