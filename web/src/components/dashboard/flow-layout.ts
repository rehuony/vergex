import type {
  ActivityBlock,
  ActivityEvent,
  ActivityKind,
} from '@/api/dashboard'

export type SceneBlock = Exclude<ActivityBlock, 'other'>

export type LineKey =
  | 'exchange>market'
  | 'market>traders'
  | 'market>forecast'
  | 'telegram>models'
  | 'traders>models'
  | 'futures>models'
  | 'traders>accounts'
  | 'futures>accounts'
  | 'accounts>positions'
  | 'models>agent_cost'
  | 'accounts>fees'

export const LINE_KIND: Record<LineKey, ActivityKind> = {
  'exchange>market': 'market',
  'market>traders': 'market',
  'market>forecast': 'market',
  'telegram>models': 'agent',
  'traders>models': 'agent',
  'futures>models': 'agent',
  'traders>accounts': 'orders',
  'futures>accounts': 'orders',
  'accounts>positions': 'funds',
  'models>agent_cost': 'funds',
  'accounts>fees': 'funds',
}

// The stage is laid out so the flow reads at a glance. Everything runs left to right, one
// column at a time: where data comes from, the engines, where they send things, what comes
// of it. Calls to a model run on an upper deck and orders and money on the ground, so the
// two lines no arrangement can keep apart pass over each other instead of crossing.
export const DECK_Y = [0.06, 1.5] as const

export interface BlockPlace {
  x: number
  z: number
  /** Its gate reaches the upper deck. */
  tall: boolean
  /** Holds a whole market's worth of coins. */
  large?: boolean
}

export const BLOCKS: Record<SceneBlock, BlockPlace> = {
  exchange: { x: -19, z: 0, tall: false },
  market: { x: -12, z: 0, tall: false, large: true },
  telegram: { x: -3, z: -9, tall: true },
  traders: { x: -3, z: -4.4, tall: true },
  futures: { x: -3, z: 0.4, tall: true },
  forecast: { x: -3, z: 6.4, tall: false, large: true },
  models: { x: 6.5, z: -5.2, tall: true },
  accounts: { x: 6.5, z: 2, tall: false },
  agent_cost: { x: 15, z: -5.2, tall: true },
  positions: { x: 15, z: 0.4, tall: false },
  fees: { x: 15, z: 5.8, tall: false },
}

export interface LinePlace {
  from: SceneBlock
  to: SceneBlock
  /** 0 on the ground, 1 on the upper deck. */
  deck: 0 | 1
  /** Where it leaves its block and reaches the next, across the block's depth. */
  leave: number
  reach: number
}

const line = (
  from: SceneBlock,
  to: SceneBlock,
  deck: 0 | 1,
  leave = 0,
  reach = 0
): LinePlace => ({ from, to, deck, leave, reach })

export const LINES: Record<LineKey, LinePlace> = {
  'exchange>market': line('exchange', 'market', 0),
  'market>traders': line('market', 'traders', 0, -0.8),
  'market>forecast': line('market', 'forecast', 0, 0.8),
  'telegram>models': line('telegram', 'models', 1, 0, -0.7),
  'traders>models': line('traders', 'models', 1),
  'futures>models': line('futures', 'models', 1, 0, 0.7),
  'traders>accounts': line('traders', 'accounts', 0, 0, -0.6),
  'futures>accounts': line('futures', 'accounts', 0, 0, 0.6),
  'models>agent_cost': line('models', 'agent_cost', 1),
  'accounts>positions': line('accounts', 'positions', 0, -0.6),
  'accounts>fees': line('accounts', 'fees', 0, 0.6),
}

/** The stages of the flow, named along the front of the floor. */
export const STAGES = [
  { key: 'data', x: -13 },
  { key: 'engines', x: -3 },
  { key: 'out', x: 6.5 },
  { key: 'result', x: 13.5 },
] as const

const LINE_KEYS = new Set<string>(Object.keys(LINES))

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
