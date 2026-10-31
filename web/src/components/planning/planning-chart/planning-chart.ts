import {
  LineStyle,
  type AutoscaleInfo,
  type IChartApi,
  type IPriceLine,
  type ISeriesApi,
} from 'lightweight-charts'
import type { ChartContext, Drawing, Plan, PlanningChart } from '@/api/planning'
import { THEME_EVENT } from '@/contexts/theme-context'
import {
  type CandleResponse,
  type CandleRow,
  chartQuery,
  fetchHistory,
  fetchJSON,
  instrument,
  logicalAt,
  resolutionSeconds,
  resolutions,
  ticker,
  toBar,
  toVolume,
} from './planning-chart-data'
import {
  colors,
  createMarketChart,
  themeOptions,
} from '@/components/market-chart/market-chart-surface'
import {
  OverlayPrimitive,
  chartFrame,
  type Shape,
} from './planning-chart-surface'
import {
  annotationKind,
  comparisonShapes,
  drawingShapes,
  levelPrices,
  planPriceLines,
  planZones,
  selectAnnotations,
} from './planning-chart-shapes'

// Frontier market chart on lightweight-charts. Candles come from
// /api/planning/candles (history) and /api/planning/feed (SSE, ~3 s).
// Plan levels, plan comparisons and AI drawings are painted on top.

const initialBars = 500
const maxBars = 4800

export interface WorkspaceChartOptions {
  venue: string
  symbol: string
  resolution?: string
  container?: HTMLElement | string
  onStatus?: (text: string, warning: boolean) => void
  onPrice?: (candle: CandleRow, data: CandleResponse) => void
  onContext?: (context: ChartContext) => void
  onError?: (message: string) => void
  onAvailability?: (available: boolean, message?: string) => void
}

export class WorkspaceChart implements PlanningChart {
  private venue: string
  private symbol: string
  private resolution: string
  private readonly onStatus: NonNullable<WorkspaceChartOptions['onStatus']>
  private readonly onPrice: NonNullable<WorkspaceChartOptions['onPrice']>
  private readonly onContext: NonNullable<WorkspaceChartOptions['onContext']>
  private readonly onError: NonNullable<WorkspaceChartOptions['onError']>
  private readonly onAvailability: NonNullable<
    WorkspaceChartOptions['onAvailability']
  >
  private root: HTMLDivElement | null = null
  private toolbar: HTMLDivElement | null = null
  private chart: IChartApi | null = null
  private candles: ISeriesApi<'Candlestick'> | null = null
  private volume: ISeriesApi<'Histogram'> | null = null
  private overlay: OverlayPrimitive
  private loadError = ''
  private disposed = false
  private scene = 0
  private rows: CandleRow[] = []
  private times: number[] = []
  private exhausted = false
  private loadingOlder = false
  private source: EventSource | null = null
  private lastReceived = 0
  private ageTimer: ReturnType<typeof setInterval> | undefined
  private plan: Plan | null = null
  private comparisonPlans: Plan[] = []
  private focused = false
  private planLines: IPriceLine[] = []
  private planShapes: Shape[] = []
  private aiLines: IPriceLine[] = []
  private aiShapes: Shape[] = []
  private aiCandidates: Drawing[] = []
  private manualShapes: Shape[] = []
  private manualLines: IPriceLine[] = []

  constructor(options: WorkspaceChartOptions) {
    this.venue = options.venue
    this.symbol = options.symbol
    this.resolution = resolutions[options.resolution || '']
      ? (options.resolution as string)
      : '60'
    this.onStatus = options.onStatus || (() => {})
    this.onPrice = options.onPrice || (() => {})
    this.onContext = options.onContext || (() => {})
    this.onError = options.onError || (() => {})
    this.onAvailability = options.onAvailability || (() => {})
    this.overlay = new OverlayPrimitive((time) =>
      logicalAt(this.times, this.step(), time)
    )
    const container =
      typeof options.container === 'string' || !options.container
        ? document.getElementById(options.container || 'chart-container')
        : options.container
    if (!container) {
      this.fail('Chart container is missing. Reload the page and try again.')
      return
    }
    try {
      this.mount(container)
    } catch (e) {
      this.fail(
        'Chart failed to start: ' + (e instanceof Error ? e.message : String(e))
      )
      return
    }
    this.ageTimer = setInterval(() => {
      if (this.lastReceived && Date.now() - this.lastReceived > 15000)
        this.onStatus('Market updates interrupted · stale data', true)
    }, 1000)
  }

  private fail(message: string) {
    this.loadError = message
    this.onAvailability(false, message)
    this.onError(message)
  }

  private mount(container: HTMLElement) {
    const { root, toolbar, host } = chartFrame((resolution) => {
      this.setResolution(resolution).catch((e) => this.onError(e.message))
    })
    container.append(root)
    this.root = root
    this.toolbar = toolbar
    this.markResolution()

    const { chart, candles, volume } = createMarketChart(host, (base) =>
      this.autoscale(base)
    )
    candles.attachPrimitive(this.overlay)
    chart.timeScale().subscribeVisibleLogicalRangeChange((range) => {
      if (range && range.from < 20) this.loadOlder()
    })
    this.chart = chart
    window.addEventListener(THEME_EVENT, this.repaintTheme)
    this.candles = candles
    this.volume = volume
  }

  private markResolution() {
    for (const button of this.toolbar?.querySelectorAll('button') || [])
      button.setAttribute(
        'aria-pressed',
        String(button.dataset.resolution === this.resolution)
      )
  }

  private step() {
    return resolutionSeconds[this.resolution]
  }

  private key(extra: Record<string, string> = {}) {
    return chartQuery(ticker(this.venue, this.symbol), this.resolution, extra)
  }

  context(): ChartContext {
    let visible: { from: number; to: number } | undefined
    try {
      const range = this.chart?.timeScale().getVisibleRange()
      if (range) {
        // The viewport includes future whitespace, which is not market data.
        const from = Math.max(0, Math.floor(Number(range.from)))
        const to = Math.min(
          Math.floor(Number(range.to)),
          Math.floor(Date.now() / 1000) + 1
        )
        if (Number.isFinite(from) && Number.isFinite(to) && to > from)
          visible = { from, to }
      }
    } catch {
      /* time scale has no data yet */
    }
    return {
      venue: this.venue,
      symbol: this.symbol,
      interval: resolutions[this.resolution],
      resolution: this.resolution,
      ...(visible
        ? { visible_from: visible.from, visible_to: visible.to }
        : {}),
    }
  }

  async whenReady() {
    if (this.disposed || !this.chart)
      throw new Error(this.loadError || 'Chart is not ready yet. Try again.')
  }

  private history(to: number, count: number) {
    return fetchHistory((extra) => this.key(extra), to, count)
  }

  private setRows(rows: CandleRow[]) {
    this.rows = rows.slice(-maxBars)
    this.times = this.rows.map((r) => r.time)
    this.candles?.setData(this.rows.map(toBar))
    this.volume?.setData(this.rows.map(toVolume))
  }

  private async loadPrecision(scene: number) {
    try {
      const quote = await fetchJSON<{ tick?: string }>(
        '/api/planning/instrument?' +
          new URLSearchParams({ venue: this.venue, symbol: this.symbol })
      )
      const tick = Number(quote.tick)
      if (scene !== this.scene || !(tick > 0)) return
      const precision = (
        String(quote.tick).replace(/0+$/, '').split('.')[1] || ''
      ).length
      this.candles?.applyOptions({
        priceFormat: { type: 'price', precision, minMove: tick },
      })
    } catch {
      /* Axis precision is display-only; executable plans validate tick rules separately. */
    }
  }

  private async loadScene(scene: number) {
    this.closeFeed()
    this.rows = []
    this.times = []
    this.exhausted = false
    this.lastReceived = 0
    this.setRows([])
    this.onAvailability(false, 'Loading ' + this.symbol + ' candles…')
    this.onStatus('Connecting market feed…', false)
    this.loadPrecision(scene)
    let data
    try {
      data = await this.history(Math.floor(Date.now() / 1000) + 1, initialBars)
    } catch (e) {
      if (scene !== this.scene || this.disposed) return
      const message = e instanceof Error ? e.message : String(e)
      this.onAvailability(
        false,
        'Could not load candles for this market: ' + message
      )
      this.onStatus('Market data read failed', true)
      this.onError(message)
      return
    }
    if (scene !== this.scene || this.disposed) return
    this.exhausted = data.exhausted
    this.setRows(data.candles)
    if (!this.rows.length) {
      this.onAvailability(
        false,
        'The exchange returned no candles for this market. Check the symbol and try again.'
      )
      this.onStatus('No candles available for this market', true)
    } else {
      const n = this.rows.length
      this.chart?.timeScale().setVisibleLogicalRange({
        from: Math.max(0, n - 120),
        to: n + 8,
      })
      this.onAvailability(true)
      this.onPrice(this.rows[n - 1], data)
    }
    this.render()
    this.connectFeed(scene)
  }

  private async loadOlder() {
    if (this.loadingOlder || this.exhausted || !this.rows.length) return
    if (this.rows.length >= maxBars) return
    const scene = this.scene
    this.loadingOlder = true
    try {
      const data = await this.history(this.rows[0].time, 300)
      if (scene !== this.scene || this.disposed) return
      if (data.exhausted || !data.candles.length) this.exhausted = true
      const merged = new Map(this.rows.map((r) => [r.time, r]))
      for (const row of data.candles) merged.set(row.time, row)
      this.setRows([...merged.values()].sort((a, b) => a.time - b.time))
    } catch {
      /* scrolling further back is best effort */
    } finally {
      this.loadingOlder = false
    }
  }

  private connectFeed(scene: number) {
    if (this.disposed) return
    const source = new EventSource('/api/planning/feed?' + this.key())
    this.source = source
    let pending = Promise.resolve()
    source.onmessage = (event) => {
      pending = pending.then(() => this.receive(scene, source, event.data))
    }
    source.onerror = () => {
      if (this.source === source)
        this.onStatus('Market feed disconnected · reconnecting', true)
    }
  }

  private closeFeed() {
    this.source?.close()
    this.source = null
  }

  private async receive(scene: number, source: EventSource, raw: string) {
    if (this.source !== source || scene !== this.scene) return
    let message: { type: string; data: CandleResponse & { message?: string } }
    try {
      message = JSON.parse(raw)
    } catch {
      this.onStatus('Invalid market data', true)
      return
    }
    if (message.type === 'feed_error') {
      this.onStatus('Market feed unavailable · reconnecting', true)
      if (!this.rows.length)
        this.onAvailability(
          false,
          'Could not load candles for this market: ' +
            (message.data?.message || 'Market feed connection failed')
        )
      return
    }
    if (message.type !== 'candles') return
    const data = message.data
    const rows = (data.candles || []).slice().sort((a, b) => a.time - b.time)
    if (!rows.length) {
      this.onStatus('No candles available for this market', true)
      return
    }
    const last = this.rows[this.rows.length - 1]?.time || 0
    const forward = rows.filter((r) => r.time >= last)
    if (!forward.length) return
    const step = this.step()
    let cursor = last
    const gap = forward.some((r) => {
      if (cursor && r.time > cursor + step) return true
      cursor = r.time
      return false
    })
    if (gap) {
      // Never append across a missing interval: refetch history instead.
      this.onStatus('Backfilling missing candles…', true)
      try {
        const fresh = await this.history(
          Math.floor(Date.now() / 1000) + 1,
          Math.max(initialBars, this.rows.length)
        )
        if (this.source !== source || scene !== this.scene) return
        this.setRows(fresh.candles)
        this.render()
      } catch {
        this.onStatus('Candle gap detected · reload the chart', true)
        return
      }
    } else {
      for (const row of forward) {
        this.candles?.update(toBar(row))
        this.volume?.update(toVolume(row))
        if (this.rows[this.rows.length - 1]?.time === row.time)
          this.rows[this.rows.length - 1] = row
        else {
          this.rows.push(row)
          this.times.push(row.time)
        }
      }
    }
    this.lastReceived = Date.now()
    this.onAvailability(true)
    this.onStatus(
      data.stale
        ? 'Market data is stale'
        : data.mode === 'demo'
          ? 'Demo data · not live prices'
          : 'Market feed active · updates every 3 seconds',
      Boolean(data.stale)
    )
    const latest = this.rows[this.rows.length - 1]
    if (latest) this.onPrice(latest, data)
  }

  async setContext(
    venue: string,
    symbol: string,
    resolution = this.resolution
  ) {
    instrument(ticker(venue, symbol))
    await this.whenReady()
    const scene = ++this.scene
    this.venue = venue
    this.symbol = symbol
    this.resolution = resolutions[resolution] ? resolution : '60'
    this.plan = null
    this.comparisonPlans = []
    this.focused = false
    this.aiCandidates = []
    this.clearManual()
    this.markResolution()
    this.onContext(this.context())
    await this.loadScene(scene)
  }

  async setResolution(resolution: string) {
    if (!resolutions[resolution] || resolution === this.resolution) return
    await this.whenReady()
    const scene = ++this.scene
    this.resolution = resolution
    this.aiCandidates = []
    this.markResolution()
    this.onContext(this.context())
    await this.loadScene(scene)
  }

  async plot(plan: Plan | null, _phase = 'draft', comparisons: Plan[] = []) {
    this.comparisonPlans = comparisons
      .filter(
        (p) =>
          p.snapshot.kind === 'perpetual' &&
          p.snapshot.venue === this.venue &&
          p.snapshot.symbol === this.symbol
      )
      .slice(0, 4)
    if (plan?.id !== this.plan?.id) this.focused = false
    this.plan = plan
    if (!this.chart || this.disposed) return
    this.render()
  }

  private planVisible() {
    const plan = this.plan
    return (
      !!plan &&
      (!plan.snapshot.kind || plan.snapshot.kind === 'perpetual') &&
      plan.snapshot.venue === this.venue &&
      plan.snapshot.symbol === this.symbol
    )
  }

  private render() {
    this.renderPlan()
    this.renderAI()
    this.overlay.setShapes([
      ...this.planShapes,
      ...this.aiShapes,
      ...this.manualShapes,
    ])
  }

  private removeLines(lines: IPriceLine[]) {
    for (const line of lines)
      try {
        this.candles?.removePriceLine(line)
      } catch {
        /* series already removed */
      }
  }

  private renderPlan() {
    this.removeLines(this.planLines)
    this.planLines = []
    this.planShapes = []
    const candles = this.candles
    if (!candles || this.times.length < 2) return
    if (this.comparisonPlans.length >= 2) {
      this.planShapes = comparisonShapes(this.comparisonPlans, this.plan?.id)
      return
    }
    const plan = this.plan
    if (!plan || !this.planVisible()) return
    for (const [title, price, color] of planPriceLines(plan))
      this.planLines.push(
        candles.createPriceLine({
          price: Number(price),
          color,
          lineWidth: 2,
          lineStyle: LineStyle.Solid,
          axisLabelVisible: true,
          title,
        })
      )
    // Risk/reward zones span the last dozen candles into the right margin.
    const start = this.times[Math.max(0, this.times.length - 12)]
    const end = this.times[this.times.length - 1] + this.step() * 6
    this.planShapes = planZones(plan, start, end)
  }

  private planLevels() {
    return levelPrices([
      ...(this.planVisible() && this.plan ? [this.plan] : []),
      ...this.comparisonPlans,
    ])
  }

  // While a plan is focused, the price scale stretches to keep every plan
  // level on screen together with the visible candles.
  private autoscale(base: AutoscaleInfo | null): AutoscaleInfo | null {
    if (!this.focused || !base?.priceRange) return base
    const levels = this.planLevels()
    if (!levels.length) return base
    const lo = Math.min(base.priceRange.minValue, ...levels)
    const hi = Math.max(base.priceRange.maxValue, ...levels)
    const pad = (hi - lo) * 0.05
    return {
      ...base,
      priceRange: { minValue: lo - pad, maxValue: hi + pad },
    }
  }

  async focusPlan() {
    await this.whenReady()
    if (!this.plan || !this.times.length) return
    this.focused = true
    const n = this.times.length
    this.chart?.timeScale().setVisibleLogicalRange({
      from: Math.max(0, n - 1 - 64),
      to: n - 1 + 9,
    })
    this.candles?.priceScale().applyOptions({ autoScale: true })
  }

  private renderAI() {
    this.removeLines(this.aiLines)
    this.aiLines = []
    this.aiShapes = []
    const candles = this.candles
    if (!candles || this.comparisonPlans.length >= 2) return
    const levels = this.planVisible() ? this.planLevels() : []
    for (const d of selectAnnotations(this.aiCandidates, levels)) {
      const color = {
        support: colors.up,
        resistance: colors.down,
        other: colors.info,
      }[annotationKind(d)]
      this.paintDrawing(d, color, true, this.aiLines, this.aiShapes)
    }
  }

  private paintDrawing(
    d: Drawing,
    color: string,
    dashed: boolean,
    lines: IPriceLine[],
    shapes: Shape[]
  ) {
    const points = d.points.map((p) => ({
      time: Number(p.time),
      price: Number(p.price),
    }))
    if (points.some((p) => !Number.isFinite(p.price))) return
    if (d.kind === 'horizontal_line' && points.length >= 1 && this.candles) {
      lines.push(
        this.candles.createPriceLine({
          price: points[0].price,
          color,
          lineWidth: 1,
          lineStyle: dashed ? LineStyle.Dashed : LineStyle.Solid,
          axisLabelVisible: true,
          title: d.label.slice(0, 32),
        })
      )
      return
    }
    shapes.push(...drawingShapes(d, points, color, dashed))
  }

  private clearManual() {
    this.removeLines(this.manualLines)
    this.manualLines = []
    this.manualShapes = []
  }

  async draw(drawings: Drawing[], source = 'manual', interval?: string) {
    await this.whenReady()
    if (source === 'ai') {
      // Drawings are only meaningful on the timeframe they were made for.
      if (interval && interval !== this.context().interval) return
      this.aiCandidates = drawings.slice(0, 10)
    } else {
      for (const d of drawings.slice(0, 20))
        this.paintDrawing(
          d,
          colors.info,
          false,
          this.manualLines,
          this.manualShapes
        )
    }
    this.render()
  }

  // Repaints the chart chrome after the page switches theme.
  private readonly repaintTheme = () => {
    this.chart?.applyOptions(themeOptions())
  }

  dispose() {
    window.removeEventListener(THEME_EVENT, this.repaintTheme)
    this.disposed = true
    this.scene++
    clearInterval(this.ageTimer)
    this.closeFeed()
    this.chart?.remove()
    this.chart = null
    this.candles = null
    this.volume = null
    this.root?.remove()
    this.root = null
  }
}
