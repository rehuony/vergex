import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/api'
import { notify } from '@/components/common/notify'
import { t, type Language } from '@/i18n'
import { marketSettingsQuery } from './forecast-queries'

export const MAX_WATCHLIST = 100

const NONE: string[] = []

// The coins starred in the coin list. A star puts a coin on the watchlist itself, so a
// starred coin is collected at every interval and alerts, whatever its volume does.
export function useWatchlist(language: Language) {
  const client = useQueryClient()
  const { data } = useQuery(marketSettingsQuery)
  const watchlist = data?.watchlist ?? NONE

  const toggle = async (symbol: string) => {
    if (!data) return
    const starred = watchlist.includes(symbol)
    if (!starred && watchlist.length >= MAX_WATCHLIST) {
      notify.warning(
        t('forecastSettings.watchlistLimit', language, { count: MAX_WATCHLIST })
      )
      return
    }
    const next = {
      ...data,
      watchlist: starred
        ? watchlist.filter((held) => held !== symbol)
        : [...watchlist, symbol],
    }
    client.setQueryData(marketSettingsQuery.queryKey, next)
    try {
      client.setQueryData(
        marketSettingsQuery.queryKey,
        await api.saveMarketDataSettings(next)
      )
    } catch (err) {
      client.setQueryData(marketSettingsQuery.queryKey, data)
      notify.error(
        err instanceof Error
          ? err.message
          : t('forecastSettings.saveFailed', language)
      )
    }
  }

  return { watchlist, toggle }
}
