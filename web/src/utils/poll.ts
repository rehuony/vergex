import type { SWRConfiguration } from 'swr'

// Options for data read on a timer. SWR stops polling once a read fails, so here a
// failed read tries again at the same pace: a page left open recovers on its own,
// and the exchange is never asked more often than usual.
export function pollEvery(ms: number): SWRConfiguration {
  return {
    refreshInterval: ms,
    onErrorRetry: (_error, _key, _config, revalidate) => {
      setTimeout(() => void revalidate(), ms)
    },
  }
}
