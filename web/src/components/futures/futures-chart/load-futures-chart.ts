import type { FuturesChart } from '@/api/futures'

declare global {
  interface Window {
    WorkspaceChart: new (options: Record<string, unknown>) => FuturesChart
  }
}

let chartModule: Promise<void> | null = null
// loadFuturesChart lazy-loads the lightweight-charts workspace chart so the
// charting code stays out of the main bundle, and exposes it on window for
// the Futures page.
export function loadFuturesChart(): Promise<void> {
  if (!chartModule)
    chartModule = import('./futures-chart')
      .then(({ WorkspaceChart }) => {
        window.WorkspaceChart =
          WorkspaceChart as unknown as Window['WorkspaceChart']
      })
      .catch((e) => {
        chartModule = null
        throw new Error(
          'Chart failed to load. Reload the page and try again. ' +
            (e instanceof Error ? e.message : String(e))
        )
      })
  return chartModule
}
