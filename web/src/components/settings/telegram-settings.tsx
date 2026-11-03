import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowRight,
  Check,
  ChevronRight,
  Copy,
  ExternalLink,
  RotateCcw,
  Send,
  Unlink,
} from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { toast } from 'sonner'
import { api } from '@/api'
import { cn } from '@/utils/cn'
import type { TelegramBindLink, TelegramConfig } from '@/types'
import { t, type Language } from '@/i18n'
import { useModelConfigs } from '@/hooks/use-connections'
import { confirmToast } from '@/components/common/notify'
import { AppSelect } from '@/components/ui/select'
import { DialogSteps, dialogInputClass } from '@/components/common/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PanelLoading } from '@/components/common/panel'
import {
  SettingBadge,
  SettingList,
  SettingRow,
  SettingState,
} from '@/components/common/setting-row'
import { fmtTime } from '@/utils/trader-format'
import { telegramVisitorsQuery, visitorsTotal } from './telegram-queries'
import { TelegramVisitors } from './telegram-visitors'

const ALERT_KINDS = [
  'order_rejected',
  'safe_mode',
  'position_closed',
  'plan_attention',
  'forecast',
  'system',
  'daily_report',
]

const CONFIG_KEY = ['telegram-config']
// While waiting to be bound, the page looks often enough to move on by itself.
const BINDING_POLL_MS = 3000
// The soonest a code is replaced, whatever this computer's clock makes of its expiry.
const MIN_CODE_LIFE_MS = 60000

const command = (text: string) => (
  <code className="font-mono text-vergex-ink">{text}</code>
)

// Heads a step where a form opened from a list has SettingBack, and as tall.
function StepsHeader({
  current,
  language,
}: {
  current: number
  language: Language
}) {
  return (
    <div className="flex min-h-15 items-center justify-center py-3">
      <DialogSteps
        current={current}
        labels={[
          t('telegram.createBot', language),
          t('telegram.bindAccount', language),
          t('telegram.done', language),
        ]}
      />
    </div>
  )
}

// What to do on this step, on the same grid as the fields it leads to.
function HowTo({
  items,
  language,
}: {
  items: ReactNode[]
  language: Language
}) {
  return (
    <SettingRow label={t('telegram.howTo', language)} top>
      <ol className="list-decimal space-y-1 pl-4 text-[13px] leading-5 text-vergex-muted sm:py-2">
        {items.map((item, index) => (
          <li key={index}>{item}</li>
        ))}
      </ol>
    </SettingRow>
  )
}

// The binding link for a phone, where Telegram often is when Settings is on a computer. Its
// box is there while the link is read, so nothing moves when it comes.
function BindScan({
  link,
  failed,
  language,
}: {
  link?: string
  failed: boolean
  language: Language
}) {
  return (
    <SettingRow label={t('telegram.scan', language)} top>
      {failed ? (
        <p className="text-sm text-vergex-muted">—</p>
      ) : (
        <span className="rounded-md border border-vergex-line bg-vergex-qr-ground p-2 text-vergex-qr-ink">
          {link ? (
            <QRCodeSVG
              value={link}
              size={112}
              level="M"
              bgColor="transparent"
              fgColor="currentColor"
              title={t('telegram.scan', language)}
              className="block"
            />
          ) : (
            <Skeleton className="block size-28" />
          )}
        </span>
      )}
    </SettingRow>
  )
}

// What the page is waiting for, and how long the code on screen has left. Only the wait is
// read out: this sits in a live region, which would otherwise speak every second.
function WaitingToBind({
  expiresAt,
  language,
}: {
  expiresAt: string
  language: Language
}) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [])
  const left = Math.max(new Date(expiresAt).getTime() - now, 0)
  const pad = (value: number) => String(value).padStart(2, '0')
  const time = `${pad(Math.floor(left / 60000))}:${pad(Math.floor(left / 1000) % 60)}`
  return (
    <>
      {t('telegram.waitingToBind', language)}
      <span aria-hidden="true" className="tabular-nums">
        {' · '}
        {t('telegram.newCodeIn', language, { time })}
      </span>
    </>
  )
}

// What to send the bot once Start has asked for it, on a Telegram anywhere.
function BindCode({
  code,
  failed,
  language,
}: {
  code?: string
  failed: boolean
  language: Language
}) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code ?? '')
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      toast.error(t('copyFailed', language))
    }
  }
  return (
    <SettingRow label={t('telegram.bindCode', language)}>
      {code ? (
        <>
          <code className="min-w-0 flex-1 font-mono text-sm text-vergex-ink [overflow-wrap:anywhere]">
            {code}
          </code>
          <Button
            type="button"
            size="sm"
            className="max-sm:h-10"
            onClick={() => void copy()}
          >
            {copied ? (
              <Check className="size-4" />
            ) : (
              <Copy className="size-4" />
            )}
            {t(copied ? 'copied' : 'copy', language)}
          </Button>
        </>
      ) : failed ? (
        <p className="text-sm text-vergex-muted">—</p>
      ) : (
        <Skeleton className="w-72" />
      )}
    </SettingRow>
  )
}

// How the bot is really doing, in the badge every connection has: a chat being bound says
// nothing about whether it runs.
function BotStatus({
  config,
  language,
}: {
  config: TelegramConfig
  language: Language
}) {
  const running = config.bot_state === 'running'
  const failed = config.bot_state === 'failed'
  const degraded = config.bot_state === 'degraded'
  const name = config.bot_username ? `@${config.bot_username}` : ''
  const reason = failed || degraded ? config.bot_error : ''
  return (
    <>
      {name && (
        <span title={name} className="min-w-0 truncate text-sm text-vergex-ink">
          {name}
        </span>
      )}
      <SettingBadge
        tone={
          running
            ? 'success'
            : failed
              ? 'danger'
              : degraded
                ? 'warning'
                : 'muted'
        }
      >
        {t(
          running
            ? 'telegram.statusRunning'
            : failed
              ? 'telegram.statusFailed'
              : degraded
                ? 'telegram.statusDegraded'
                : 'telegram.statusStarting',
          language
        )}
      </SettingBadge>
      {reason && (
        <span
          title={reason}
          className="min-w-0 max-w-full truncate text-xs text-vergex-muted"
        >
          {reason}
        </span>
      )}
    </>
  )
}

/** Telegram bot setup: create the bot, bind a chat, then pick its model. */
export function TelegramSettings({ language }: { language: Language }) {
  const id = useId()
  const client = useQueryClient()
  const [token, setToken] = useState('')
  const [tokenError, setTokenError] = useState('')
  const [modelId, setModelId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [bindLink, setBindLink] = useState<TelegramBindLink | null>(null)
  const [bindError, setBindError] = useState('')
  // Counts the codes that ran out on screen, each of which asks for the next.
  const [expiredCodes, setExpiredCodes] = useState(0)
  const [showingVisitors, setShowingVisitors] = useState(false)

  const {
    data: config,
    error,
    refetch,
  } = useQuery({
    queryKey: CONFIG_KEY,
    queryFn: () => api.getTelegramConfig(),
    refetchInterval: (query) => {
      const current = query.state.data
      return current?.token_masked && !current.is_bound
        ? BINDING_POLL_MS
        : false
    },
  })
  const { data: allModels } = useModelConfigs()
  const models = (allModels ?? []).filter((m) => m.enabled)
  const chosenModel = modelId ?? config?.agent_model_id ?? ''

  // Only what is saved decides the step, so setup runs one way: nothing here can show a step
  // the server is not on.
  const step = !config?.token_masked ? 0 : config.is_bound ? 2 : 1

  const { data: visitors, error: visitorsError } = useQuery({
    ...telegramVisitorsQuery,
    enabled: step === 2,
  })

  // A binding link is good for one use and ten minutes, so each visit to this step asks for
  // a new one, as does a code that runs out while it is shown.
  useEffect(() => {
    if (step !== 1) return
    let live = true
    let renewal: ReturnType<typeof setTimeout> | undefined
    setBindLink(null)
    setBindError('')
    api
      .createTelegramBindLink(language)
      .then((link) => {
        if (!live) return
        setBindLink(link)
        renewal = setTimeout(
          () => setExpiredCodes((count) => count + 1),
          Math.max(
            new Date(link.expires_at).getTime() - Date.now(),
            MIN_CODE_LIFE_MS
          )
        )
      })
      .catch(
        (err) =>
          live &&
          setBindError(
            err instanceof Error && err.message
              ? err.message
              : t('telegram.bindLinkFailed', language)
          )
      )
    return () => {
      live = false
      clearTimeout(renewal)
    }
  }, [step, language, expiredCodes])

  const boundUser = [
    config?.username,
    config?.bound_chat_id ? `ID ${config.bound_chat_id}` : '',
  ]
    .filter(Boolean)
    .join(' · ')

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
        setTokenError(t('telegram.invalidTokenFormat', language))
        return
      }
      setTokenError('')
      try {
        await api.updateTelegramConfig(token.trim(), chosenModel || undefined)
        toast.success(t('telegram.tokenSaved', language))
        await refetch()
        setToken('')
        setModelId(null)
      } catch (err) {
        setTokenError(
          err instanceof Error && err.message
            ? err.message
            : t('telegram.saveFailed', language)
        )
      }
    })

  // Back to the first step means giving up the saved token, which stops the bot.
  const reconfigure = () =>
    run(async () => {
      const ok = await confirmToast(
        t('telegram.reconfigureMessage', language),
        {
          title: t('telegram.reconfigureToken', language),
          okText: t('telegram.reconfigureToken', language),
          cancelText: t('cancel', language),
          tone: 'danger',
        }
      )
      if (!ok) return
      try {
        await api.clearTelegramToken()
        await refetch()
      } catch {
        toast.error(t('telegram.reconfigureFailed', language))
      }
    })

  // On the last step everything takes effect as it is chosen, so nothing waits for a save.
  const changeModel = (next: string) =>
    run(async () => {
      setModelId(next)
      try {
        await api.updateTelegramModel(next)
        await refetch()
        toast.success(t('telegram.modelUpdated', language))
      } catch {
        toast.error(t('telegram.modelUpdateFailed', language))
      } finally {
        setModelId(null)
      }
    })

  // Saved one after another, so a quick run of presses all lands: the server works each out
  // from what it last saved.
  const alertSaves = useRef<Promise<unknown>>(Promise.resolve())
  const toggleAlert = (kind: string, on: boolean) => {
    client.setQueryData<TelegramConfig>(CONFIG_KEY, (prev) =>
      prev ? { ...prev, alerts: { ...prev.alerts, [kind]: on } } : prev
    )
    alertSaves.current = alertSaves.current
      .then(() => api.updateTelegramAlerts({ [kind]: on }))
      .catch(() => {
        toast.error(t('telegram.alertsFailed', language))
        void refetch()
      })
  }

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
      const ok = await confirmToast(
        t('telegram.unbindMessage', language, { user: boundUser }),
        {
          title: t('telegram.unbindAccount', language),
          okText: t('telegram.unbindAccount', language),
          cancelText: t('cancel', language),
          tone: 'danger',
        }
      )
      if (!ok) return
      try {
        await api.unbindTelegram()
        toast.success(t('telegram.unbound', language))
        await refetch()
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
          value={chosenModel}
          onChange={(next) =>
            step === 2 ? void changeModel(next) : setModelId(next)
          }
          options={[
            { value: '', label: t('telegram.autoSelect', language) },
            ...models.map((m) => ({
              value: m.id,
              label: `${m.name}${m.model_name ? ` · ${m.model_name}` : ''}`,
            })),
          ]}
          className="h-9"
        />
      )}
    </SettingRow>
  )

  if (showingVisitors) {
    return (
      <TelegramVisitors
        onBack={() => setShowingVisitors(false)}
        language={language}
      />
    )
  }

  if (!config) {
    return (
      <div className="flex h-full flex-col">
        {error ? (
          <SettingState role="alert">
            {t('telegram.loadFailed', language)}
          </SettingState>
        ) : (
          <PanelLoading className="flex-1" />
        )}
      </div>
    )
  }

  // The page moves on by itself once bound, which only a running bot can do.
  const waiting =
    bindLink &&
    (config.bot_state === 'running' || config.bot_state === 'degraded') ? (
      <WaitingToBind expiresAt={bindLink.expires_at} language={language} />
    ) : undefined

  return (
    <SettingList
      // Once it is set up, there are no more steps to show.
      head={
        step < 2 ? (
          <StepsHeader current={step} language={language} />
        ) : undefined
      }
      message={
        step === 0
          ? tokenError
          : step === 1
            ? bindError || waiting
            : config.alerts_error &&
              t('telegram.alertsProblem', language, {
                reason: config.alerts_error,
              })
      }
      messageTone={step === 1 && !bindError ? 'muted' : 'danger'}
      actions={
        step === 0 ? (
          <>
            <Button asChild size="sm">
              <a
                href="https://t.me/BotFather"
                target="_blank"
                rel="noopener noreferrer"
              >
                {t('telegram.openBotFather', language)}
                <ExternalLink className="size-4" />
              </a>
            </Button>
            <Button
              type="button"
              size="sm"
              variant="primary"
              onClick={() => void saveToken()}
            >
              {busy
                ? t('telegram.savingToken', language)
                : t('telegram.saveAndContinue', language)}
              <ArrowRight className="size-4" />
            </Button>
          </>
        ) : step === 1 ? (
          bindLink ? (
            <Button asChild size="sm" variant="primary">
              <a href={bindLink.link} target="_blank" rel="noopener noreferrer">
                {t('telegram.openBindLink', language)}
                <ExternalLink className="size-4" />
              </a>
            </Button>
          ) : (
            <Button type="button" size="sm" variant="primary" disabled>
              {t('telegram.openBindLink', language)}
              <ExternalLink className="size-4" />
            </Button>
          )
        ) : (
          <>
            <Button type="button" size="sm" onClick={() => void testAlert()}>
              <Send className="size-4" />
              {t('telegram.sendTest', language)}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="danger"
              onClick={() => void unbind()}
            >
              <Unlink className="size-4" />
              {t('telegram.unbindAccount', language)}
            </Button>
          </>
        )
      }
    >
      {step === 0 ? (
        <>
          <HowTo
            language={language}
            items={[
              <>
                {t('telegram.step1Desc1', language)} {command('/newbot')}
              </>,
              t('telegram.step1Desc2', language),
              t('telegram.step1Desc3', language),
            ]}
          />
          <SettingRow label="Bot Token" htmlFor={`${id}-token`}>
            <input
              id={`${id}-token`}
              type="password"
              value={token}
              onChange={(e) => {
                setToken(e.target.value)
                setTokenError('')
              }}
              placeholder="123456789:ABCdefGHIjklmNOPQRstuvwxYZ"
              // A bot token, not a login: keep password managers out.
              autoComplete="off"
              aria-invalid={Boolean(tokenError)}
              className={cn(dialogInputClass, 'h-9 font-mono')}
            />
          </SettingRow>
          {modelRow}
        </>
      ) : step === 1 ? (
        <>
          <SettingRow label="Bot">
            <BotStatus config={config} language={language} />
          </SettingRow>
          <SettingRow label="Bot Token">
            <p className="min-w-0 flex-1 truncate font-mono text-sm text-vergex-ink">
              {config.token_masked}
            </p>
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              onClick={() => void reconfigure()}
            >
              <RotateCcw className="size-4" />
              {t('telegram.reconfigureToken', language)}
            </Button>
          </SettingRow>
          <BindScan
            link={bindLink?.link}
            failed={Boolean(bindError)}
            language={language}
          />
          <BindCode
            code={bindLink?.code}
            failed={Boolean(bindError)}
            language={language}
          />
        </>
      ) : (
        <>
          <SettingRow label="Bot">
            <BotStatus config={config} language={language} />
            <code className="ml-auto font-mono text-xs text-vergex-muted">
              {config.token_masked}
            </code>
          </SettingRow>
          <SettingRow label={t('telegram.boundUser', language)}>
            <p
              title={boundUser}
              className="min-w-0 flex-1 truncate text-sm text-vergex-ink max-sm:basis-full"
            >
              {boundUser}
            </p>
            {config.bound_at && (
              <p className="shrink-0 whitespace-nowrap text-xs tabular-nums text-vergex-muted">
                {t('telegram.boundAt', language, {
                  time: fmtTime(config.bound_at),
                })}
              </p>
            )}
          </SettingRow>
          <SettingRow label={t('telegram.visitors', language)}>
            <p className="min-w-0 flex-1 truncate text-sm text-vergex-ink">
              {visitors ? (
                visitors.length ? (
                  visitorsTotal(visitors, language)
                ) : (
                  t('telegram.visitorsEmpty', language)
                )
              ) : visitorsError ? (
                '—'
              ) : (
                <Skeleton className="w-28" />
              )}
            </p>
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              onClick={() => setShowingVisitors(true)}
            >
              {t('telegram.visitorsView', language)}
              <ChevronRight className="size-4" />
            </Button>
          </SettingRow>
          {modelRow}
          <SettingRow label={t('telegram.alertsLabel', language)} top>
            <div
              role="group"
              aria-label={t('telegram.alertsLabel', language)}
              className="grid w-full grid-cols-2 gap-2 sm:grid-cols-3"
            >
              {ALERT_KINDS.filter(
                (kind) => config.alerts?.[kind] !== undefined
              ).map((kind) => (
                <Button
                  key={kind}
                  type="button"
                  variant="option"
                  aria-pressed={Boolean(config.alerts?.[kind])}
                  onClick={() => toggleAlert(kind, !config.alerts?.[kind])}
                  className="min-w-0 px-2 max-sm:h-10"
                >
                  <span className="truncate">
                    {t(`telegram.alert.${kind}`, language)}
                  </span>
                </Button>
              ))}
            </div>
          </SettingRow>
        </>
      )}
    </SettingList>
  )
}
