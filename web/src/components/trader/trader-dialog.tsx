import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import {
  Dialog,
  DialogField,
  dialogInputClass,
} from '@/components/common/dialog'
import { NumberField } from '@/components/common/form-fields'
import { Segmented } from '@/components/common/segmented'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import { testnetMark } from '@/constants/model-providers'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import type {
  Account,
  AgentModel,
  CreateTraderRequest,
  TraderKind,
} from '@/types'
import { cn } from '@/utils/cn'

export const MIN_INTERVAL = 3
export const MAX_INTERVAL = 60

export function accountLabel(account: Account) {
  const venue = account.venue_name || account.venue
  const name = account.name ? `${venue} · ${account.name}` : venue
  return name + testnetMark(account)
}

export function modelLabel(model: AgentModel) {
  return model.customModelName
    ? `${model.name} · ${model.customModelName}`
    : model.name
}

interface TraderDialogProps {
  models: AgentModel[]
  accounts: Account[]
  onClose: () => void
}

// What a trader starts with: its name, model, account, interval and margin mode.
// Creating one also fixes its kind and picks a preset; everything else about it
// is edited on its Settings tab.
export function NewTraderDialog({
  models,
  accounts,
  onClose,
  onCreate,
}: TraderDialogProps & {
  onCreate: (request: CreateTraderRequest) => Promise<void>
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`traderDialog.${key}`, language)
  const [form, setForm] = useState(() => ({
    name: '',
    model: models[0]?.id ?? '',
    account: accounts[0]?.id ?? '',
    interval: 15,
    cross: true,
  }))
  const [kind, setKind] = useState<TraderKind>('agent')
  const [preset, setPreset] = useState('')
  const [saving, setSaving] = useState(false)
  const patch = (next: Partial<typeof form>) =>
    setForm((current) => ({ ...current, ...next }))

  const { data: presets } = useQuery({
    queryKey: ['trader-presets', kind],
    queryFn: () => api.getTraderPresets(kind),
  })
  const chosenPreset = preset || presets?.[0]?.id || ''

  const create = async () => {
    setSaving(true)
    try {
      await onCreate({
        name: form.name.trim(),
        agent_model_id: form.model,
        account_id: form.account,
        kind,
        preset: chosenPreset,
        scan_interval_minutes: form.interval,
        is_cross_margin: form.cross,
      })
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      size="md"
      title={t('createTrader', language)}
      onClose={onClose}
      closeLabel={t('cancel', language)}
      footer={
        <>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('cancel', language)}
          </Button>
          <Button
            type="button"
            variant="primary"
            disabled={
              saving || !form.name.trim() || !form.model || !form.account
            }
            onClick={() => void create()}
          >
            {saving
              ? t('traderSettings.saving', language)
              : t('createTrader', language)}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <NameField value={form.name} onChange={(name) => patch({ name })} />
        <DialogField label={label('kind')}>
          <Segmented<TraderKind>
            aria-label={label('kind')}
            value={kind}
            options={[
              { value: 'agent', label: label('kindAgent') },
              { value: 'grid', label: label('kindGrid') },
            ]}
            onChange={(next) => {
              setKind(next)
              setPreset('')
            }}
          />
        </DialogField>
        <DialogField label={label('preset')} htmlFor="trader-preset">
          <AppSelect
            id="trader-preset"
            value={chosenPreset}
            options={(presets ?? []).map((item) => ({
              value: item.id,
              label: t(`traderSettings.profile.${item.id}`, language),
            }))}
            onChange={setPreset}
          />
        </DialogField>
        <RunWithFields
          form={form}
          models={models}
          accounts={accounts}
          onPatch={patch}
        />
      </div>
    </Dialog>
  )
}

function NameField({
  value,
  onChange,
}: {
  value: string
  onChange: (name: string) => void
}) {
  const { language } = useLanguage()
  return (
    <DialogField label={t('traderDialog.name', language)} htmlFor="trader-name">
      <input
        id="trader-name"
        className={cn(dialogInputClass, 'max-sm:text-base')}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </DialogField>
  )
}

interface RunWith {
  model: string
  account: string
  interval: number
  cross: boolean
}

function RunWithFields({
  form,
  models,
  accounts,
  onPatch,
}: {
  form: RunWith
  models: AgentModel[]
  accounts: Account[]
  onPatch: (patch: Partial<RunWith>) => void
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`traderDialog.${key}`, language)
  return (
    <>
      <DialogField label={label('model')} htmlFor="trader-model">
        <AppSelect
          id="trader-model"
          value={form.model}
          options={models.map((model) => ({
            value: model.id,
            label: modelLabel(model),
          }))}
          onChange={(model) => onPatch({ model })}
        />
      </DialogField>
      <DialogField label={label('account')} htmlFor="trader-account">
        <AppSelect
          id="trader-account"
          value={form.account}
          options={accounts.map((account) => ({
            value: account.id,
            label: accountLabel(account),
          }))}
          onChange={(account) => onPatch({ account })}
        />
      </DialogField>
      <DialogField label={label('interval')} htmlFor="trader-interval">
        <div className="flex items-center gap-2">
          <NumberField
            id="trader-interval"
            value={form.interval}
            min={MIN_INTERVAL}
            max={MAX_INTERVAL}
            suffix={label('minutes')}
            onChange={(interval) => onPatch({ interval })}
          />
        </div>
      </DialogField>
      <DialogField label={label('margin')}>
        <Segmented<'cross' | 'isolated'>
          aria-label={label('margin')}
          value={form.cross ? 'cross' : 'isolated'}
          options={[
            { value: 'cross', label: label('marginCross') },
            { value: 'isolated', label: label('marginIsolated') },
          ]}
          onChange={(margin) => onPatch({ cross: margin === 'cross' })}
        />
      </DialogField>
    </>
  )
}
