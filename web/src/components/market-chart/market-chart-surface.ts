import {
  CandlestickSeries,
  ColorType,
  CrosshairMode,
  HistogramSeries,
  createChart,
  type AutoscaleInfo,
  type IChartApi,
} from 'lightweight-charts'

// The market chart's surface on lightweight-charts, shared by Frontier and
// Forecast: colours, theme and the candle and volume series.

export const colors = {
  up: '#2E8B57',
  down: '#D6433A',
  entry: '#9b6a14',
  info: '#3575a8',
  text: '#71819a',
  grid: '#eef1f6',
}
// The volume bar under a candle.
export const volumeColor = (up: boolean) =>
  up ? 'rgba(46,139,87,0.25)' : 'rgba(214,67,58,0.25)'

// Surface, axis text and grid follow the page theme; light keeps the chart's
// own quieter grid, dark reads the theme tokens.
export function themeColors() {
  const root = document.documentElement
  if (root.dataset.theme !== 'dark')
    return {
      background: '#FFFFFF',
      text: colors.text,
      grid: colors.grid,
      label: 'rgba(255,255,255,0.85)',
    }
  const css = getComputedStyle(root)
  const token = (name: string) => css.getPropertyValue(name).trim()
  return {
    background: token('--surface'),
    text: token('--muted'),
    grid: token('--line'),
    label: 'rgba(23,27,34,0.85)',
  }
}

// themeOptions repaints the chart chrome for the current page theme.
export function themeOptions() {
  const theme = themeColors()
  return {
    layout: {
      background: { type: ColorType.Solid, color: theme.background },
      textColor: theme.text,
    },
    grid: {
      vertLines: { color: theme.grid },
      horzLines: { color: theme.grid },
    },
  }
}

// createMarketChart creates the candle and volume series on host.
export function createMarketChart(
  host: HTMLElement,
  autoscale: (base: AutoscaleInfo | null) => AutoscaleInfo | null
) {
  const theme = themeColors()
  const chart = createChart(host, {
    autoSize: true,
    layout: {
      background: { type: ColorType.Solid, color: theme.background },
      textColor: theme.text,
      attributionLogo: false,
    },
    grid: {
      vertLines: { color: theme.grid },
      horzLines: { color: theme.grid },
    },
    crosshair: { mode: CrosshairMode.Normal },
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
    upColor: colors.up,
    downColor: colors.down,
    borderUpColor: colors.up,
    borderDownColor: colors.down,
    wickUpColor: colors.up,
    wickDownColor: colors.down,
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
const projectionFill = (up: boolean) =>
  up ? 'rgba(46,139,87,0.18)' : 'rgba(214,67,58,0.18)'
const projectionEdge = (up: boolean) =>
  up ? 'rgba(46,139,87,0.65)' : 'rgba(214,67,58,0.65)'

// addProjectionSeries adds the series that draws a forecast as simulated candles.
export function addProjectionSeries(chart: IChartApi) {
  return chart.addSeries(CandlestickSeries, {
    upColor: projectionFill(true),
    downColor: projectionFill(false),
    borderUpColor: projectionEdge(true),
    borderDownColor: projectionEdge(false),
    wickUpColor: projectionEdge(true),
    wickDownColor: projectionEdge(false),
    priceLineVisible: false,
    lastValueVisible: false,
  })
}
