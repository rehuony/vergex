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
import { wheelIntent } from './chart-input'
import { priceDigits } from './chart-legend'
import {
  addProjectionSeries,
  candleColors,
  colors,
  createMarketChart,
  projectionColors,
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

// The lines stay achromatic, told apart by shade and dash: green and red are
// for the market's own meaning.
const lineStyles: Record<
  keyof ChartLines,
  { color: () => string; width: 1 | 2; style?: LineStyle }
> = {
  ema20: { color: () => colors.inkSoft, width: 1 },
  ema50: { color: () => colors.muted, width: 1, style: LineStyle.Dashed },
  bollUpper: { color: () => colors.faint, width: 1, style: LineStyle.Dotted },
  bollMiddle: { color: () => colors.faint, width: 1 },
  bollLower: { color: () => colors.faint, width: 1, style: LineStyle.Dotted },
  vwap: { color: () => colors.ink, width: 1 },
  rsi: { color: () => colors.inkSoft, width: 1 },
}

// What a new coin opens on: recent candles large enough to read, and room to the
// right for the simulated ones.
const INITIAL_BARS = 120
const INITIAL_FUTURE_BARS = 12

// How far the zoom buttons go at a press, and how few candles they stop at.
const ZOOM_STEP = 0.75
const MIN_ZOOM_BARS = 12

const RSI_PANE = 1
const RSI_PANE_HEIGHT = 120

function priceFormat(price: number) {
  const precision = priceDigits(price)
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
  private rsiLevels: IPriceLine[] = []
  // What setData last drew, to draw again in the new colours.
  private shown: Parameters<LocalMarketChart['setData']> | null = null
  private fittedFor: string | null = null
  private lastTime = 0
  private live: ChartCandle | null = null
  private wheelZooms = false

  constructor(private readonly host: HTMLElement) {
    const { chart, candles, volume } = createMarketChart(host, (base) => base)
    this.chart = chart
    this.candles = candles
    this.volume = volume
    this.markers = createSeriesMarkers(candles, [])
    this.projection = addProjectionSeries(chart)
    // An upright swipe on a phone scrolls the page, and the wheel is decided in onWheel.
    chart.applyOptions({
      handleScale: { mouseWheel: false },
      handleScroll: { vertTouchDrag: false },
    })
    host.addEventListener('wheel', this.onWheel, {
      capture: true,
      passive: true,
    })
    window.addEventListener(THEME_EVENT, this.repaint)
  }

  // Runs before the library sees the wheel. A scroll meant for the page never reaches it,
  // so nothing stops the page moving, and it only zooms while Ctrl or Command is held.
  private readonly onWheel = (event: WheelEvent) => {
    const intent = wheelIntent(event)
    if (intent === 'page') {
      event.stopPropagation()
      return
    }
    const zooms = intent === 'zoom'
    if (zooms !== this.wheelZooms) {
      this.wheelZooms = zooms
      this.chart?.applyOptions({ handleScale: { mouseWheel: zooms } })
    }
  }

  // Shows fewer candles for a factor under 1 and more over it, keeping the right edge,
  // where the newest candles and the forecast are.
  zoom(factor: number) {
    const scale = this.chart?.timeScale()
    const range = scale?.getVisibleLogicalRange()
    if (!scale || !range) return
    const span = Math.max(MIN_ZOOM_BARS, (range.to - range.from) * factor)
    scale.setVisibleLogicalRange({ from: range.to - span, to: range.to })
  }

  zoomIn() {
    this.zoom(ZOOM_STEP)
  }

  zoomOut() {
    this.zoom(1 / ZOOM_STEP)
  }

  // Goes back to what a new coin opens on, wherever the view has been moved to.
  resetView() {
    const rows = this.shown?.[0].length ?? 0
    if (rows === 0) return
    this.chart?.priceScale('right').applyOptions({ autoScale: true })
    this.chart?.timeScale().setVisibleLogicalRange({
      from: Math.max(0, rows - INITIAL_BARS),
      to: rows + INITIAL_FUTURE_BARS,
    })
  }

  // Says which candle the pointer is over, by its time, or null when it is off them.
  onCrosshair(listener: (time: number | null) => void) {
    this.chart?.subscribeCrosshairMove((param) => {
      listener(typeof param.time === 'number' ? param.time : null)
    })
  }

  // fitFor names what is shown: the view fits once for each, and after that the
  // user's scroll and zoom stay where they left them.
  setData(
    rows: ChartCandle[],
    levels: ChartLevel[],
    markers: ChartMarker[],
    fitFor: string
  ) {
    this.shown = [rows, levels, markers, fitFor]
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
      this.resetView()
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
      this.rsiLevels = [30, 70].map((price) =>
        series.createPriceLine({
          price,
          color: colors.faint,
          lineStyle: LineStyle.Dotted,
          lineWidth: 1,
          axisLabelVisible: false,
        })
      )
    }
    return series
  }

  private layoutRSI() {
    const panes = this.chart?.panes() ?? []
    if (panes.length > RSI_PANE && this.lines.has('rsi'))
      panes[RSI_PANE].setHeight(RSI_PANE_HEIGHT)
  }

  private readonly repaint = () => {
    this.chart?.applyOptions(themeOptions())
    this.candles.applyOptions(candleColors())
    this.projection.applyOptions(projectionColors())
    for (const [name, series] of this.lines)
      series.applyOptions({ color: lineStyles[name].color() })
    for (const line of this.rsiLevels)
      line.applyOptions({ color: colors.faint })
    if (this.shown) this.setData(...this.shown)
  }

  dispose() {
    this.host.removeEventListener('wheel', this.onWheel, { capture: true })
    window.removeEventListener(THEME_EVENT, this.repaint)
    this.chart?.remove()
    this.chart = null
  }
}
