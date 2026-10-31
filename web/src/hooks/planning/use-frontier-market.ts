import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { useExchangeConfigs, useModelConfigs } from '@/hooks/use-connections'
import {
  defaultMarket,
  frontierVenues,
  preferredAccount as preferredTradingAccount,
  rememberAccount,
} from '@/utils/planning/frontier-accounts'
import type { AIModel, Exchange } from '@/types'

interface FrontierMarketOptions {
  setError: Dispatch<SetStateAction<string>>
  // Called when changed connections select another trading account.
  onAccountSwitch: () => void
}

const NO_ACCOUNTS: Exchange[] = []
const NO_MODELS: AIModel[] = []

// useFrontierMarket holds what Frontier works on: the enabled AI models and
// trading accounts, the chosen model and account, and the open venue and
// market. Connections come from settings through the shared cache, so a change
// saved there applies here at once. On load it opens the preferred account's
// venue unless the user already chose a market.
export function useFrontierMarket({
  setError,
  onAccountSwitch,
}: FrontierMarketOptions) {
  const exchanges = useExchangeConfigs()
  const aiModels = useModelConfigs()
  const accounts = useMemo(
    () =>
      (exchanges.data ?? NO_ACCOUNTS).filter(
        (a) => a.enabled && frontierVenues.includes(a.exchange_type || '')
      ),
    [exchanges.data]
  )
  const models = useMemo(
    () =>
      (aiModels.data ?? NO_MODELS).filter((m) => m.enabled && m.has_api_key),
    [aiModels.data]
  )
  const configsLoaded = Boolean(exchanges.data && aiModels.data)
  const [exchangeID, setExchangeID] = useState('')
  const [modelID, setModelID] = useState('')
  const [venue, setVenue] = useState('binance')
  const requestedVenue = useRef('binance')
  const marketChosen = useRef(false)
  const initialized = useRef(false)
  const [symbol, setSymbol] = useState('BTCUSDT')

  useEffect(() => {
    const failure = exchanges.error || aiModels.error
    if (failure) setError(String(failure.message))
  }, [exchanges.error, aiModels.error])

  useEffect(() => {
    if (!configsLoaded) return
    setModelID((old) =>
      models.some((m) => m.id === old) ? old : models[0]?.id || ''
    )
    if (!initialized.current) {
      initialized.current = true
      const preferred =
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
