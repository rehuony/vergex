import {
  type IPrimitivePaneRenderer,
  type IPrimitivePaneView,
  type ISeriesPrimitive,
  type Logical,
  type PrimitivePaneViewZOrder,
  type SeriesAttachedParameter,
  type Time,
} from 'lightweight-charts'
import { buttonVariants } from '@/components/ui/button'
import { themeColors } from '@/components/market-chart/market-chart-surface'
import { cn } from '@/utils/cn'
import { resolutions } from './planning-chart-data'

// What the Frontier chart adds to the shared surface: its timeframe toolbar and
// the overlay that paints zones, segments and labels over the candles.

export const compareColors = ['#9b6a14', '#3575a8', '#8050a1', '#237a69']

// chartFrame builds the chart's DOM: a timeframe toolbar above the canvas
// host.
export function chartFrame(onResolution: (resolution: string) => void) {
  const root = document.createElement('div')
  root.className = 'planning-chart-root'
  const toolbar = document.createElement('div')
  toolbar.className = 'planning-chart-toolbar'
  toolbar.setAttribute('role', 'group')
  toolbar.setAttribute('aria-label', 'Chart timeframe')
  for (const [resolution, label] of Object.entries(resolutions)) {
    const button = document.createElement('button')
    button.type = 'button'
    button.className = cn(
      buttonVariants({ variant: 'segment', size: 'sm' }),
      'h-6 rounded-md px-2 text-[11px] tabular-nums'
    )
    button.textContent = label
    button.dataset.resolution = resolution
    button.addEventListener('click', () => onResolution(resolution))
    toolbar.append(button)
  }
  const host = document.createElement('div')
  host.className = 'planning-chart-canvas'
  root.append(toolbar, host)
  return { root, toolbar, host }
}

// Overlay shapes. x is either a unix time or a fraction of the pane width
// (comparison lanes stay in place while the user scrolls).
export type X = { time: number } | { frac: number }
export type Shape =
  | {
      type: 'rect'
      x1: X
      x2: X
      p1: number
      p2: number
      color: string
      alpha: number
      border: number
    }
  | {
      type: 'segment'
      x1: X
      p1: number
      x2: X
      p2: number
      color: string
      width: number
      dashed: boolean
    }
  | {
      type: 'text'
      x: X
      p: number
      text: string
      color: string
      size: number
      bold: boolean
    }

type RenderTarget = Parameters<IPrimitivePaneRenderer['draw']>[0]

export class OverlayPrimitive implements ISeriesPrimitive<Time> {
  private shapes: Shape[] = []
  private params: SeriesAttachedParameter<Time> | null = null
  private readonly views: IPrimitivePaneView[]
  constructor(private readonly toLogical: (time: number) => number | null) {
    const view = (order: PrimitivePaneViewZOrder): IPrimitivePaneView => ({
      zOrder: () => order,
      renderer: () => ({
        draw: (target: RenderTarget) => this.paint(target, order === 'bottom'),
      }),
    })
    this.views = [view('bottom'), view('top')]
  }
  attached(params: SeriesAttachedParameter<Time>) {
    this.params = params
  }
  detached() {
    this.params = null
  }
  paneViews() {
    return this.views
  }
  setShapes(shapes: Shape[]) {
    this.shapes = shapes
    this.params?.requestUpdate()
  }
  private paint(target: RenderTarget, background: boolean) {
    const params = this.params
    if (!params || !this.shapes.length) return
    target.useMediaCoordinateSpace(({ context: ctx, mediaSize }) => {
      const x = (v: X) => {
        if ('frac' in v) return v.frac * mediaSize.width
        const logical = this.toLogical(v.time)
        return logical === null
          ? null
          : params.chart.timeScale().logicalToCoordinate(logical as Logical)
      }
      const y = (price: number) => params.series.priceToCoordinate(price)
      for (const s of this.shapes) {
        if ((s.type === 'rect') !== background) continue
        if (s.type === 'rect') {
          const x1 = x(s.x1),
            x2 = x(s.x2),
            y1 = y(s.p1),
            y2 = y(s.p2)
          if (x1 === null || x2 === null || y1 === null || y2 === null) continue
          ctx.globalAlpha = s.alpha
          ctx.fillStyle = s.color
          ctx.fillRect(
            Math.min(x1, x2),
            Math.min(y1, y2),
            Math.abs(x2 - x1),
            Math.abs(y2 - y1)
          )
          ctx.globalAlpha = 1
          if (s.border) {
            ctx.strokeStyle = s.color
            ctx.lineWidth = s.border
            ctx.setLineDash([])
            ctx.strokeRect(
              Math.min(x1, x2),
              Math.min(y1, y2),
              Math.abs(x2 - x1),
              Math.abs(y2 - y1)
            )
          }
        } else if (s.type === 'segment') {
          const x1 = x(s.x1),
            x2 = x(s.x2),
            y1 = y(s.p1),
            y2 = y(s.p2)
          if (x1 === null || x2 === null || y1 === null || y2 === null) continue
          ctx.strokeStyle = s.color
          ctx.lineWidth = s.width
          ctx.setLineDash(s.dashed ? [5, 4] : [])
          ctx.beginPath()
          ctx.moveTo(x1, y1)
          ctx.lineTo(x2, y2)
          ctx.stroke()
          ctx.setLineDash([])
        } else {
          const tx = x(s.x),
            ty = y(s.p)
          if (tx === null || ty === null) continue
          ctx.font = `${s.bold ? 600 : 400} ${s.size}px ui-sans-serif, system-ui, sans-serif`
          ctx.textBaseline = 'bottom'
          const width = ctx.measureText(s.text).width
          ctx.fillStyle = themeColors().label
          ctx.fillRect(tx - 2, ty - s.size - 5, width + 4, s.size + 4)
          ctx.fillStyle = s.color
          ctx.fillText(s.text, tx, ty - 2)
        }
      }
    })
  }
}
