import { defaultLimits, type Constraints } from '@/api/planning'
import { Button } from '@/components/ui/button'
import { useFrontierLabel } from './use-frontier-label'

// PlanningAdvancedTools holds the risk budget inputs, the manual entry
// launcher and discussion preferences of the Frontier page.
export function PlanningAdvancedTools({
  limits,
  onLimitsChange,
  accountSized,
  busy,
  onCustomEntry,
  preferences,
  onPreferencesChange,
  onSavePreferences,
  onInspectTools,
  tools,
}: {
  limits: Constraints
  onLimitsChange: (limits: Constraints) => void
  accountSized: boolean
  busy: boolean
  onCustomEntry: () => void
  preferences: string
  onPreferencesChange: (value: string) => void
  onSavePreferences: () => void
  onInspectTools: () => void
  tools: unknown[]
}) {
  const label = useFrontierLabel('tools')
  return (
    <details className="planning-tools">
      <summary>{label('summary')}</summary>
      <div className="planning-limit-grid">
        {(
          Object.keys(defaultLimits) as Exclude<
            keyof Constraints,
            'leverage' | 'available_margin' | 'sizing_policy' | 'planning_only'
          >[]
        )
          .filter(
            (key) =>
              !accountSized ||
              !['equity', 'max_risk', 'max_notional'].includes(key)
          )
          .map((key) => (
            <label key={key}>
              {label(`limit.${key}`)}
              <input
                value={limits[key]}
                inputMode="decimal"
                disabled={
                  busy ||
                  (accountSized &&
                    ['equity', 'max_risk', 'max_notional'].includes(key))
                }
                onChange={(e) =>
                  onLimitsChange({ ...limits, [key]: e.target.value })
                }
              />
            </label>
          ))}
      </div>
      <p>{label('sizingNote')}</p>
      <Button type="button" size="sm" disabled={busy} onClick={onCustomEntry}>
        {label('customEntry')}
      </Button>
      <label>
        {label('preferences')}
        <textarea
          value={preferences}
          onChange={(e) => onPreferencesChange(e.target.value)}
          maxLength={1600}
        />
      </label>
      <div className="flex flex-wrap gap-2">
        <Button type="button" size="sm" onClick={onSavePreferences}>
          {label('savePreferences')}
        </Button>
        <Button type="button" size="sm" onClick={onInspectTools}>
          {label('inspect')}
        </Button>
      </div>
      {tools.length > 0 && <pre>{JSON.stringify(tools, null, 2)}</pre>}
    </details>
  )
}
