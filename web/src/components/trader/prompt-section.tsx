import { t, type Language } from '@/i18n'
import { SettingRow } from '@/components/common/setting-row'
import type { AgentConfig, PromptSectionsConfig } from '@/types'
import { textAreaClass } from '@/components/common/form-fields'

const SECTIONS: Array<{ key: keyof PromptSectionsConfig; name: string }> = [
  { key: 'role_definition', name: 'promptRole' },
  { key: 'trading_frequency', name: 'promptFrequency' },
  { key: 'entry_standards', name: 'promptEntry' },
  { key: 'decision_process', name: 'promptProcess' },
]

// The parts of the system prompt a trader can rewrite, then the extra
// instructions appended after them.
export function PromptSection({
  agent,
  language,
  onPatchAgent,
}: {
  agent: AgentConfig
  language: Language
  onPatchAgent: (patch: (agent: AgentConfig) => AgentConfig) => void
}) {
  const label = (key: string) => t(`traderSettings.${key}`, language)

  return (
    <>
      {SECTIONS.map(({ key, name }) => (
        <SettingRow
          key={key}
          label={label(name)}
          htmlFor={`prompt-${key}`}
          className="sm:items-start"
        >
          <textarea
            id={`prompt-${key}`}
            className={textAreaClass}
            value={agent.prompt_sections?.[key] ?? ''}
            onChange={(event) =>
              onPatchAgent((current) => ({
                ...current,
                prompt_sections: {
                  ...current.prompt_sections,
                  [key]: event.target.value,
                },
              }))
            }
          />
        </SettingRow>
      ))}
      <SettingRow
        label={label('customPrompt')}
        htmlFor="prompt-custom"
        className="sm:items-start"
      >
        <textarea
          id="prompt-custom"
          className={textAreaClass}
          value={agent.custom_prompt ?? ''}
          onChange={(event) =>
            onPatchAgent((current) => ({
              ...current,
              custom_prompt: event.target.value,
            }))
          }
        />
      </SettingRow>
    </>
  )
}
