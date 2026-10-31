import { useState, type MutableRefObject } from 'react'
import { getAuthHeaders, handleJSONResponse } from '@/api/helpers'
import { type Constraints, planningAPI } from '@/api/planning'
import { PlanningAdvancedTools } from './planning-advanced-tools'
import { useFrontierLabel } from './use-frontier-label'

interface FrontierAdvancedProps {
  limits: Constraints
  onLimitsChange: (limits: Constraints) => void
  accountSized: boolean
  busy: boolean
  onCustomEntry: () => void
  preferences: string
  onPreferencesChange: (value: string) => void
  // The assistant session whose entry tools are inspected.
  sessionRef: MutableRefObject<string>
  onStatus: (status: string) => void
  onError: (message: string) => void
}

// FrontierAdvanced is the collapsed "Advanced tools" section: risk limits,
// a custom entry, saved planning preferences and the session's entry tools.
export function FrontierAdvanced({
  limits,
  onLimitsChange,
  accountSized,
  busy,
  onCustomEntry,
  preferences,
  onPreferencesChange,
  sessionRef,
  onStatus,
  onError,
}: FrontierAdvancedProps) {
  const label = useFrontierLabel('tools')
  const [tools, setTools] = useState<unknown[]>([])
  async function savePreferences() {
    try {
      const r = await fetch('/api/planning/preferences', {
        method: 'PUT',
        headers: getAuthHeaders(),
        body: JSON.stringify({ text: preferences }),
      })
      await handleJSONResponse(r)
      onStatus(label('preferencesSaved'))
    } catch (e) {
      onError(String(e))
    }
  }
  async function inspectEntryTools() {
    try {
      setTools(
        await planningAPI<unknown[]>(
          'entry-plugins?session_id=' + sessionRef.current
        )
      )
    } catch (e) {
      onError(String(e))
    }
  }
  return (
    <details className="frontier-advanced">
      <summary>{label('advanced')}</summary>
      <div>
        <PlanningAdvancedTools
          limits={limits}
          onLimitsChange={onLimitsChange}
          accountSized={accountSized}
          busy={busy}
          onCustomEntry={onCustomEntry}
          preferences={preferences}
          onPreferencesChange={onPreferencesChange}
          onSavePreferences={savePreferences}
          onInspectTools={inspectEntryTools}
          tools={tools}
        />
      </div>
    </details>
  )
}
