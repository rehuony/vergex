import {
  createSeriesMarkers,
  LineStyle,
  type IChartApi,
  type IPriceLine,
  type ISeriesApi,
  type ISeriesMarkersPluginApi,
  type SeriesMarker,
  type Time,
  type UTCTimestamp,
} from 'lightweight-charts'
import { THEME_EVENT } from '@/contexts/theme-context'
import {
  colors,
  createMarketChart,
  themeOptions,
  volumeColor,
} from './market-chart-surface'

export interface ChartCandle {
  time: number // Unix seconds.
  open: number
  high: number
  low: number
  close: number
  volume: number
}

export interface ChartLevel {
  price: number
  title: string
  tone: 'up' | 'down'
}

export interface ChartMarker {
  time: number // Unix seconds.
  tone: 'up' | 'down'
  position: 'above' | 'below'
}

// A read-only chart of candles already held locally. It opens no connection of
// its own: whoever owns the data passes it in.
export class LocalMarketChart {
  private chart: IChartApi | null
  private candles: ISeriesApi<'Candlestick'>
  private volume: ISeriesApi<'Histogram'>
  private markers: ISeriesMarkersPluginApi<Time>
  private lines: IPriceLine[] = []
  private fitted = false

  constructor(host: HTMLElement) {
    const { chart, candles, volume } = createMarketChart(host, (base) => base)
    this.chart = chart
    this.candles = candles
    this.volume = volume
    this.markers = createSeriesMarkers(candles, [])
    window.addEventListener(THEME_EVENT, this.repaint)
  }

  setData(rows: ChartCandle[], levels: ChartLevel[], markers: ChartMarker[]) {
    this.candles.setData(
      rows.map((row) => ({
        time: row.time as UTCTimestamp,
        open: row.open,
        high: row.high,
        low: row.low,
        close: row.close,
      }))
    )
    this.volume.setData(
      rows.map((row) => ({
        time: row.time as UTCTimestamp,
        value: row.volume,
        color: volumeColor(row.close >= row.open),
      }))
    )
    this.lines.forEach((line) => this.candles.removePriceLine(line))
    this.lines = levels.map((level) =>
      this.candles.createPriceLine({
        price: level.price,
        title: level.title,
        color: level.tone === 'up' ? colors.up : colors.down,
        lineStyle: LineStyle.Dashed,
        lineWidth: 1,
        axisLabelVisible: true,
      })
    )
    this.markers.setMarkers(
      markers.map(
        (marker): SeriesMarker<Time> => ({
          time: marker.time as UTCTimestamp,
          position: marker.position === 'above' ? 'aboveBar' : 'belowBar',
          shape: 'circle',
          color: marker.tone === 'up' ? colors.up : colors.down,
          size: 0.6,
        })
      )
    )
    // Fit once; after that the user's scroll and zoom stay where they left them.
    if (!this.fitted && rows.length > 0) {
      this.chart?.timeScale().fitContent()
      this.fitted = true
    }
  }

  private readonly repaint = () => {
    this.chart?.applyOptions(themeOptions())
  }

  dispose() {
    window.removeEventListener(THEME_EVENT, this.repaint)
    this.chart?.remove()
    this.chart = null
  }
}
