import { useEffect, useId, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BookOpen,
  Check,
  Copy,
  Landmark,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import type { Account, AccountState, KeyPermissions } from '@/types'
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
import { PanelLoading } from '@/components/common/panel'
import { getVenueIcon } from '@/components/common/venue-icons'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { AppSelect } from '@/components/ui/select'
import { AccountLedger } from './account-ledger'
import {
  SettingBack,
  SettingBadge,
  SettingItem,
  SettingList,
  SettingRow,
  SettingState,
} from '@/components/common/setting-row'
import { codedText } from '@/utils/request-errors'

const inputClass = cn(dialogInputClass, 'h-9')

const VENUES: Record<string, string> = { binance: 'Binance', okx: 'OKX' }

// Which environment an account is on: "Binance", or "Binance · Testnet".
const environment = (
  account: Pick<Account, 'venue' | 'testnet'>,
  language: Language
) =>
  [
    VENUES[account.venue] ?? account.venue,
    account.testnet && t('connections.testnet', language),
  ]
    .filter(Boolean)
    .join(' · ')

// The accounts by environment. The server sends them in that order, by name within it.
function byEnvironment(accounts: Account[], language: Language) {
  const groups = new Map<string, Account[]>()
  for (const account of accounts) {
    const name = environment(account, language)
    groups.set(name, [...(groups.get(name) ?? []), account])
  }
  return [...groups]
}

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

// What a key may do, as far as its venue says.
const permissionNames = (permissions: KeyPermissions, language: Language) =>
  (
    [
      [permissions.read, 'read'],
      [permissions.trade, 'trade'],
      [permissions.withdraw, 'withdraw'],
      [permissions.ip_restricted, 'ipRestricted'],
    ] as const
  )
    .filter(([on]) => on)
    .map(([, name]) => t(`connections.permissions.${name}`, language))
    .join(' · ')

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
        <PanelLoading className="flex-1" />
      ) : saved.length === 0 ? (
        <SettingState>
          <Landmark className="size-6" aria-hidden="true" />
          {t('connections.noAccounts', language)}
        </SettingState>
      ) : (
        byEnvironment(saved, language).map(([group, members]) => (
          <section
            key={group}
            aria-label={group}
            className="border-t border-vergex-line pt-3 first:border-t-0 first:pt-1"
          >
            <h4 className="text-xs font-medium text-vergex-muted">{group}</h4>
            <div>
              {members.map((account) => {
                const state = states.data?.states?.[account.id]
                const name = account.name || account.venue_name
                const phase = rows[account.id] ?? listPhase(state, states)
                const money = state?.status === 'ok' ? balance(state) : ''
                const may =
                  state?.permissions &&
                  permissionNames(state.permissions, language)
                return (
                  <SettingItem
                    key={account.id}
                    icon={venueIcon(account.venue)}
                    title={name}
                    badge={
                      <>
                        {stateBadge(state, phase, language)}
                        {state?.permissions?.withdraw && (
                          <SettingBadge tone="danger">
                            {t('connections.permissions.canWithdraw', language)}
                          </SettingBadge>
                        )}
                      </>
                    }
                    // Always a line, so a row keeps its height as the answer comes.
                    detail={
                      phase === 'checking' ? (
                        <Skeleton className="w-28" />
                      ) : money || may ? (
                        <>
                          {money && <span className="mono">{money}</span>}
                          {money && may && ' · '}
                          {may}
                        </>
                      ) : (
                        '\u00a0'
                      )
                    }
                  >
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      aria-label={`${t('connections.verify', language)} ${name}`}
                      disabled={phase === 'checking'}
                      onClick={() => void verify(account.id)}
                      // An icon alone on phones, where the name needs the room.
                      className="max-sm:size-10 max-sm:px-0"
                    >
                      <RefreshCw className="size-4" />
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
                      className="max-sm:size-10 max-sm:px-0"
                    >
                      <Pencil className="size-4" />
                      <span className="max-sm:sr-only">
                        {t('connections.edit', language)}
                      </span>
                    </Button>
                  </SettingItem>
                )
              })}
            </div>
          </section>
        ))
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

// Adds an account, or edits one: its name, and any keys typed in. Blank keys keep the
// saved ones. What it trades on, its venue and whether it is on testnet, stays as it began.
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
  const [accountName, setAccountName] = useState(existing?.name ?? '')
  const [testnet, setTestnet] = useState(false)
  const [apiKey, setApiKey] = useState('')
  const [secretKey, setSecretKey] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState(false)
  const [ledgerOpen, setLedgerOpen] = useState(false)
  const canTravel = useKeysCanTravel()
  const { data: serverIP } = useQuery({
    queryKey: ['server-ip'],
    queryFn: () => api.getServerIP(),
    refetchOnWindowFocus: false,
  })
  const okx = type === 'okx'
  const state = existing && states.data?.states?.[existing.id]
  const problem =
    state?.error_message &&
    codedText('accountError', state.error_code, state.error_message, language)
  const name = accountName.trim()
  const renamed = Boolean(existing) && name !== existing?.name
  const keysTyped = [apiKey, secretKey, passphrase].some(
    (key) => key.trim() !== ''
  )
  // A new account needs every key; an edit saves a new name and the keys typed in.
  const ready = existing
    ? name !== '' && (renamed || keysTyped)
    : name !== '' &&
      apiKey.trim() !== '' &&
      secretKey.trim() !== '' &&
      (!okx || passphrase.trim() !== '')
  // Only keys need the encrypted route.
  const canSave = ready && (!keysTyped || Boolean(canTravel))

  const run = async (
    action: () => Promise<void>,
    failed: string,
    recheck = true
  ) => {
    if (busy) return
    setBusy(true)
    setMessage('')
    try {
      await action()
      // Check the account again with what was just saved.
      await Promise.all([refetch(), recheck && states.refetch()])
      onDone()
    } catch {
      setMessage(t(failed, language))
    } finally {
      setBusy(false)
    }
  }

  const save = (e: FormEvent) => {
    e.preventDefault()
    if (!canSave) return
    const keys = {
      api_key: apiKey.trim(),
      secret_key: secretKey.trim(),
      passphrase: okx ? passphrase.trim() : '',
    }
    void run(
      async () => {
        if (existing) {
          if (renamed) await api.renameAccount(existing.id, name)
          if (keysTyped)
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
            name,
            testnet,
          })
          toast.success(t('agentTradersToast.accountCreated', language))
        }
      },
      'saveConfigFailed',
      !existing || keysTyped
    )
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
      !(await confirmToast(
        t('confirmDeleteAccount', language, {
          name: existing.name || existing.venue_name,
        }),
        {
          title: t('deleteAccountTitle', language),
          okText: t('delete', language),
          tone: 'danger',
        }
      ))
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

  if (existing && ledgerOpen) {
    return (
      <AccountLedger
        account={existing}
        language={language}
        onBack={() => setLedgerOpen(false)}
      />
    )
  }

  return (
    <form
      onSubmit={save}
      aria-label={t('accountsTab', language)}
      className="h-full"
    >
      <SettingList
        head={
          <SettingBack
            onBack={onDone}
            backLabel={t('back', language)}
            icon={venueIcon(type)}
            title={
              existing
                ? existing.name || existing.venue_name
                : t('connections.addAccount', language)
            }
            badge={
              existing && stateBadge(state, listPhase(state, states), language)
            }
            detail={
              existing && (
                <span title={problem || undefined}>
                  {[environment(existing, language), problem]
                    .filter(Boolean)
                    .join(' · ')}
                </span>
              )
            }
          >
            {existing && (
              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="max-sm:h-10"
                onClick={() => setLedgerOpen(true)}
              >
                <BookOpen className="size-4" />
                {t('connections.ledger.title', language)}
              </Button>
            )}
          </SettingBack>
        }
        message={
          message ||
          (canTravel === false && t('connections.insecure', language))
        }
        actions={
          <>
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
              disabled={!canSave || busy}
            >
              <Check className="size-4" />
              {busy ? t('saving', language) : t('saveConfig', language)}
            </Button>
          </>
        }
      >
        {!existing && (
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
              className="h-9 w-auto min-w-0 flex-1"
            />
            <Button
              type="button"
              variant="option"
              aria-pressed={testnet}
              onClick={() => setTestnet((on) => !on)}
            >
              {t('connections.testnet', language)}
            </Button>
          </SettingRow>
        )}
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
        {existing && (
          <SettingRow label={t('connections.equity', language)}>
            {state?.status === 'ok' ? (
              <>
                <span className="mono text-sm text-vergex-ink">
                  {balance(state)}
                </span>
                {state.available_balance !== undefined && (
                  <span className="text-xs text-vergex-muted">
                    {t('connections.available', language)}{' '}
                    <span className="mono">
                      {funds(state.available_balance)} {state.asset}
                    </span>
                  </span>
                )}
              </>
            ) : listPhase(state, states) === 'checking' ? (
              <Skeleton className="w-28" />
            ) : (
              <span className="mono text-sm text-vergex-muted">—</span>
            )}
          </SettingRow>
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
        <SettingRow label={t('connections.serverIP', language)}>
          {serverIP?.public_ip ? (
            <>
              <code className="min-w-0 flex-1 truncate font-mono text-sm text-vergex-ink">
                {serverIP.public_ip}
              </code>
              <Button
                type="button"
                size="sm"
                className="max-sm:h-10"
                onClick={() => void copyIP(serverIP.public_ip)}
              >
                {copied ? (
                  <Check className="size-4" />
                ) : (
                  <Copy className="size-4" />
                )}
                {t(copied ? 'ipCopied' : 'copyIP', language)}
              </Button>
            </>
          ) : (
            <Skeleton className="w-28" />
          )}
        </SettingRow>
      </SettingList>
    </form>
  )
}
