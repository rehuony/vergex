import type { Drawing, HoldingLine, Plan } from '@/api/futures'
import { colors } from '@/components/market-chart/market-chart-surface'
import { activeLanguage, t } from '@/i18n'
import {
  nativeMarket,
  type AccountPosition,
} from '@/utils/futures/futures-account'
import type { Shape } from './futures-chart-surface'

// What the Futures chart draws for plans and agent drawings, as price lines and
// overlay shapes.

const fmt = (value: number | string) =>
  Number(value).toLocaleString('en-US', { maximumFractionDigits: 8 })

const lineTitle = (key: string, params?: Record<string, string | number>) =>
  t(`futures.chartLine.${key}`, activeLanguage(), params)

// holdingLines lists what the account really holds on this market: where each position was
// entered, and where the exchange's stop and take-profit orders sit.
export function holdingLines(
  venue: string,
  symbol: string,
  positions: AccountPosition[],
  orders: {
    type: string
    price: number
    stop_price: number
    quantity: number
  }[]
): HoldingLine[] {
  const market = nativeMarket(symbol, venue)
  return [
    ...positions
      .filter((p) => nativeMarket(p.symbol, venue) === market)
      .map(
        (p): HoldingLine => ({
          tone: p.side === 'long' ? 'up' : 'down',
          price: p.entry,
          title: lineTitle(p.side === 'long' ? 'long' : 'short', {
            leverage: p.leverage,
            quantity: fmt(p.quantity),
          }),
        })
      ),
    ...orders.flatMap((o): HoldingLine[] => {
      const target = o.type.includes('TAKE_PROFIT')
      if (!target && !o.type.includes('STOP')) return []
      return [
        {
          tone: target ? 'up' : 'down',
          price: Number(o.stop_price || o.price),
          title: lineTitle(target ? 'targetOrder' : 'stopOrder', {
            quantity: fmt(o.quantity),
          }),
        },
      ]
    }),
  ].filter((line) => Number.isFinite(line.price) && line.price > 0)
}

export function annotationKind(d: Drawing) {
  if (/support|\bS[1-9]\b/i.test(d.label)) return 'support'
  if (/resistance|\bR[1-9]\b/i.test(d.label)) return 'resistance'
  return 'other'
}

// selectAnnotations drops agent horizontal lines that duplicate plan levels or
// each other, then keeps at most four, balanced between support and
// resistance regardless of model ordering.
export function selectAnnotations(drawings: Drawing[], levels: number[]) {
  const near = (a: number, b: number) =>
    Math.abs(a - b) <= Math.max(a, b) * 0.00015
  const picked: Drawing[] = []
  for (const d of drawings) {
    const price = Number(d.points[0]?.price)
    if (
      d.kind === 'horizontal_line' &&
      (levels.some((p) => near(p, price)) ||
        picked.some(
          (o) =>
            o.kind === 'horizontal_line' &&
            near(Number(o.points[0]?.price), price)
        ))
    )
      continue
    picked.push(d)
  }
  if (picked.length <= 4) return picked
  const support = picked.filter((d) => annotationKind(d) === 'support')
  const resistance = picked.filter((d) => annotationKind(d) === 'resistance')
  const balanced = support.slice(0, 2).concat(resistance.slice(0, 2))
  for (const d of picked)
    if (balanced.length < 4 && !balanced.includes(d)) balanced.push(d)
  return balanced
}

// planPriceLines lists a plan's entry, stop loss and take profit lines as
// [title, price, color].
export function planPriceLines(plan: Plan): [string, string, string][] {
  const kind = plan.idea.execution?.kind
  const entries: [string, string][] =
    kind === 'ladder'
      ? plan.orders.map((o) => [
          lineTitle('limitOrder', {
            index: o.index,
            quantity: fmt(o.quantity),
          }),
          o.price,
        ])
      : [
          [
            kind === 'twap'
              ? lineTitle('twap', {
                  count: plan.orders.length,
                  last: Math.max(...plan.orders.map((o) => o.offset_seconds)),
                })
              : lineTitle(kind === 'market' ? 'estEntry' : 'entry'),
            plan.idea.entry,
          ],
        ]
  return [
    ...entries.map(([label, price]): [string, string, string] => [
      label,
      price,
      colors.ink,
    ]),
    [
      lineTitle('stopLoss', { amount: Number(plan.risk).toFixed(2) }),
      plan.idea.stop,
      colors.down,
    ],
    [
      lineTitle('takeProfit', { amount: Number(plan.reward).toFixed(2) }),
      plan.idea.target,
      colors.up,
    ],
  ]
}

// planZones shades the risk and reward zones from entry to stop and target
// between two times.
export function planZones(plan: Plan, start: number, end: number): Shape[] {
  const shapes: Shape[] = []
  for (const [price, color] of [
    [plan.idea.stop, colors.down],
    [plan.idea.target, colors.up],
  ])
    shapes.push({
      type: 'rect',
      x1: { time: start },
      x2: { time: end },
      p1: Number(plan.idea.entry),
      p2: Number(price),
      color,
      alpha: 0.14,
      border: 0,
    })
  return shapes
}

// comparisonShapes lays each plan out in its own lane across the right part
// of the pane, so overlapping price levels stay readable.
export function comparisonShapes(plans: Plan[], selected: string | undefined) {
  const shapes: Shape[] = []
  const left = 0.26,
    right = 0.9,
    lane = (right - left) / plans.length
  plans.forEach((p, i) => {
    const orders = p.orders?.length
      ? p.orders
      : [{ price: p.idea.entry, quantity: p.quantity }]
    const total = orders.reduce((n, o) => n + Number(o.quantity), 0)
    const entry = total
      ? orders.reduce((n, o) => n + Number(o.price) * Number(o.quantity), 0) /
        total
      : Number(p.idea.entry)
    const active = p.id === selected
    const color = colors.ink
    const tag = 'ABCD'[i]
    const x1 = { frac: left + i * lane + 0.005 },
      x2 = { frac: left + (i + 1) * lane - 0.005 }
    for (const [price, c] of [
      [p.idea.stop, colors.down],
      [p.idea.target, colors.up],
    ] as const)
      shapes.push({
        type: 'rect',
        x1,
        x2,
        p1: entry,
        p2: Number(price),
        color: c,
        alpha: active ? 0.16 : 0.07,
        border: active ? 1 : 0,
      })
    const lines: [number, string][] = active
      ? orders.map((o) => [Number(o.price), color])
      : [[entry, color]]
    lines.push([Number(p.idea.stop), colors.down])
    lines.push([Number(p.idea.target), colors.up])
    for (const [price, c] of lines)
      shapes.push({
        type: 'segment',
        x1,
        p1: price,
        x2,
        p2: price,
        color: c,
        width: active ? 2 : 1,
        dashed: !active,
      })
    const entryLabel =
      orders.length > 1 && p.idea.execution?.kind === 'ladder'
        ? 'Average entry ≈ '
        : p.idea.execution?.kind === 'market'
          ? 'Est. market entry '
          : 'Entry '
    const labels: [number, string, string, number][] = [
      [
        Number(p.idea.target),
        tag +
          ' · ' +
          (p.idea.side === 'long' ? 'Long' : 'Short') +
          ' Target ' +
          fmt(p.idea.target),
        colors.up,
        13,
      ],
      [entry, entryLabel + fmt(entry), color, 13],
      [Number(p.idea.stop), 'Stop loss ' + fmt(p.idea.stop), colors.down, 12],
    ]
    for (const [price, text, c, size] of labels)
      shapes.push({
        type: 'text',
        x: x1,
        p: price,
        text,
        color: c,
        size,
        bold: active,
      })
  })
  return shapes
}

// levelPrices collects every positive price level of the given plans.
export function levelPrices(plans: Plan[]) {
  return plans
    .flatMap((p) => [
      p.idea.entry,
      p.idea.stop,
      p.idea.target,
      ...(p.orders || []).map((o) => o.price),
    ])
    .map(Number)
    .filter((n) => Number.isFinite(n) && n > 0)
}

// drawingShapes turns a two-point drawing into a trend line or rectangle
// with its label; other kinds draw nothing.
export function drawingShapes(
  d: Drawing,
  points: { time: number; price: number }[],
  color: string,
  dashed: boolean
): Shape[] {
  if (points.length < 2 || points.some((p) => !Number.isFinite(p.time)))
    return []
  const [a, b] = points
  const shapes: Shape[] = []
  if (d.kind === 'trend_line') {
    shapes.push({
      type: 'segment',
      x1: { time: a.time },
      p1: a.price,
      x2: { time: b.time },
      p2: b.price,
      color,
      width: 1,
      dashed,
    })
  } else if (d.kind === 'rectangle') {
    shapes.push({
      type: 'rect',
      x1: { time: a.time },
      x2: { time: b.time },
      p1: a.price,
      p2: b.price,
      color,
      alpha: 0.08,
      border: 1,
    })
  } else return shapes
  if (d.label)
    shapes.push({
      type: 'text',
      x: { time: Math.min(a.time, b.time) },
      p: Math.max(a.price, b.price),
      text: d.label.slice(0, 48),
      color,
      size: 11,
      bold: false,
    })
  return shapes
}
