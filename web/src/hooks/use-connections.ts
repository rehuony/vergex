import { useQueries, useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import type { AgentModel, Account } from '@/types'

// models and exchange accounts are edited in settings, over whatever page
// is open. Every reader shares these cache keys, so a saved change shows on
// that page at once.

export const useModelConfigs = () =>
  useQuery({
    queryKey: ['model-configs'],
    queryFn: () => api.getModelConfigs(),
  })

export const useSupportedModels = () =>
  useQuery({
    queryKey: ['supported-models'],
    queryFn: () => api.getSupportedModels(),
    refetchOnWindowFocus: false,
  })

export const MODEL_CATALOG_KEY = 'model-catalog'

// The model a connection is set to: the name given to it, else its provider's default.
export const connectionModel = (model: AgentModel) =>
  model.customModelName || model.defaultModel || model.name

// What each connection's provider offers, its own model first. Providers rarely change
// this, and a provider that will not say leaves just the model it is set to.
export function useModelChoices(models: AgentModel[]) {
  const catalogs = useQueries({
    queries: models.map((model) => ({
      queryKey: [MODEL_CATALOG_KEY, model.id],
      queryFn: () => api.getAvailableModels(model.id),
      staleTime: 60 * 60 * 1000,
      refetchOnWindowFocus: false,
      retry: false,
    })),
  })
  return models.flatMap((model, index) => {
    const own = connectionModel(model)
    const others = (catalogs[index].data ?? []).filter((name) => name !== own)
    return [own, ...others].map((name) => ({ id: model.id, name }))
  })
}

export const useAccounts = () =>
  useQuery({ queryKey: ['accounts'], queryFn: () => api.getAccounts() })

// Checking an account calls the exchange, so it runs when the list opens or
// on request, not on focus.
export const ACCOUNT_STATES_KEY = ['account-states']

export const useAccountStates = (options?: { refetchOnMount?: boolean }) =>
  useQuery({
    queryKey: ACCOUNT_STATES_KEY,
    queryFn: () => api.getAccountStates(),
    refetchOnWindowFocus: false,
    ...options,
  })

export const useTraders = () =>
  useQuery({ queryKey: ['traders'], queryFn: () => api.getTraders(true) })

// A model counts as set up once it is enabled or holds a key or endpoint;
// a disabled one stays listed so it can be edited again.
export const configuredModels = (models: AgentModel[] = []) =>
  models.filter(
    (m) => m.enabled || m.has_api_key || Boolean(m.customApiUrl?.trim())
  )

export const configuredAccounts = (accounts: Account[] = []) =>
  accounts.filter((e) => e.has_api_key)
