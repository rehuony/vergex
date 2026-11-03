import {
  CandlestickSeries,
  ColorType,
  CrosshairMode,
  HistogramSeries,
  LineSeries,
  createChart,
  type AutoscaleInfo,
  type IChartApi,
  type IPrimitivePaneRenderer,
  type IPrimitivePaneView,
  type ISeriesPrimitive,
  type SeriesAttachedParameter,
  type Time,
  type UTCTimestamp,
} from 'lightweight-charts'
import type { ChartForecast } from '@/api/forecast'

// The market chart's surface on lightweight-charts, shared by Futures and
// Forecast: colours, theme and the candle and volume series.

const token = (name: string) =>
  getComputedStyle(document.documentElement).getPropertyValue(name).trim()

// A hex token at an opacity, for what the chart library draws see-through.
const tint = (hex: string, opacity: number) => {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16))
  return `rgba(${r},${g},${b},${opacity})`
}

// The theme's tokens, read each time something is drawn, so a repaint after the
// theme changes picks up the new values.
export const colors = {
  get up() {
    return token('--success')
  },
  get down() {
    return token('--danger')
  },
  get ink() {
    return token('--ink')
  },
  get inkSoft() {
    return token('--ink-soft')
  },
  get muted() {
    return token('--muted')
  },
  get faint() {
    return token('--faint')
  },
}

// The volume bar under a candle.
export const volumeColor = (up: boolean) =>
  tint(up ? colors.up : colors.down, 0.25)

// What the overlay paints over the candles: a backing for its labels and their
// font.
export function overlayStyle() {
  return { label: tint(token('--surface'), 0.85), font: token('--font-sans') }
}

// themeOptions repaints the chart chrome for the current page theme.
export function themeOptions() {
  const grid = token('--line')
  const crosshair = colors.faint
  return {
    layout: {
      background: { type: ColorType.Solid, color: token('--surface') },
      textColor: colors.muted,
    },
    grid: {
      vertLines: { color: grid },
      horzLines: { color: grid },
    },
    crosshair: {
      vertLine: { color: crosshair },
      horzLine: { color: crosshair },
    },
  }
}

// candleColors repaints the candles for the current page theme.
export function candleColors() {
  const up = colors.up
  const down = colors.down
  return {
    upColor: up,
    downColor: down,
    borderUpColor: up,
    borderDownColor: down,
    wickUpColor: up,
    wickDownColor: down,
  }
}

// priceMargins is the room above and below the candles, as shares of the pane. The
// bottom holds the volume lane; a chart may ask for more at the top for what floats
// there.
export const priceMargins = { top: 0.08, bottom: 0.16 }

// createMarketChart creates the candle and volume series on host.
export function createMarketChart(
  host: HTMLElement,
  autoscale: (base: AutoscaleInfo | null) => AutoscaleInfo | null
) {
  const theme = themeOptions()
  const chart = createChart(host, {
    autoSize: true,
    layout: {
      ...theme.layout,
      fontFamily: token('--font-mono'),
      attributionLogo: false,
    },
    grid: theme.grid,
    crosshair: { ...theme.crosshair, mode: CrosshairMode.Normal },
    rightPriceScale: {
      borderVisible: false,
      scaleMargins: priceMargins,
    },
    timeScale: {
      borderVisible: false,
      timeVisible: true,
      secondsVisible: false,
      rightOffset: 8,
    },
  })
  const candles = chart.addSeries(CandlestickSeries, {
    ...candleColors(),
    autoscaleInfoProvider: (base: () => AutoscaleInfo | null) =>
      autoscale(base()),
  })
  const volume = chart.addSeries(HistogramSeries, {
    priceScaleId: 'volume',
    priceFormat: { type: 'volume' },
    lastValueVisible: false,
    priceLineVisible: false,
  })
  // The volume lane takes the bottom of the pane, under the candles' own margin.
  chart.priceScale('volume').applyOptions({
    scaleMargins: { top: 0.88, bottom: 0 },
  })
  return { chart, candles, volume }
}

// forecastPath lays a forecast out bar by bar. Its middle drifts in the call's
// direction as far as the score says, up to the expected move at the target, and the
// band around it widens as the square root of time to the expected move. A neutral
// call, or a score a curve has put on the other side of 50 from the call, drifts
// nowhere.
export function forecastPath(forecast: ChartForecast) {
  const move = forecast.expectedMovePct / 100
  let drift = ((forecast.prob - 0.5) / 0.5) * move
  if (
    forecast.direction === 'neutral' ||
    (forecast.direction === 'up') !== drift > 0
  )
    drift = 0
  const points = []
  for (let bar = 0; bar <= forecast.horizon; bar++) {
    const share = bar / forecast.horizon
    points.push({
      time: forecast.time + bar * forecast.step,
      mid: forecast.close * (1 + drift * share),
      half: forecast.close * move * Math.sqrt(share),
    })
  }
  return points
}

// addHorizonSeries adds a series with nothing to draw, whose whitespace points keep
// the forecast's horizon on the time scale, so the band has somewhere to go.
export function addHorizonSeries(chart: IChartApi) {
  return chart.addSeries(LineSeries, {
    priceLineVisible: false,
    lastValueVisible: false,
    crosshairMarkerVisible: false,
  })
}

// horizonPoints are the whitespace points of a forecast's horizon.
export function horizonPoints(forecast: ChartForecast | null) {
  if (!forecast) return []
  return forecastPath(forecast)
    .slice(1)
    .map((point) => ({ time: point.time as UTCTimestamp }))
}

type RenderTarget = Parameters<IPrimitivePaneRenderer['draw']>[0]

// ForecastBand draws a forecast on the candle series it is attached to: a see-through
// band in the call's colour from the last close to the target candle, with a dashed
// middle. It only restates the reading, which is why it is drawn faint: only the
// direction of these calls has been tested, not the size or the path.
export class ForecastBand implements ISeriesPrimitive<Time> {
  private forecast: ChartForecast | null = null
  private params: SeriesAttachedParameter<Time> | null = null
  private readonly views: IPrimitivePaneView[] = [
    {
      zOrder: () => 'normal',
      renderer: () => ({
        draw: (target: RenderTarget) => this.paint(target),
      }),
    },
  ]
  attached(params: SeriesAttachedParameter<Time>) {
    this.params = params
  }
  detached() {
    this.params = null
  }
  paneViews() {
    return this.views
  }
  set(forecast: ChartForecast | null) {
    this.forecast = forecast
    this.params?.requestUpdate()
  }
  // Draws again in the theme's colours.
  repaint() {
    this.params?.requestUpdate()
  }
  private paint(target: RenderTarget) {
    const params = this.params
    const forecast = this.forecast
    if (!params || !forecast || forecast.horizon <= 0) return
    const tone =
      forecast.direction === 'up'
        ? colors.up
        : forecast.direction === 'down'
          ? colors.down
          : colors.muted
    const scale = params.chart.timeScale()
    const points: { x: number; mid: number; high: number; low: number }[] = []
    for (const point of forecastPath(forecast)) {
      const x = scale.timeToCoordinate(point.time as UTCTimestamp)
      const mid = params.series.priceToCoordinate(point.mid)
      const high = params.series.priceToCoordinate(point.mid + point.half)
      const low = params.series.priceToCoordinate(point.mid - point.half)
      if (x === null || mid === null || high === null || low === null) return
      points.push({ x, mid, high, low })
    }
    target.useMediaCoordinateSpace(({ context: ctx }) => {
      ctx.beginPath()
      points.forEach((p, i) =>
        i === 0 ? ctx.moveTo(p.x, p.high) : ctx.lineTo(p.x, p.high)
      )
      for (let i = points.length - 1; i >= 0; i--)
        ctx.lineTo(points[i].x, points[i].low)
      ctx.closePath()
      ctx.fillStyle = tint(tone, 0.12)
      ctx.fill()
      ctx.strokeStyle = tint(tone, 0.45)
      ctx.lineWidth = 1
      ctx.setLineDash([])
      ctx.stroke()
      ctx.beginPath()
      ctx.setLineDash([4, 3])
      ctx.strokeStyle = tint(tone, 0.9)
      ctx.lineWidth = 1.5
      points.forEach((p, i) =>
        i === 0 ? ctx.moveTo(p.x, p.mid) : ctx.lineTo(p.x, p.mid)
      )
      ctx.stroke()
      ctx.setLineDash([])
    })
  }
}
