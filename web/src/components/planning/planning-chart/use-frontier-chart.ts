import {
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import {
  type Plan,
  type PlanningChart,
  type PlanningState,
} from '@/api/planning'
import { loadPlanningChart } from './load-planning-chart'

type PlanRecord = PlanningState['records'][string]

// useFrontierChart mounts the Frontier market chart for the open market,
// reports its live price, and plots the selected plan or the
// plans being compared.
export function useFrontierChart(
  venue: string,
  symbol: string,
  current: PlanRecord | undefined,
  compare: string[],
  records: PlanningState['records'],
  runState: string | undefined,
  setError: Dispatch<SetStateAction<string>>
) {
  const chart = useRef<PlanningChart | null>(null)
  const [price, setPrice] = useState('—')
  // What the chart shows, once it exists.
  const [interval, setInterval] = useState('')
  // Goes up each time the chart starts a new scene, which clears what was drawn on it.
  const [scene, setScene] = useState(0)
  const [onForecast, setOnForecast] = useState(false)
  useEffect(() => {
    let cancelled = false
    let chartFailure = ''
    const reportChartFailure = (message: string) => {
      if (cancelled) return
      chartFailure = message
      setError(message)
    }
    loadPlanningChart()
      .then(() => {
        if (cancelled) return
        chart.current?.dispose()
        chart.current = new window.WorkspaceChart({
          venue,
          symbol,
          onStatus: () => {},
          onPrice: (c: { close: string }) => !cancelled && setPrice(c.close),
          onContext: (context: { interval: string }) => {
            if (cancelled) return
            setInterval(context.interval)
            setScene((current) => current + 1)
          },
          onForecastHover: (hovering: boolean) =>
            !cancelled && setOnForecast(hovering),
          onError: reportChartFailure,
          onAvailability: (available: boolean) => {
            if (cancelled) return
            if (available && chartFailure)
              setError((message) => (message === chartFailure ? '' : message))
          },
        })
        chart.current
          .setContext(venue, symbol, '60')
          .catch((e) => reportChartFailure(e.message))
      })
      .catch((e) => reportChartFailure(e.message))
    return () => {
      cancelled = true
      chart.current?.dispose()
      chart.current = null
    }
  }, [venue, symbol])
  useEffect(() => {
    if (!chart.current) return
    const list = compare
      .map((id) => records[id]?.plan)
      .filter(
        (p): p is Plan =>
          !!p && p.snapshot.symbol === symbol && p.snapshot.venue === venue
      )
    chart.current
      .plot(
        current?.plan || null,
        runState || current?.state || 'draft',
        list.length >= 2 ? list : []
      )
      .catch((e) => setError(e.message))
  }, [current, compare, records, symbol, venue, runState, scene])
  return { chart, price, setPrice, interval, scene, onForecast }
}
