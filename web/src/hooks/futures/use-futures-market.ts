import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { useSearchParams } from 'react-router-dom'
import { useAccounts, useModelConfigs } from '@/hooks/use-connections'
import { nativeMarket } from '@/utils/futures/futures-account'
import {
  defaultMarket,
  futuresVenues,
  preferredAccount as preferredTradingAccount,
  rememberAccount,
} from '@/utils/futures/futures-accounts'
import type { AgentModel, Account } from '@/types'

interface FuturesMarketOptions {
  setError: Dispatch<SetStateAction<string>>
  // Called when changed connections select another trading account.
  onAccountSwitch: () => void
}

const NO_ACCOUNTS: Account[] = []
const NO_MODELS: AgentModel[] = []
const MARKET_NAME = /^[A-Z0-9\p{Script=Han}-]{1,40}$/u

// What the address asks for. Anything it cannot use is ignored.
function readMarketLink(params: URLSearchParams) {
  const venue = params.get('venue') ?? ''
  const symbol = (params.get('symbol') ?? '').toUpperCase()
  const validVenue = futuresVenues.includes(venue) ? venue : undefined
  return {
    venue: validVenue,
    symbol: MARKET_NAME.test(symbol)
      ? nativeMarket(symbol, validVenue ?? 'binance')
      : undefined,
    account: params.get('account') ?? undefined,
  }
}

// useFuturesMarket holds what Futures works on: the enabled models and
// trading accounts, the chosen model and account, and the open venue and
// market. Connections come from settings through the shared cache, so a change
// saved there applies here at once. On load it opens what the address names, or
// else the preferred account's venue unless the user already chose a market.
export function useFuturesMarket({
  setError,
  onAccountSwitch,
}: FuturesMarketOptions) {
  const savedAccounts = useAccounts()
  const agentModels = useModelConfigs()
  const accounts = useMemo(
    () =>
      (savedAccounts.data ?? NO_ACCOUNTS).filter((a) =>
        futuresVenues.includes(a.venue || '')
      ),
    [savedAccounts.data]
  )
  const models = useMemo(
    () =>
      (agentModels.data ?? NO_MODELS).filter((m) => m.enabled && m.has_api_key),
    [agentModels.data]
  )
  const configsLoaded = Boolean(savedAccounts.data && agentModels.data)
  const [params] = useSearchParams()
  const [link] = useState(() => readMarketLink(params))
  const [accountId, setAccountId] = useState('')
  const [modelID, setModelID] = useState('')
  // Another of that provider's models, instead of the one the connection is set to.
  const [modelName, setModelName] = useState('')
  const [venue, setVenue] = useState(link.venue ?? 'binance')
  const requestedVenue = useRef(venue)
  const marketChosen = useRef(
    Boolean(link.venue || link.symbol || link.account)
  )
  const initialized = useRef(false)
  const [symbol, setSymbol] = useState(link.symbol ?? defaultMarket(venue))

  useEffect(() => {
    const failure = savedAccounts.error || agentModels.error
    if (failure) setError(String(failure.message))
  }, [savedAccounts.error, agentModels.error])

  useEffect(() => {
    if (!configsLoaded) return
    if (!models.some((m) => m.id === modelID)) {
      setModelID(models[0]?.id || '')
      setModelName('')
    }
    if (!initialized.current) {
      initialized.current = true
      const linked = accounts.find((a) => a.id === link.account)
      if (linked && linked.venue !== requestedVenue.current) {
        requestedVenue.current = linked.venue
        setVenue(linked.venue)
        setSymbol((old) =>
          link.symbol
            ? nativeMarket(old, linked.venue)
            : defaultMarket(linked.venue)
        )
      }
      const preferred =
        linked ||
        accounts.find(
          (a) =>
            a.id === preferredTradingAccount(accounts, requestedVenue.current)
        ) ||
        (!marketChosen.current ? accounts[0] : undefined)
      setAccountId(preferred?.id || '')
      if (
        preferred &&
        !marketChosen.current &&
        preferred.venue !== requestedVenue.current
      ) {
        requestedVenue.current = preferred.venue
        setVenue(preferred.venue)
        setSymbol(defaultMarket(preferred.venue || ''))
      }
      return
    }
    // Settings changed the connections: keep the choice while it still exists.
    const matching = accounts.filter((a) => a.venue === venue)
    const next = matching.some((a) => a.id === accountId)
      ? accountId
      : preferredTradingAccount(accounts, venue)
    if (next !== accountId) {
      setAccountId(next)
      rememberAccount(venue, next)
      onAccountSwitch()
    }
  }, [configsLoaded, accounts, models])

  return {
    configsLoaded,
    accounts,
    models,
    accountId,
    setAccountId,
    modelID,
    modelName,
    chooseModel: (id: string, name = '') => {
      setModelID(id)
      setModelName(name)
    },
    venue,
    setVenue,
    requestedVenue,
    marketChosen,
    symbol,
    setSymbol,
  }
}
