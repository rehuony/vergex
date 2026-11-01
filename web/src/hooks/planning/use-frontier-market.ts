import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { useSearchParams } from 'react-router-dom'
import { useExchangeConfigs, useModelConfigs } from '@/hooks/use-connections'
import { nativeMarket } from '@/utils/planning/frontier-account'
import {
  defaultMarket,
  frontierVenues,
  preferredAccount as preferredTradingAccount,
  rememberAccount,
} from '@/utils/planning/frontier-accounts'
import type { AgentModel, Exchange } from '@/types'

interface FrontierMarketOptions {
  setError: Dispatch<SetStateAction<string>>
  // Called when changed connections select another trading account.
  onAccountSwitch: () => void
}

const NO_ACCOUNTS: Exchange[] = []
const NO_MODELS: AgentModel[] = []
const MARKET_NAME = /^[A-Z0-9\p{Script=Han}-]{1,40}$/u

// What the address asks for. Anything it cannot use is ignored.
function readMarketLink(params: URLSearchParams) {
  const venue = params.get('venue') ?? ''
  const symbol = (params.get('symbol') ?? '').toUpperCase()
  const validVenue = frontierVenues.includes(venue) ? venue : undefined
  return {
    venue: validVenue,
    symbol: MARKET_NAME.test(symbol)
      ? nativeMarket(symbol, validVenue ?? 'binance')
      : undefined,
    account: params.get('account') ?? undefined,
  }
}

// useFrontierMarket holds what Frontier works on: the enabled models and
// trading accounts, the chosen model and account, and the open venue and
// market. Connections come from settings through the shared cache, so a change
// saved there applies here at once. On load it opens what the address names, or
// else the preferred account's venue unless the user already chose a market.
export function useFrontierMarket({
  setError,
  onAccountSwitch,
}: FrontierMarketOptions) {
  const exchanges = useExchangeConfigs()
  const agentModels = useModelConfigs()
  const accounts = useMemo(
    () =>
      (exchanges.data ?? NO_ACCOUNTS).filter((a) =>
        frontierVenues.includes(a.exchange_type || '')
      ),
    [exchanges.data]
  )
  const models = useMemo(
    () =>
      (agentModels.data ?? NO_MODELS).filter((m) => m.enabled && m.has_api_key),
    [agentModels.data]
  )
  const configsLoaded = Boolean(exchanges.data && agentModels.data)
  const [params] = useSearchParams()
  const [link] = useState(() => readMarketLink(params))
  const [exchangeID, setExchangeID] = useState('')
  const [modelID, setModelID] = useState('')
  const [venue, setVenue] = useState(link.venue ?? 'binance')
  const requestedVenue = useRef(venue)
  const marketChosen = useRef(
    Boolean(link.venue || link.symbol || link.account)
  )
  const initialized = useRef(false)
  const [symbol, setSymbol] = useState(link.symbol ?? defaultMarket(venue))

  useEffect(() => {
    const failure = exchanges.error || agentModels.error
    if (failure) setError(String(failure.message))
  }, [exchanges.error, agentModels.error])

  useEffect(() => {
    if (!configsLoaded) return
    setModelID((old) =>
      models.some((m) => m.id === old) ? old : models[0]?.id || ''
    )
    if (!initialized.current) {
      initialized.current = true
      const linked = accounts.find((a) => a.id === link.account)
      if (linked && linked.exchange_type !== requestedVenue.current) {
        requestedVenue.current = linked.exchange_type
        setVenue(linked.exchange_type)
        setSymbol((old) =>
          link.symbol
            ? nativeMarket(old, linked.exchange_type)
            : defaultMarket(linked.exchange_type)
        )
      }
      const preferred =
        linked ||
        accounts.find(
          (a) =>
            a.id === preferredTradingAccount(accounts, requestedVenue.current)
        ) ||
        (!marketChosen.current &&
        accounts.length === 1 &&
        accounts[0].exchange_type !== requestedVenue.current
          ? accounts[0]
          : undefined)
      setExchangeID(preferred?.id || '')
      if (
        preferred &&
        !marketChosen.current &&
        preferred.exchange_type !== requestedVenue.current
      ) {
        requestedVenue.current = preferred.exchange_type
        setVenue(preferred.exchange_type)
        setSymbol(defaultMarket(preferred.exchange_type || ''))
      }
      return
    }
    // Settings changed the connections: keep the choice while it still exists.
    const matching = accounts.filter((a) => a.exchange_type === venue)
    const next = matching.some((a) => a.id === exchangeID)
      ? exchangeID
      : preferredTradingAccount(accounts, venue)
    if (next !== exchangeID) {
      setExchangeID(next)
      rememberAccount(venue, next)
      onAccountSwitch()
    }
  }, [configsLoaded, accounts, models])

  return {
    configsLoaded,
    accounts,
    models,
    exchangeID,
    setExchangeID,
    modelID,
    setModelID,
    venue,
    setVenue,
    requestedVenue,
    marketChosen,
    symbol,
    setSymbol,
  }
}
