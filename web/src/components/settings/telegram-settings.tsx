import { useEffect, useId, useState, type ReactNode } from 'react'
import { ArrowRight, Check, ExternalLink, Unlink } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/api'
import { cn } from '@/utils/cn'
import type { TelegramConfig, AgentModel } from '@/types'
import { t, type Language } from '@/i18n'
import { AppSelect } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  DialogNote,
  DialogSteps,
  dialogInputClass,
} from '@/components/common/dialog'
import { Button } from '@/components/ui/button'
import { SettingActions, SettingRow } from '@/components/common/setting-row'

const ALERT_KINDS = [
  'order_rejected',
  'safe_mode',
  'position_closed',
  'plan_attention',
  'forecast',
]

const command = (text: string) => (
  <code className="font-mono text-vergex-ink">{text}</code>
)

// The numbered how-to for the current step.
function HowTo({ items }: { items: ReactNode[] }) {
  return (
    <DialogNote>
      <ol className="list-decimal space-y-1 pl-4 text-[13px] leading-5">
        {items.map((item, index) => (
          <li key={index}>{item}</li>
        ))}
      </ol>
    </DialogNote>
  )
}

/** Telegram bot setup: create the bot, bind a chat, then pick its model. */
export function TelegramSettings({ language }: { language: Language }) {
  const id = useId()
  const [step, setStep] = useState(0)
  const [token, setToken] = useState('')
  const [modelId, setModelId] = useState('')
  const [config, setConfig] = useState<TelegramConfig | null>(null)
  const [models, setModels] = useState<AgentModel[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    Promise.all([
      api.getTelegramConfig().catch(() => null),
      api.getModelConfigs().catch(() => [] as AgentModel[]),
    ])
      .then(([cfg, allModels]) => {
        setModels(allModels.filter((m) => m.enabled))
        if (cfg) {
          setConfig(cfg)
          setModelId(cfg.agent_model_id ?? '')
          if (cfg.is_bound) setStep(2)
          else if (cfg.token_masked) setStep(1)
        }
      })
      .finally(() => setIsLoading(false))
  }, [])

  // Runs one request at a time behind the busy flag.
  const run = async (action: () => Promise<void>) => {
    if (busy) return
    setBusy(true)
    try {
      await action()
    } finally {
      setBusy(false)
    }
  }

  const saveToken = () =>
    run(async () => {
      // Looks like "123456789:ABCdef..."
      if (!/^\d+:[A-Za-z0-9_-]{35,}$/.test(token.trim())) {
        toast.error(t('telegram.invalidTokenFormat', language))
        return
      }
      try {
        await api.updateTelegramConfig(token.trim(), modelId || undefined)
        toast.success(t('telegram.tokenSaved', language))
        setConfig(await api.getTelegramConfig())
        setToken('')
        setStep(1)
      } catch {
        toast.error(t('telegram.saveFailed', language))
      }
    })

  const checkBinding = () =>
    run(async () => {
      try {
        const updated = await api.getTelegramConfig()
        setConfig(updated)
        if (updated.is_bound) {
          setStep(2)
          toast.success(t('telegram.bindSuccess', language))
        } else {
          toast.info(t('telegram.noStartReceived', language))
        }
      } catch {
        toast.error(t('telegram.checkFailed', language))
      }
    })

  const saveModel = () =>
    run(async () => {
      try {
        await api.updateTelegramModel(modelId)
        setConfig((prev) =>
          prev ? { ...prev, agent_model_id: modelId } : prev
        )
        toast.success(t('telegram.modelUpdated', language))
      } catch {
        toast.error(t('telegram.modelUpdateFailed', language))
      }
    })

  const setAlert = (kind: string, on: boolean) =>
    run(async () => {
      try {
        const alerts = await api.updateTelegramAlerts({ [kind]: on })
        setConfig((prev) => (prev ? { ...prev, alerts } : prev))
      } catch {
        toast.error(t('telegram.alertsFailed', language))
      }
    })

  const testAlert = () =>
    run(async () => {
      try {
        await api.testTelegramAlert()
        toast.success(t('telegram.testSent', language))
      } catch (err) {
        toast.error(
          err instanceof Error
            ? err.message
            : t('telegram.testFailed', language)
        )
      }
    })

  const unbind = () =>
    run(async () => {
      try {
        await api.unbindTelegram()
        toast.success(t('telegram.unbound', language))
        const updated = await api.getTelegramConfig()
        setConfig(updated)
        setStep(updated.token_masked ? 1 : 0)
      } catch {
        toast.error(t('telegram.unbindFailed', language))
      }
    })

  const modelRow = (
    <SettingRow label={t('telegram.model', language)} htmlFor={`${id}-model`}>
      {models.length === 0 ? (
        <p className="text-sm text-vergex-muted">
          {t('telegram.noEnabledModels', language)}
        </p>
      ) : (
        <AppSelect
          id={`${id}-model`}
          value={modelId}
          onChange={setModelId}
          options={[
            { value: '', label: t('telegram.autoSelect', language) },
            ...models.map((m) => ({
              value: m.id,
              label: `${m.name}${m.customModelName ? ` · ${m.customModelName}` : ''}`,
            })),
          ]}
          className="h-9"
        />
      )}
    </SettingRow>
  )
  const tokenRow = config?.token_masked && (
    <SettingRow label="Bot Token">
      <p className="truncate font-mono text-sm text-vergex-ink">
        {config.token_masked}
      </p>
    </SettingRow>
  )

  return (
    <div className="space-y-5 pt-4">
      <div className="flex justify-center">
        <DialogSteps
          current={step}
          labels={[
            t('telegram.createBot', language),
            t('telegram.bindAccount', language),
            t('telegram.done', language),
          ]}
        />
      </div>

      {isLoading ? (
        <p className="py-8 text-center text-sm text-vergex-muted">
          {t('telegram.loading', language)}
        </p>
      ) : step === 0 ? (
        <>
          <HowTo
            items={[
              <>
                {t('telegram.step1Desc1', language)} {command('@BotFather')}
              </>,
              <>
                {t('telegram.step1Desc2', language)} {command('/newbot')}{' '}
                {t('telegram.step1Desc2Suffix', language)}
              </>,
              t('telegram.step1Desc3', language),
              t('telegram.step1Desc4', language),
            ]}
          />
          <div>
            <SettingRow label="Bot Token" htmlFor={`${id}-token`}>
              <input
                id={`${id}-token`}
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="123456789:ABCdefGHIjklmNOPQRstuvwxYZ"
                // A bot token, not a login: keep password managers out.
                autoComplete="off"
                className={cn(dialogInputClass, 'h-9 font-mono')}
              />
            </SettingRow>
            {modelRow}
            <SettingActions>
              <Button asChild size="sm">
                <a
                  href="https://t.me/BotFather"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <ExternalLink className="h-4 w-4" />
                  {t('telegram.openBotFather', language)}
                </a>
              </Button>
              <Button
                type="button"
                size="sm"
                variant="primary"
                onClick={() => void saveToken()}
                disabled={busy || !token.trim()}
              >
                {busy ? (
                  t('telegram.savingToken', language)
                ) : (
                  <>
                    {t('telegram.saveAndContinue', language)}
                    <ArrowRight className="h-4 w-4" />
                  </>
                )}
              </Button>
            </SettingActions>
          </div>
        </>
      ) : step === 1 ? (
        <>
          <HowTo
            items={[
              t('telegram.step2Desc1', language),
              <>
                {t('telegram.step2Desc2', language)} {command('/start')}
              </>,
              t('telegram.step2Desc3', language),
            ]}
          />
          <div>
            {tokenRow}
            <SettingActions>
              <Button
                type="button"
                size="sm"
                onClick={() => {
                  setStep(0)
                  setToken('')
                }}
              >
                {t('telegram.reconfigureToken', language)}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="primary"
                onClick={() => void checkBinding()}
                disabled={busy}
              >
                <Check className="h-4 w-4" />
                {t('telegram.checkStatus', language)}
              </Button>
            </SettingActions>
          </div>
        </>
      ) : (
        <>
          <div>
            <SettingRow label={t('status', language)}>
              <span className="inline-flex items-center gap-2 text-sm text-vergex-success">
                <span className="size-2 rounded-full bg-vergex-success" />
                {t('telegram.statusBound', language)}
              </span>
            </SettingRow>
            {tokenRow}
            {modelRow}
            {ALERT_KINDS.filter(
              (kind) => config?.alerts?.[kind] !== undefined
            ).map((kind) => (
              <SettingRow
                key={kind}
                label={t(`telegram.alert.${kind}`, language)}
                className="max-sm:grid-cols-[minmax(0,1fr)_auto] max-sm:items-center"
              >
                <Switch
                  aria-label={t(`telegram.alert.${kind}`, language)}
                  checked={Boolean(config?.alerts?.[kind])}
                  disabled={busy}
                  onChange={(on) => void setAlert(kind, on)}
                />
              </SettingRow>
            ))}
            <SettingActions>
              <Button
                type="button"
                size="sm"
                onClick={() => void testAlert()}
                disabled={busy}
              >
                {t('telegram.sendTest', language)}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="danger"
                onClick={() => void unbind()}
                disabled={busy}
              >
                <Unlink className="h-4 w-4" />
                {t('telegram.unbindAccount', language)}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="primary"
                onClick={() => void saveModel()}
                disabled={busy || modelId === (config?.agent_model_id ?? '')}
              >
                {t('telegram.save', language)}
              </Button>
            </SettingActions>
          </div>
        </>
      )}
    </div>
  )
}
