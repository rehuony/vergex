import {
  CandlestickSeries,
  ColorType,
  CrosshairMode,
  HistogramSeries,
  createChart,
  type AutoscaleInfo,
  type IChartApi,
} from 'lightweight-charts'

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
      scaleMargins: { top: 0.08, bottom: 0.22 },
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
  chart.priceScale('volume').applyOptions({
    scaleMargins: { top: 0.82, bottom: 0 },
  })
  return { chart, candles, volume }
}

// A simulated candle must not pass for a real one: mostly see-through, with only
// its outline at some strength.
export function projectionColors() {
  const up = colors.up
  const down = colors.down
  return {
    upColor: tint(up, 0.18),
    downColor: tint(down, 0.18),
    borderUpColor: tint(up, 0.65),
    borderDownColor: tint(down, 0.65),
    wickUpColor: tint(up, 0.65),
    wickDownColor: tint(down, 0.65),
  }
}

// addProjectionSeries adds the series that draws a forecast as simulated candles.
export function addProjectionSeries(chart: IChartApi) {
  return chart.addSeries(CandlestickSeries, {
    ...projectionColors(),
    priceLineVisible: false,
    lastValueVisible: false,
  })
}
