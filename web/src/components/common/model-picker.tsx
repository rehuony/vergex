import { AppSelect } from '@/components/ui/select'
import { connectionModel, useModelChoices } from '@/hooks/use-connections'
import type { AgentModel } from '@/types'

// A model picked from the list: which connection, and which of its provider's models.
const choiceValue = (id: string, name: string) => `${id} ${name}`

// The model a conversation is on, as wide as its name.
export function ModelPicker({
  label,
  models,
  modelID,
  modelName,
  disabled,
  onChange,
}: {
  label: string
  models: AgentModel[]
  modelID: string
  modelName: string
  disabled?: boolean
  // Another of that connection's models, or blank for the one it is set to.
  onChange: (id: string, name?: string) => void
}) {
  const choices = useModelChoices(models)
  const current = models.find((model) => model.id === modelID)
  return (
    <AppSelect
      aria-label={label}
      className="mr-auto h-8 w-auto min-w-0 border-transparent px-2 text-[13px] hover:bg-vergex-surface-muted max-sm:h-10"
      value={choiceValue(
        modelID,
        modelName || (current ? connectionModel(current) : '')
      )}
      disabled={disabled}
      onChange={(value) => {
        const { id, name } = choices.find(
          (choice) => choiceValue(choice.id, choice.name) === value
        )!
        const model = models.find((row) => row.id === id)
        onChange(id, model && name === connectionModel(model) ? '' : name)
      }}
      options={choices.map(({ id, name }) => ({
        value: choiceValue(id, name),
        label: name,
      }))}
    />
  )
}
