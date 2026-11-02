import { useEffect, useId, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  Copy,
  ExternalLink,
  Landmark,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import type { Account, AccountState } from '@/types'
import { t, type Language } from '@/i18n'
import { api } from '@/api'
import { cn } from '@/utils/cn'
import { CryptoService, diagnoseWebCryptoEnvironment } from '@/utils/crypto'
import { confirmToast } from '@/components/common/notify'
import {
  configuredAccounts,
  ACCOUNT_STATES_KEY,
  useAccounts,
  useAccountStates,
  useTraders,
} from '@/hooks/use-connections'
import { dialogInputClass } from '@/components/common/dialog'
import { getVenueIcon } from '@/components/common/venue-icons'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import {
  SettingActions,
  SettingBack,
  SettingBadge,
  SettingItem,
  SettingList,
  SettingRow,
  SettingState,
} from '@/components/common/setting-row'

const inputClass = cn(dialogInputClass, 'h-9')

const VENUES: Record<string, string> = { binance: 'Binance', okx: 'OKX' }

const BINANCE_KEY_GUIDE =
  'https://www.binance.com/en/support/faq/how-to-create-api-keys-on-binance-360002502072'

const venueIcon = (type: string) =>
  getVenueIcon(type, { width: 20, height: 20 }) || (
    <Landmark className="size-5" />
  )

const funds = (value: string | number) => {
  const raw = String(value).replace(/,/g, '').trim()
  if (!/^\d+(?:\.\d+)?$/.test(raw)) return String(value)
  const amount = Number(raw)
  return amount.toLocaleString('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: amount > 0 && amount < 0.01 ? 6 : 2,
  })
}

// "321.9725 USDT" → "321.97 USDT"
function balance(state: AccountState) {
  const asset = state.asset || ''
  const shown = state.display_balance || String(state.total_equity ?? '')
  const amount =
    asset && shown.endsWith(` ${asset}`)
      ? shown.slice(0, -asset.length - 1)
      : shown
  return `${funds(amount)}${asset ? ` ${asset}` : ''}`
}

const KNOWN_STATES = [
  'ok',
  'missing_credentials',
  'invalid_credentials',
  'permission_denied',
]

// "checking" while a check runs, "failed" when it could not finish.
type CheckPhase = 'checking' | 'failed' | undefined

function stateBadge(
  state: AccountState | undefined,
  phase: CheckPhase,
  language: Language
) {
  const key =
    phase === 'checking'
      ? 'checking'
      : phase === 'failed'
        ? 'unavailable'
        : !state
          ? 'unknown'
          : KNOWN_STATES.includes(state.status)
            ? state.status
            : 'unavailable'
  const tone =
    key === 'ok'
      ? 'success'
      : key === 'invalid_credentials' || key === 'permission_denied'
        ? 'danger'
        : 'muted'
  return (
    <SettingBadge tone={tone}>
      {t(`connections.state.${key}`, language)}
    </SettingBadge>
  )
}

// The list check's phase for an account it has no answer for yet.
const listPhase = (
  state: AccountState | undefined,
  states: { isLoading: boolean; isFetching: boolean; error?: unknown }
): CheckPhase =>
  state
    ? undefined
    : states.isLoading || states.isFetching
      ? 'checking'
      : states.error
        ? 'failed'
        : undefined

/** Accounts that Futures and traders trade on. */
export function AccountSettings({ language }: { language: Language }) {
  // null shows the list, '' a new account, an id that account.
  const [editing, setEditing] = useState<string | null>(null)
  const { data: accounts, error, isLoading } = useAccounts()
  const client = useQueryClient()
  const states = useAccountStates()
  const saved = configuredAccounts(accounts)
  // Accounts being checked one by one, and those whose check failed.
  const [rows, setRows] = useState<Record<string, CheckPhase>>({})

  const verify = async (id: string) => {
    setRows((prev) => ({ ...prev, [id]: 'checking' }))
    try {
      const state = await api.verifyAccount(id)
      client.setQueryData<typeof states.data>(ACCOUNT_STATES_KEY, (prev) => ({
        states: { ...prev?.states, [id]: state },
      }))
      setRows((prev) => ({ ...prev, [id]: undefined }))
    } catch {
      setRows((prev) => ({ ...prev, [id]: 'failed' }))
    }
  }

  if (editing !== null && accounts)
    return (
      <AccountForm
        key={editing}
        existing={saved.find((e) => e.id === editing)}
        language={language}
        onDone={() => setEditing(null)}
      />
    )

  return (
    <SettingList
      actions={
        <Button
          type="button"
          size="sm"
          variant="primary"
          disabled={!accounts}
          onClick={() => setEditing('')}
        >
          <Plus className="size-4" />
          {t('connections.addAccount', language)}
        </Button>
      }
    >
      {error ? (
        <SettingState role="alert">
          {t('connections.loadFailed', language)}
        </SettingState>
      ) : isLoading || !accounts ? (
        <SettingState role="status">
          {t('connections.loading', language)}
        </SettingState>
      ) : saved.length === 0 ? (
        <SettingState>
          <Landmark className="size-6" aria-hidden="true" />
          {t('connections.noAccounts', language)}
        </SettingState>
      ) : (
        saved.map((account) => {
          const state = states.data?.states?.[account.id]
          const name = account.name || account.venue_name
          const phase = rows[account.id] ?? listPhase(state, states)
          return (
            <SettingItem
              key={account.id}
              icon={venueIcon(account.venue)}
              title={name}
              badge={stateBadge(state, phase, language)}
              detail={[
                VENUES[account.venue] ?? account.venue,
                state?.status === 'ok' && balance(state),
              ]
                .filter(Boolean)
                .join(' · ')}
            >
              <Button
                type="button"
                size="sm"
                variant="secondary"
                aria-label={`${t('connections.verify', language)} ${name}`}
                disabled={phase === 'checking'}
                onClick={() => void verify(account.id)}
                // An icon alone on phones, where the name needs the room.
                className="max-sm:w-9 max-sm:px-0"
              >
                <RefreshCw className="size-4 sm:hidden" aria-hidden="true" />
                <span className="max-sm:sr-only">
                  {t('connections.verify', language)}
                </span>
              </Button>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                aria-label={`${t('connections.edit', language)} ${name}`}
                onClick={() => setEditing(account.id)}
              >
                {t('connections.edit', language)}
              </Button>
            </SettingItem>
          )
        })
      )}
    </SettingList>
  )
}

// Keys travel encrypted when the server asks for it, which needs Web Crypto:
// HTTPS or localhost. Null while unknown.
function useKeysCanTravel() {
  const [ok, setOk] = useState<boolean | null>(null)
  useEffect(() => {
    let live = true
    CryptoService.fetchCryptoConfig()
      .then((config) => {
        const env = diagnoseWebCryptoEnvironment()
        return (
          !config.transport_encryption ||
          (env.isSecureContext && env.hasSubtleCrypto)
        )
      })
      // Without the server's answer, saving reports the real error.
      .catch(() => true)
      .then((result) => live && setOk(result))
    return () => {
      live = false
    }
  }, [])
  return ok
}

// Adds an account, or edits one: blank keys keep the saved ones.
function AccountForm({
  existing,
  language,
  onDone,
}: {
  existing?: Account
  language: Language
  onDone: () => void
}) {
  const id = useId()
  const { refetch } = useAccounts()
  // The list has just checked; opening an account does not check again.
  const states = useAccountStates({ refetchOnMount: false })
  const { data: traders } = useTraders()
  const [type, setType] = useState(existing?.venue ?? 'binance')
  const [accountName, setAccountName] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [secretKey, setSecretKey] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState(false)
  const canTravel = useKeysCanTravel()
  const { data: serverIP } = useQuery({
    queryKey: ['server-ip'],
    queryFn: () => api.getServerIP(),
    enabled: type === 'binance',
    refetchOnWindowFocus: false,
  })
  const okx = type === 'okx'
  const state = existing && states.data?.states?.[existing.id]
  // A new account needs every key; an edit replaces the ones typed in.
  const ready = existing
    ? [apiKey, secretKey, passphrase].some((key) => key.trim() !== '')
    : accountName.trim() !== '' &&
      apiKey.trim() !== '' &&
      secretKey.trim() !== '' &&
      (!okx || passphrase.trim() !== '')

  const run = async (action: () => Promise<void>, failed: string) => {
    if (busy) return
    setBusy(true)
    setMessage('')
    try {
      await action()
      // Check the account again with what was just saved.
      await Promise.all([refetch(), states.refetch()])
      onDone()
    } catch {
      setMessage(t(failed, language))
    } finally {
      setBusy(false)
    }
  }

  const save = (e: FormEvent) => {
    e.preventDefault()
    if (!ready || !canTravel) return
    const keys = {
      api_key: apiKey.trim(),
      secret_key: secretKey.trim(),
      passphrase: okx ? passphrase.trim() : '',
    }
    void run(async () => {
      if (existing) {
        await api.updateAccountsEncrypted({
          accounts: {
            [existing.id]: keys,
          },
        })
        toast.success(t('agentTradersToast.accountUpdated', language))
      } else {
        await api.createAccountEncrypted({
          ...keys,
          venue: type,
          name: accountName.trim(),
        })
        toast.success(t('agentTradersToast.accountCreated', language))
      }
    }, 'saveConfigFailed')
  }

  const remove = async () => {
    if (!existing) return
    const users = (traders ?? []).filter((tr) => tr.account_id === existing.id)
    if (users.length > 0) {
      setMessage(
        t('connections.inUse', language, {
          names: users.map((tr) => tr.trader_name).join(', '),
        })
      )
      return
    }
    if (
      !(await confirmToast(t('confirmDeleteAccount', language), {
        tone: 'danger',
      }))
    )
      return
    await run(async () => {
      await api.deleteAccount(existing.id)
      toast.success(t('agentTradersToast.accountDeleted', language))
    }, 'deleteAccountFailed')
  }

  const copyIP = async (ip: string) => {
    try {
      await navigator.clipboard.writeText(ip)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      toast.error(t('copyIPFailed', language))
    }
  }

  // Keys are labelled as the exchanges' own consoles name them.
  const keyRow = (
    label: string,
    field: string,
    value: string,
    set: (value: string) => void,
    placeholder: string,
    saved?: boolean
  ) => (
    <SettingRow label={label} htmlFor={`${id}-${field}`}>
      <input
        id={`${id}-${field}`}
        type="password"
        autoComplete="off"
        required={!existing}
        value={value}
        onChange={(e) => set(e.target.value)}
        placeholder={
          existing && saved
            ? t('connections.keySaved', language)
            : t(placeholder, language)
        }
        className={inputClass}
      />
    </SettingRow>
  )

  return (
    <form onSubmit={save} aria-label={t('accountsTab', language)}>
      <SettingBack
        onBack={onDone}
        backLabel={t('back', language)}
        icon={venueIcon(type)}
        title={
          existing
            ? existing.name || existing.venue_name
            : t('connections.addAccount', language)
        }
        detail={existing && (VENUES[type] ?? type)}
      >
        {type === 'binance' && (
          <Button asChild variant="link" size="sm" className="text-[13px]">
            <a
              href={BINANCE_KEY_GUIDE}
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('accountConfig.viewTutorial', language)}
              <ExternalLink className="size-3.5" />
            </a>
          </Button>
        )}
      </SettingBack>
      {existing ? (
        <>
          <SettingRow label={t('status', language)}>
            {stateBadge(state, listPhase(state, states), language)}
            {state?.error_message && (
              <span className="min-w-0 truncate text-xs text-vergex-muted">
                {state.error_message}
              </span>
            )}
          </SettingRow>
          {state?.status === 'ok' && (
            <SettingRow label={t('connections.balance', language)}>
              <span className="text-sm tabular-nums text-vergex-ink">
                {balance(state)}
              </span>
              {state.available_balance !== undefined && (
                <span className="text-xs tabular-nums text-vergex-muted">
                  {t('connections.available', language)}{' '}
                  {funds(state.available_balance)} {state.asset}
                </span>
              )}
            </SettingRow>
          )}
        </>
      ) : (
        <>
          <SettingRow
            label={t('connections.venue', language)}
            htmlFor={`${id}-type`}
          >
            <AppSelect
              id={`${id}-type`}
              value={type}
              onChange={setType}
              options={Object.entries(VENUES).map(([value, label]) => ({
                value,
                label,
              }))}
              className="h-9"
            />
          </SettingRow>
          <SettingRow
            label={t('connections.accountName', language)}
            htmlFor={`${id}-name`}
          >
            <input
              id={`${id}-name`}
              type="text"
              required
              value={accountName}
              onChange={(e) => setAccountName(e.target.value)}
              placeholder={t('accountConfig.accountNamePlaceholder', language)}
              className={inputClass}
            />
          </SettingRow>
        </>
      )}
      {keyRow(
        'API Key',
        'key',
        apiKey,
        setApiKey,
        'enterAPIKey',
        existing?.has_api_key
      )}
      {keyRow(
        'Secret Key',
        'secret',
        secretKey,
        setSecretKey,
        'enterSecretKey',
        existing?.has_secret_key
      )}
      {okx &&
        keyRow(
          'Passphrase',
          'passphrase',
          passphrase,
          setPassphrase,
          'enterPassphrase',
          existing?.has_passphrase
        )}
      {type === 'binance' && (
        <SettingRow label={t('connections.whitelistIP', language)}>
          {serverIP?.public_ip ? (
            <>
              <code className="font-mono text-sm text-vergex-ink">
                {serverIP.public_ip}
              </code>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-7 px-2 text-xs"
                onClick={() => void copyIP(serverIP.public_ip)}
              >
                {copied ? (
                  <Check className="size-3.5" />
                ) : (
                  <Copy className="size-3.5" />
                )}
                {t(copied ? 'ipCopied' : 'copyIP', language)}
              </Button>
            </>
          ) : (
            <span className="text-sm text-vergex-muted">
              {t('loadingServerIP', language)}
            </span>
          )}
        </SettingRow>
      )}
      <SettingActions
        message={
          message ||
          (canTravel === false && t('connections.insecure', language))
        }
      >
        {existing && (
          <Button
            type="button"
            size="sm"
            variant="danger"
            disabled={busy}
            onClick={() => void remove()}
          >
            <Trash2 className="size-4" />
            {t('delete', language)}
          </Button>
        )}
        <Button
          type="submit"
          size="sm"
          variant="primary"
          disabled={!ready || !canTravel || busy}
        >
          {busy ? t('saving', language) : t('saveConfig', language)}
        </Button>
      </SettingActions>
    </form>
  )
}
