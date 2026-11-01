import useSWR, { type SWRConfiguration } from 'swr'
import { api } from '@/api'
import type { AgentModel, Exchange, TraderInfo } from '@/types'

// models and exchange accounts are edited in settings, over whatever page
// is open. Every reader shares these cache keys, so a saved change shows on
// that page at once.

export const useModelConfigs = () =>
  useSWR<AgentModel[]>('model-configs', () => api.getModelConfigs())

export const useSupportedModels = () =>
  useSWR<AgentModel[]>('supported-models', () => api.getSupportedModels(), {
    revalidateOnFocus: false,
  })

export const useSupportedExchanges = () =>
  useSWR<Exchange[]>('supported-exchanges', () => api.getSupportedExchanges(), {
    revalidateOnFocus: false,
  })

export const useExchangeConfigs = () =>
  useSWR<Exchange[]>('exchange-configs', () => api.getExchangeConfigs())

// Checking an account calls the exchange, so it runs when the list opens or
// on request, not on focus.
export const useExchangeStates = (config?: SWRConfiguration) =>
  useSWR('exchange-account-states', () => api.getExchangeAccountState(), {
    revalidateOnFocus: false,
    ...config,
  })

export const useTraders = () =>
  useSWR<TraderInfo[]>('traders', () => api.getTraders(true))

// A model counts as set up once it is enabled or holds a key or endpoint;
// a disabled one stays listed so it can be edited again.
export const configuredModels = (models: AgentModel[] = []) =>
  models.filter(
    (m) => m.enabled || m.has_api_key || Boolean(m.customApiUrl?.trim())
  )

export const configuredExchanges = (exchanges: Exchange[] = []) =>
  exchanges.filter((e) => e.has_api_key)
