import { t, type Language } from '@/i18n/translations'
import { SettingRow } from '@/components/common/setting-row'
import type { AIStrategyConfig, PromptSectionsConfig } from '@/types'
import { textAreaClass } from '@/components/common/form-fields'

const SECTIONS: Array<{ key: keyof PromptSectionsConfig; name: string }> = [
  { key: 'role_definition', name: 'promptRole' },
  { key: 'trading_frequency', name: 'promptFrequency' },
  { key: 'entry_standards', name: 'promptEntry' },
  { key: 'decision_process', name: 'promptProcess' },
]

// The parts of the system prompt a strategy can rewrite, then the extra
// instructions appended after them.
export function PromptSection({
  ai,
  language,
  onPatchAI,
}: {
  ai: AIStrategyConfig
  language: Language
  onPatchAI: (patch: (ai: AIStrategyConfig) => AIStrategyConfig) => void
}) {
  const label = (key: string) => t(`strategy.${key}`, language)

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
            value={ai.prompt_sections?.[key] ?? ''}
            onChange={(event) =>
              onPatchAI((current) => ({
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
          value={ai.custom_prompt ?? ''}
          onChange={(event) =>
            onPatchAI((current) => ({
              ...current,
              custom_prompt: event.target.value,
            }))
          }
        />
      </SettingRow>
    </>
  )
}
