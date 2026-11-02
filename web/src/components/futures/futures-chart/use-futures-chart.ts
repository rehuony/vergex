import {
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { type Plan, type FuturesChart, type FuturesState } from '@/api/futures'
import { loadFuturesChart } from './load-futures-chart'

type PlanRecord = FuturesState['records'][string]

// useFuturesChart mounts the Futures market chart for the open market,
// reports its live price, and plots the selected plan or the
// plans being compared.
export function useFuturesChart(
  venue: string,
  symbol: string,
  current: PlanRecord | undefined,
  compare: string[],
  records: FuturesState['records'],
  runState: string | undefined,
  setError: Dispatch<SetStateAction<string>>
) {
  const chart = useRef<FuturesChart | null>(null)
  const [price, setPrice] = useState('—')
  // What the chart shows, once it exists.
  const [interval, setInterval] = useState('')
  const [resolution, setResolution] = useState('60')
  // Goes up each time the chart starts a new scene, which clears what was drawn on it.
  const [scene, setScene] = useState(0)
  useEffect(() => {
    let cancelled = false
    let chartFailure = ''
    const reportChartFailure = (message: string) => {
      if (cancelled) return
      chartFailure = message
      setError(message)
    }
    loadFuturesChart()
      .then(() => {
        if (cancelled) return
        chart.current?.dispose()
        chart.current = new window.WorkspaceChart({
          venue,
          symbol,
          onStatus: () => {},
          onPrice: (c: { close: string }) => !cancelled && setPrice(c.close),
          onContext: (context: { interval: string; resolution: string }) => {
            if (cancelled) return
            setInterval(context.interval)
            setResolution(context.resolution)
            setScene((current) => current + 1)
          },
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
  const changeResolution = (next: string) => {
    chart.current?.setResolution(next).catch((e) => setError(e.message))
  }
  return {
    chart,
    price,
    setPrice,
    interval,
    resolution,
    changeResolution,
    scene,
  }
}
