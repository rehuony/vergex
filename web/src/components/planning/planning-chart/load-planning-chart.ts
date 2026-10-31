import type { PlanningChart } from '@/api/planning'

declare global {
  interface Window {
    WorkspaceChart: new (options: Record<string, unknown>) => PlanningChart
  }
}

let chartModule: Promise<void> | null = null
// loadPlanningChart lazy-loads the lightweight-charts workspace chart so the
// charting code stays out of the main bundle, and exposes it on window for
// the Frontier page.
export function loadPlanningChart(): Promise<void> {
  if (!chartModule)
    chartModule = import('./planning-chart')
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
