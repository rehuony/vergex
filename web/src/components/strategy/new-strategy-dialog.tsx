import { useState } from 'react'
import { t, type Language } from '@/i18n'
import {
  Dialog,
  DialogField,
  dialogInputClass,
} from '@/components/common/dialog'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import type { StrategyType } from '@/types'
import { cn } from '@/utils/cn'

// Names a new strategy and fixes its type, which cannot change afterwards.
export function NewStrategyDialog({
  language,
  onCreate,
  onClose,
}: {
  language: Language
  onCreate: (name: string, type: StrategyType) => Promise<void>
  onClose: () => void
}) {
  const label = (key: string) => t(`strategy.${key}`, language)
  const [name, setName] = useState(label('newStrategy'))
  const [type, setType] = useState<StrategyType>('agent_trading')
  const [creating, setCreating] = useState(false)

  const create = async () => {
    setCreating(true)
    try {
      await onCreate(name.trim(), type)
    } finally {
      setCreating(false)
    }
  }

  return (
    <Dialog
      size="sm"
      title={label('newStrategy')}
      onClose={onClose}
      closeLabel={label('close')}
      footer={
        <>
          <Button type="button" variant="secondary" onClick={onClose}>
            {label('cancel')}
          </Button>
          <Button
            type="button"
            variant="primary"
            disabled={creating || !name.trim()}
            onClick={() => void create()}
          >
            {label('create')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <DialogField label={label('name')} htmlFor="new-strategy-name">
          <input
            id="new-strategy-name"
            className={cn(dialogInputClass, 'max-sm:text-base')}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </DialogField>
        <DialogField label={label('type')} htmlFor="new-strategy-type">
          <AppSelect
            id="new-strategy-type"
            value={type}
            options={[
              { value: 'agent_trading', label: label('typeAgent') },
              { value: 'grid_trading', label: label('typeGrid') },
            ]}
            onChange={(next) => setType(next as StrategyType)}
          />
        </DialogField>
      </div>
    </Dialog>
  )
}
