import type { ActivityBlock, ActivityEvent, ActivityKind } from '@/api/activity'

// Everything in the scene sits on one grid, so routes and members read as one lattice.
const SMALL_PITCH = 8
const LARGE_PITCH = 12
/** From here the scene has the room for the larger grid. */
const LARGE_WIDTH = 860
const LARGE_HEIGHT = 520
/** The smallest scene the layout keeps clear of its own labels. */
export const MIN_WIDTH = 640
export const MIN_HEIGHT = 360

export type SceneBlock = Exclude<ActivityBlock, 'agent_cost' | 'fees' | 'other'>

export type LineKey =
  | 'market>traders'
  | 'market>forecast'
  | 'telegram>models'
  | 'traders>models'
  | 'frontier>models'
  | 'traders>accounts'
  | 'frontier>accounts'
  | 'accounts>positions'
  | 'models>agent_cost'
  | 'accounts>fees'

export const LINE_KIND: Record<LineKey, ActivityKind> = {
  'market>traders': 'market',
  'market>forecast': 'market',
  'telegram>models': 'agent',
  'traders>models': 'agent',
  'frontier>models': 'agent',
  'traders>accounts': 'orders',
  'frontier>accounts': 'orders',
  'accounts>positions': 'funds',
  'models>agent_cost': 'funds',
  'accounts>fees': 'funds',
}

export interface BlockPlace {
  x: number
  y: number
  cols: number
  rows: number
}

export interface FlowLine {
  key: LineKey
  points: [number, number][]
  dots: { x: number; y: number }[]
}

export interface FlowLayout {
  width: number
  height: number
  pitch: number
  blocks: Record<SceneBlock, BlockPlace>
  lines: FlowLine[]
  /** Where the cost and fee lines end, and their figures go. */
  costLabel: { x: number; y: number }
  feesLabel: { x: number; y: number }
  /** Below the positions, for today's result. */
  todayLabel: { x: number; y: number }
}

function dotsAlong(points: [number, number][], pitch: number) {
  const dots: FlowLine['dots'] = []
  points.slice(1).forEach(([x2, y2], index) => {
    const [x1, y1] = points[index]
    const steps = (Math.abs(x2 - x1) + Math.abs(y2 - y1)) / pitch
    for (let step = index === 0 ? 0 : 1; step <= steps; step++) {
      dots.push({
        x: x1 + ((x2 - x1) * step) / steps,
        y: y1 + ((y2 - y1) * step) / steps,
      })
    }
  })
  return dots
}

// Lays the scene out left to right: what feeds the engines, the engines, then where
// they send things. Lines may cross only where one trader order line must pass a
// Frontier model line, which no arrangement of these blocks avoids.
export function layoutFlow(rawWidth: number, rawHeight: number): FlowLayout {
  const width = Math.max(rawWidth, MIN_WIDTH)
  const height = Math.max(rawHeight, MIN_HEIGHT)
  const p =
    width >= LARGE_WIDTH && height >= LARGE_HEIGHT ? LARGE_PITCH : SMALL_PITCH
  const snap = (value: number) => Math.round(value / p) * p

  const blockWidth = 6 * p
  const engineX = snap(width * 0.24)
  const rightX = snap(width * 0.62)
  const positionsX = snap(Math.min(width * 0.83, width - 112))

  const top = snap(48)
  const step = Math.floor((height - 16 - 2 * p - top) / 3 / p) * p
  const telegramY = top
  const tradersY = top + step
  const frontierY = top + step * 2
  const forecastY = top + step * 3
  const modelsY = snap(top + step / 2)
  const accountsY = snap(top + step * 2.5)
  const marketY = frontierY - 2 * p
  const marketX = 3 * p

  const blocks: FlowLayout['blocks'] = {
    market: { x: marketX, y: marketY, cols: 6, rows: 6 },
    telegram: { x: engineX, y: telegramY, cols: 6, rows: 3 },
    traders: { x: engineX, y: tradersY, cols: 6, rows: 3 },
    frontier: { x: engineX, y: frontierY, cols: 6, rows: 3 },
    forecast: { x: engineX, y: forecastY, cols: 6, rows: 3 },
    models: { x: rightX, y: modelsY, cols: 6, rows: 3 },
    accounts: { x: rightX, y: accountsY, cols: 6, rows: 3 },
    positions: { x: positionsX, y: accountsY, cols: 6, rows: 4 },
  }

  const marketOut = marketX + blockWidth
  const engineIn = engineX - p
  const engineOut = engineX + blockWidth
  const rightIn = rightX - p
  const rightOut = rightX + blockWidth
  const marketBus = snap((marketOut + engineIn) / 2)
  const bus = (share: number) => snap(engineOut + (rightIn - engineOut) * share)

  const route = (key: LineKey, points: [number, number][]): FlowLine => ({
    key,
    points,
    dots: dotsAlong(points, p),
  })

  const lines: FlowLine[] = [
    route('market>traders', [
      [marketOut, marketY + p],
      [marketBus, marketY + p],
      [marketBus, tradersY + p],
      [engineIn, tradersY + p],
    ]),
    route('market>forecast', [
      [marketOut, marketY + 4 * p],
      [marketBus, marketY + 4 * p],
      [marketBus, forecastY + p],
      [engineIn, forecastY + p],
    ]),
    route('frontier>accounts', [
      [engineOut, frontierY + 2 * p],
      [bus(0.2), frontierY + 2 * p],
      [bus(0.2), accountsY + p],
      [rightIn, accountsY + p],
    ]),
    route('traders>accounts', [
      [engineOut, tradersY + 2 * p],
      [bus(0.4), tradersY + 2 * p],
      [bus(0.4), accountsY],
      [rightIn, accountsY],
    ]),
    route('traders>models', [
      [engineOut, tradersY],
      [bus(0.6), tradersY],
      [bus(0.6), modelsY + p],
      [rightIn, modelsY + p],
    ]),
    route('frontier>models', [
      [engineOut, frontierY],
      [bus(0.75), frontierY],
      [bus(0.75), modelsY + 2 * p],
      [rightIn, modelsY + 2 * p],
    ]),
    route('telegram>models', [
      [engineOut, telegramY + p],
      [bus(0.9), telegramY + p],
      [bus(0.9), modelsY],
      [rightIn, modelsY],
    ]),
    route('accounts>positions', [
      [rightOut, accountsY + p],
      [positionsX - p, accountsY + p],
    ]),
    route('models>agent_cost', [
      [rightOut, modelsY + p],
      [positionsX - p, modelsY + p],
    ]),
    route('accounts>fees', [
      [rightX + 2 * p, accountsY + 3 * p],
      [rightX + 2 * p, accountsY + 5 * p],
    ]),
  ]

  return {
    width,
    height,
    pitch: p,
    blocks,
    lines,
    costLabel: { x: positionsX, y: modelsY + p },
    feesLabel: { x: rightX, y: accountsY + 5 * p + 20 },
    todayLabel: { x: positionsX, y: accountsY + 3 * p + 22 },
  }
}

const LINE_KEYS = new Set<string>(Object.keys(LINE_KIND))

// The line an event runs along, and whether it runs against the line's direction.
// An event outside the scene, such as a model test, has none.
export function lineOf(
  event: Pick<ActivityEvent, 'from' | 'to'>
): { key: LineKey; reverse: boolean } | null {
  const forward = `${event.from.block}>${event.to.block}`
  if (LINE_KEYS.has(forward)) {
    return { key: forward as LineKey, reverse: false }
  }
  const backward = `${event.to.block}>${event.from.block}`
  if (LINE_KEYS.has(backward)) {
    return { key: backward as LineKey, reverse: true }
  }
  return null
}
