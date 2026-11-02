import { useEffect, useState } from 'react'
import type { ForecastCandle } from '@/api/forecast'

interface FeedMessage {
  type: string
  data?: {
    candles?: {
      time: number
      open: string
      high: string
      low: string
      close: string
      volume: string
    }[]
  }
}

// The candle now forming on a coin, from the live stream. live=1 asks the feed for
// the stream alone, so a chart here never spends a request. It stays null while the
// stream is down, and the page's own refresh carries on.
export function useLiveCandle(symbol: string, interval: string) {
  const [candle, setCandle] = useState<ForecastCandle | null>(null)
  useEffect(() => {
    setCandle(null)
    if (!symbol || typeof EventSource === 'undefined') return
    const query = new URLSearchParams({
      venue: 'binance',
      symbol,
      interval,
      live: '1',
    })
    const source = new EventSource(`/api/futures/feed?${query}`)
    source.onmessage = (event) => {
      let message: FeedMessage
      try {
        message = JSON.parse(event.data)
      } catch {
        return
      }
      const rows = message.data?.candles ?? []
      const row = rows[rows.length - 1]
      if (message.type !== 'candles' || !row) return
      setCandle({
        time: row.time,
        open: Number(row.open),
        high: Number(row.high),
        low: Number(row.low),
        close: Number(row.close),
        volume: Number(row.volume),
      })
    }
    return () => source.close()
  }, [symbol, interval])
  return candle
}
