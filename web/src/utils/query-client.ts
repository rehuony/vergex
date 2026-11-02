import {
  dehydrate,
  hydrate,
  QueryClient,
  type Query,
} from '@tanstack/react-query'

// Every read shares this cache. A page shows what it holds at once and refreshes it behind
// that. Going back to something read in the last few seconds asks nothing, and what has
// left the screen stays for half an hour, so coming back to it opens at once.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      gcTime: 30 * 60_000,
      retry: 1,
      // The app is self-hosted, often on this machine or a private network, so the
      // browser saying it is offline says nothing about reaching it.
      networkMode: 'always',
    },
    mutations: { networkMode: 'always' },
  },
})

const STORAGE_KEY = 'vergex.query-cache'
const SAVE_EVERY_MS = 2_000
const KEPT_FOR_MS = 24 * 60 * 60_000

// A build's own file name changes with its code, so a read saved by another build, which
// may have another shape, is never shown by this one.
const BUILD = import.meta.url

// What opens at once after a reload, and is read again straight behind that. Only
// Forecast's: account, order and trader data stays out of the browser's storage. A coin's
// detail is large, so only those on screen are kept.
const KEPT = [
  'forecast-strategies',
  'forecast-coins',
  'forecast-settings',
  'market-data-settings',
  'market-data-status',
]

const keeps = (query: Query) => {
  const name = String(query.queryKey[0])
  return (
    query.state.status === 'success' &&
    (KEPT.includes(name) || (name === 'forecast-detail' && query.isActive()))
  )
}

// Fills the cache from the last visit before the first render, so the first paint has
// data, then keeps saving it. Everything here is a convenience: blocked or full storage
// only means opening without it.
export function restoreQueryCache() {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null')
    if (saved?.build === BUILD && Date.now() - saved.at < KEPT_FOR_MS) {
      hydrate(queryClient, saved.state)
    }
  } catch {
    /* Opens without it. */
  }
  let waiting = false
  queryClient.getQueryCache().subscribe(() => {
    if (waiting) return
    waiting = true
    window.setTimeout(() => {
      waiting = false
      try {
        localStorage.setItem(
          STORAGE_KEY,
          JSON.stringify({
            build: BUILD,
            at: Date.now(),
            state: dehydrate(queryClient, { shouldDehydrateQuery: keeps }),
          })
        )
      } catch {
        /* The next visit opens without it. */
      }
    }, SAVE_EVERY_MS)
  })
}

// Forgets everything read, on signing out, here and in storage.
export function clearQueryCache() {
  queryClient.clear()
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    /* Nothing to forget. */
  }
}
