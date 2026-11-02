import { NumberField } from '@/components/common/form-fields'
import { Segmented } from '@/components/common/segmented'
import { SettingRow } from '@/components/common/setting-row'
import { AppSelect } from '@/components/ui/select'
import { t, type Language } from '@/i18n'
import type { Account, AgentModel, TraderKind } from '@/types'
import {
  accountLabel,
  MAX_INTERVAL,
  MIN_INTERVAL,
  modelLabel,
} from './trader-dialog'

export interface BasicSettings {
  model: string
  account: string
  interval: number
  cross: boolean
}

// What a trader runs with. Changing any of it restarts a running trader, so it is only
// open while the trader is stopped.
export function BasicSection({
  kind,
  basic,
  models,
  accounts,
  locked,
  language,
  onPatch,
}: {
  kind: TraderKind
  basic: BasicSettings
  models: AgentModel[]
  accounts: Account[]
  locked: boolean
  language: Language
  onPatch: (patch: Partial<BasicSettings>) => void
}) {
  const label = (key: string) => t(`traderDialog.${key}`, language)
  return (
    <>
      <SettingRow label={label('kind')}>
        <span className="text-sm text-vergex-ink">
          {label(kind === 'grid' ? 'kindGrid' : 'kindAgent')}
        </span>
      </SettingRow>
      <SettingRow label={label('model')} htmlFor="trader-model">
        <AppSelect
          id="trader-model"
          className="min-w-0 flex-1 max-sm:h-10"
          value={basic.model}
          disabled={locked}
          options={models.map((model) => ({
            value: model.id,
            label: modelLabel(model),
          }))}
          onChange={(model) => onPatch({ model })}
        />
      </SettingRow>
      <SettingRow label={label('account')} htmlFor="trader-account">
        <AppSelect
          id="trader-account"
          className="min-w-0 flex-1 max-sm:h-10"
          value={basic.account}
          disabled={locked}
          options={accounts.map((account) => ({
            value: account.id,
            label: accountLabel(account, accounts),
          }))}
          onChange={(account) => onPatch({ account })}
        />
      </SettingRow>
      <SettingRow label={label('interval')} htmlFor="trader-interval">
        <NumberField
          id="trader-interval"
          value={basic.interval}
          min={MIN_INTERVAL}
          max={MAX_INTERVAL}
          suffix={label('minutes')}
          disabled={locked}
          onChange={(interval) => onPatch({ interval })}
        />
      </SettingRow>
      <SettingRow label={label('margin')}>
        <fieldset disabled={locked} className="min-w-0 border-0 p-0">
          <Segmented<'cross' | 'isolated'>
            aria-label={label('margin')}
            value={basic.cross ? 'cross' : 'isolated'}
            options={[
              { value: 'cross', label: label('marginCross') },
              { value: 'isolated', label: label('marginIsolated') },
            ]}
            onChange={(margin) => onPatch({ cross: margin === 'cross' })}
          />
        </fieldset>
      </SettingRow>
    </>
  )
}
