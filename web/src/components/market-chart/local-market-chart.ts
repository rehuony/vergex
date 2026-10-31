import {
  createSeriesMarkers,
  LineSeries,
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
  addProjectionSeries,
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

export interface ChartPoint {
  time: number // Unix seconds.
  value: number
}

// Lines to draw, by name. A name left out is not drawn.
export interface ChartLines {
  ema20?: ChartPoint[]
  ema50?: ChartPoint[]
  bollUpper?: ChartPoint[]
  bollMiddle?: ChartPoint[]
  bollLower?: ChartPoint[]
  vwap?: ChartPoint[]
  rsi?: ChartPoint[]
}

const lineStyles: Record<
  keyof ChartLines,
  { color: () => string; width: 1 | 2; style?: LineStyle }
> = {
  ema20: { color: () => colors.info, width: 1 },
  ema50: { color: () => colors.entry, width: 1 },
  bollUpper: { color: () => colors.text, width: 1, style: LineStyle.Dotted },
  bollMiddle: { color: () => colors.text, width: 1 },
  bollLower: { color: () => colors.text, width: 1, style: LineStyle.Dotted },
  vwap: { color: () => brandColor(), width: 1 },
  rsi: { color: () => colors.info, width: 1 },
}

// What a new coin opens on: recent candles large enough to read, and room to the
// right for the simulated ones.
const INITIAL_BARS = 120
const INITIAL_FUTURE_BARS = 12

const RSI_PANE = 1
const RSI_PANE_HEIGHT = 120

function brandColor() {
  return getComputedStyle(document.documentElement)
    .getPropertyValue('--brand')
    .trim()
}

// Enough decimals to tell prices apart, as the rest of Forecast shows them.
function priceFormat(price: number) {
  const precision = price >= 1000 ? 1 : price >= 1 ? 3 : price >= 0.01 ? 5 : 8
  return { type: 'price' as const, precision, minMove: 10 ** -precision }
}

const toBar = (row: ChartCandle) => ({
  time: row.time as UTCTimestamp,
  open: row.open,
  high: row.high,
  low: row.low,
  close: row.close,
})

const toVolume = (row: ChartCandle) => ({
  time: row.time as UTCTimestamp,
  value: row.volume,
  color: volumeColor(row.close >= row.open),
})

// A read-only chart of candles already held locally. It opens no connection of
// its own: whoever owns the data passes it in.
export class LocalMarketChart {
  private chart: IChartApi | null
  private candles: ISeriesApi<'Candlestick'>
  private volume: ISeriesApi<'Histogram'>
  private markers: ISeriesMarkersPluginApi<Time>
  private projection: ISeriesApi<'Candlestick'>
  private lines = new Map<keyof ChartLines, ISeriesApi<'Line'>>()
  private levels: IPriceLine[] = []
  private fittedFor: string | null = null
  private lastTime = 0
  private live: ChartCandle | null = null

  constructor(host: HTMLElement) {
    const { chart, candles, volume } = createMarketChart(host, (base) => base)
    this.chart = chart
    this.candles = candles
    this.volume = volume
    this.markers = createSeriesMarkers(candles, [])
    this.projection = addProjectionSeries(chart)
    window.addEventListener(THEME_EVENT, this.repaint)
  }

  // fitFor names what is shown: the view fits once for each, and after that the
  // user's scroll and zoom stay where they left them.
  setData(
    rows: ChartCandle[],
    levels: ChartLevel[],
    markers: ChartMarker[],
    fitFor: string
  ) {
    this.candles.setData(rows.map(toBar))
    this.volume.setData(rows.map(toVolume))
    const last = rows.length > 0 ? rows[rows.length - 1] : null
    this.lastTime = last?.time ?? 0
    if (last) {
      const format = priceFormat(last.close)
      this.candles.applyOptions({ priceFormat: format })
      this.projection.applyOptions({ priceFormat: format })
    }
    if (this.live && this.live.time >= this.lastTime) this.update(this.live)

    this.levels.forEach((line) => this.candles.removePriceLine(line))
    this.levels = levels.map((level) =>
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
    if (this.fittedFor !== fitFor && rows.length > 0) {
      this.chart?.timeScale().setVisibleLogicalRange({
        from: Math.max(0, rows.length - INITIAL_BARS),
        to: rows.length + INITIAL_FUTURE_BARS,
      })
      this.fittedFor = fitFor
    }
  }

  // update moves the forming candle along, between full refreshes.
  update(row: ChartCandle) {
    if (row.time < this.lastTime) return
    this.live = row
    this.lastTime = row.time
    this.candles.update(toBar(row))
    this.volume.update(toVolume(row))
  }

  // forget drops the forming candle, when what is shown changes.
  forget() {
    this.live = null
  }

  setProjection(rows: ChartCandle[]) {
    this.projection.setData(rows.map(toBar))
  }

  setLines(lines: ChartLines) {
    for (const name of Object.keys(lineStyles) as (keyof ChartLines)[]) {
      const points = lines[name]
      const held = this.lines.get(name)
      if (!points) {
        if (held) this.chart?.removeSeries(held)
        this.lines.delete(name)
        continue
      }
      const series = held ?? this.addLine(name)
      series?.setData(
        points.map((point) => ({
          time: point.time as UTCTimestamp,
          value: point.value,
        }))
      )
    }
    this.layoutRSI()
  }

  private addLine(name: keyof ChartLines) {
    if (!this.chart) return null
    const style = lineStyles[name]
    const series = this.chart.addSeries(
      LineSeries,
      {
        color: style.color(),
        lineWidth: style.width,
        lineStyle: style.style ?? LineStyle.Solid,
        priceLineVisible: false,
        lastValueVisible: false,
        crosshairMarkerVisible: false,
      },
      name === 'rsi' ? RSI_PANE : 0
    )
    this.lines.set(name, series)
    if (name === 'rsi') {
      for (const price of [30, 70]) {
        series.createPriceLine({
          price,
          color: colors.text,
          lineStyle: LineStyle.Dotted,
          lineWidth: 1,
          axisLabelVisible: false,
        })
      }
    }
    return series
  }

  private layoutRSI() {
    const panes = this.chart?.panes() ?? []
    if (panes.length > RSI_PANE && this.lines.has('rsi'))
      panes[RSI_PANE].setHeight(RSI_PANE_HEIGHT)
  }

  // A time past the last real candle is on a simulated one.
  onCrosshair(listener: (onProjection: boolean) => void) {
    this.chart?.subscribeCrosshairMove((param) => {
      listener(typeof param.time === 'number' && param.time > this.lastTime)
    })
  }

  private readonly repaint = () => {
    this.chart?.applyOptions(themeOptions())
    this.lines.get('vwap')?.applyOptions({ color: brandColor() })
  }

  dispose() {
    window.removeEventListener(THEME_EVENT, this.repaint)
    this.chart?.remove()
    this.chart = null
  }
}
