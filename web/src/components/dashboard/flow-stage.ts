import {
  AdditiveBlending,
  Box2,
  BoxGeometry,
  BufferGeometry,
  CanvasTexture,
  Color,
  ConeGeometry,
  CubicBezierCurve3,
  CylinderGeometry,
  DoubleSide,
  EdgesGeometry,
  Float32BufferAttribute,
  Fog,
  Group,
  InstancedMesh,
  Line,
  LineBasicMaterial,
  LineSegments,
  Mesh,
  MeshBasicMaterial,
  NormalBlending,
  Object3D,
  PerspectiveCamera,
  Raycaster,
  RingGeometry,
  Scene,
  SphereGeometry,
  Sprite,
  SpriteMaterial,
  Vector2,
  Vector3,
  WebGLRenderer,
  type Material,
  type Object3DEventMap,
} from 'three'
import { THEME_EVENT } from '@/contexts/theme-context'
import {
  BLOCKS,
  DECK_Y,
  LINES,
  STAGES,
  type LineKey,
  type SceneBlock,
} from './flow-layout'
import type { BlockView, Member } from './flow-members'

// The Dashboard's stage, drawn with WebGL: a thin gate for each part of the system with its
// members on its face, and a curtain for each route, as tall as the route has been busy. It
// draws only when something changes, so with nothing happening it costs nothing.

// How long an event takes along a line, however long the line is.
const TRAVEL_S = 0.56
const TRAIL = 14
const TRAIL_STEP = 0.024
/** Past this many at once, an event is counted but not drawn. */
const MAX_RUNS = 12
const RING_S = 0.7
const CALM_S = 0.5
const MEMBER_LIT_S = 0.7
const MAX_DOTS = 324

const TALL_H = 2.5
const SHORT_H = 1.4
const LARGE_H = 2.9
const GATE_THICKNESS = 0.07
const FOV = 20
const AXIS = { start: -15, end: 14, z: 8 }
/** The room a gate's name needs in the frame: half its width, and its height above the gate. */
const LABEL_HALF_PX = 72
const LABEL_TALL_PX = 40
/** How much of the frame the picture may fill, from its centre. */
const FRAME = 0.95
// The picture's middle, so the view turns about what it shows.
const TARGET = new Vector3(-2.2, 0.9, 1)
// Turned so the gates show their faces, which head-on would be edge-on, and to the side
// the flow starts from, so the exchange stands nearest and the rest runs away from it.
const YAW = -0.62
// How far a drag can take the view: past these something leaves the frame, and turning
// towards head-on the gates go edge-on.
const YAW_RANGE = [-0.3, 0.2] as const
const TILT_RANGE = [-0.22, 0.15] as const
/** Looking further down takes height the frame may not have, so the camera backs away. */
const TILT_BACK = 2.2
const ZOOM_MIN = 0.45
const ZOOM_MAX = 1.6
const DRAG_START_PX = 4

const css = (name: string) =>
  getComputedStyle(document.documentElement).getPropertyValue(name).trim()

interface Palette {
  dark: boolean
  bg: Color
  gate: Color
  edge: Color
  floor: Color
  ink: Color
  muted: Color
  faint: Color
  up: Color
  down: Color
}

function readPalette(): Palette {
  const color = (name: string) => new Color(css(name))
  const muted = color('--surface-muted')
  const ink = color('--ink')
  const dark = document.documentElement.dataset.theme === 'dark'
  return {
    dark,
    bg: color('--canvas'),
    gate: dark ? muted.clone().lerp(ink, 0.08) : muted.clone().lerp(ink, 0.1),
    edge: color('--line-strong'),
    floor: color('--line'),
    ink,
    muted: color('--muted'),
    faint: color('--faint'),
    up: color('--success'),
    down: color('--danger'),
  }
}

// A token named the way the event list names it, such as var(--ink).
const tokenColor = (value: string) => {
  const name = /var\((--[a-z-]+)\)/.exec(value)?.[1]
  return new Color(name ? css(name) : value)
}

const blockSize = (id: SceneBlock) => {
  const place = BLOCKS[id]
  return {
    w: GATE_THICKNESS,
    d: place.large ? 3 : 2.4,
    h: place.large ? LARGE_H : place.tall ? TALL_H : SHORT_H,
  }
}

/** A route with no events stays a hairline; the busiest stops short of the block's top. */
function curtainHeight(volume: number, deck: 0 | 1) {
  if (volume <= 0) return 0.08
  return Math.min(0.2 + 0.26 * Math.log10(1 + volume), deck ? 0.85 : 1.2)
}

function lineCurve(key: LineKey, height: number) {
  const place = LINES[key]
  const from = BLOCKS[place.from]
  const to = BLOCKS[place.to]
  const y = DECK_Y[place.deck] + height / 2
  const start = new Vector3(
    from.x + blockSize(place.from).w / 2,
    y,
    from.z + place.leave
  )
  const end = new Vector3(
    to.x - blockSize(place.to).w / 2,
    y,
    to.z + place.reach
  )
  const pull = (end.x - start.x) * 0.5
  return new CubicBezierCurve3(
    start,
    start.clone().add(new Vector3(pull, 0, 0)),
    end.clone().sub(new Vector3(pull, 0, 0)),
    end
  )
}

interface Run {
  key: LineKey
  curve: CubicBezierCurve3
  reverse: boolean
  color: Color
  t: number
  glow: Sprite
  done?: () => void
  arrived: boolean
}

interface Ring {
  mesh: Mesh
  t: number
}

interface BlockParts {
  group: Group
  /** Thicker than the gate, so one seen nearly edge-on can still be pointed at. */
  pick: Mesh
  /** Where each member's dot is, to light it. */
  fill: InstancedMesh | null
  slots: Map<string, number>
  rest: Color[]
  signature: string
}

interface LineParts {
  group: Group
  curve: CubicBezierCurve3
  curtain: MeshBasicMaterial
  edge: LineBasicMaterial
  marks: MeshBasicMaterial
  pick: Mesh
  signature: string
  base: number
}

export interface StageData {
  views: Record<SceneBlock, BlockView>
  volumes: Partial<Record<LineKey, number>>
}

export interface StageCallbacks {
  onHover: (key: string | null) => void
  onPick: (key: string | null) => void
  /** Called after each drawn frame, with where anything named on the stage now is. */
  onFrame: (
    project: (anchor: string) => { x: number; y: number } | null
  ) => void
  /** Whether the view has been turned or zoomed away from where it starts. */
  onViewMoved: (moved: boolean) => void
}

const disposeTree = (root: Object3D<Object3DEventMap>) =>
  root.traverse((node) => {
    const object = node as Object3D & {
      geometry?: BufferGeometry
      material?: Material | Material[]
    }
    object.geometry?.dispose()
    if (Array.isArray(object.material)) {
      object.material.forEach((material) => material.dispose())
    } else {
      object.material?.dispose()
    }
  })

function glowTexture() {
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 64
  const context = canvas.getContext('2d')
  if (context) {
    const fade = context.createRadialGradient(32, 32, 0, 32, 32, 32)
    fade.addColorStop(0, 'rgba(255,255,255,1)')
    fade.addColorStop(0.35, 'rgba(255,255,255,.35)')
    fade.addColorStop(1, 'rgba(255,255,255,0)')
    context.fillStyle = fade
    context.fillRect(0, 0, 64, 64)
  }
  return new CanvasTexture(canvas)
}

export function webglAvailable() {
  try {
    const canvas = document.createElement('canvas')
    return Boolean(canvas.getContext('webgl2') ?? canvas.getContext('webgl'))
  } catch {
    return false
  }
}

export class FlowStage {
  private readonly renderer: WebGLRenderer
  private readonly scene = new Scene()
  private readonly camera = new PerspectiveCamera(FOV, 1, 0.1, 800)
  private readonly raycaster = new Raycaster()
  private readonly calm: boolean
  private readonly glow = glowTexture()
  private readonly resizes: ResizeObserver
  private palette = readPalette()
  private blocks = new Map<SceneBlock, BlockParts>()
  private lines = new Map<LineKey, LineParts>()
  private data: StageData | null = null
  private active: (key: LineKey) => boolean = () => true
  private strong: string | null = null
  private runs: Run[] = []
  private rings: Ring[] = []
  private lit: { block: SceneBlock; slot: number; t: number }[] = []
  private calmLines: { key: LineKey; t: number }[] = []
  private comets: InstancedMesh | null = null
  private frame = 0
  private last = 0
  private disposed = false
  private distance = 80
  private readonly lens = new Vector2()
  private pitch = 0.62
  private yaw = 0
  private tilt = 0
  private zoom = 1
  private viewMoved = false
  private shownYaw = YAW
  private shownPitch = 0.62
  private pointer: { x: number; y: number } | null = null
  private press: {
    x: number
    y: number
    yaw: number
    tilt: number
    dragging: boolean
  } | null = null
  private hovered: string | null = null

  constructor(
    private readonly host: HTMLElement,
    private readonly callbacks: StageCallbacks
  ) {
    this.calm = matchMedia('(prefers-reduced-motion: reduce)').matches
    this.renderer = new WebGLRenderer({ antialias: true })
    // Sharper than this costs more than anyone can see.
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
    const canvas = this.renderer.domElement
    canvas.style.cssText = 'position:absolute;inset:0;display:block'
    host.prepend(canvas)
    canvas.addEventListener('pointerdown', this.onPointerDown)
    canvas.addEventListener('pointermove', this.onPointerMove)
    canvas.addEventListener('pointerup', this.onPointerUp)
    canvas.addEventListener('pointerleave', this.onPointerLeave)
    canvas.addEventListener('wheel', this.onWheel, { passive: false })
    canvas.addEventListener('dblclick', this.resetView)
    window.addEventListener(THEME_EVENT, this.onTheme)
    this.resizes = new ResizeObserver(() => this.resize())
    this.resizes.observe(host)
    this.buildScene()
    this.resize()
  }

  dispose() {
    this.disposed = true
    if (this.frame) cancelAnimationFrame(this.frame)
    this.resizes.disconnect()
    window.removeEventListener(THEME_EVENT, this.onTheme)
    const canvas = this.renderer.domElement
    canvas.removeEventListener('pointerdown', this.onPointerDown)
    canvas.removeEventListener('pointermove', this.onPointerMove)
    canvas.removeEventListener('pointerup', this.onPointerUp)
    canvas.removeEventListener('pointerleave', this.onPointerLeave)
    canvas.removeEventListener('wheel', this.onWheel)
    canvas.removeEventListener('dblclick', this.resetView)
    disposeTree(this.scene)
    this.glow.dispose()
    this.renderer.dispose()
    canvas.remove()
  }

  setData(data: StageData) {
    this.data = data
    this.syncBlocks()
    this.syncLines()
    this.invalidate()
  }

  /** Which lines read as in play, and the one being pointed at if any. */
  setFocus(active: (key: LineKey) => boolean, strong: string | null) {
    this.active = active
    this.strong = strong
    this.applyFocus()
    this.invalidate()
  }

  /** Returns false when too much is already running to draw this. */
  fire(
    key: LineKey,
    reverse: boolean,
    color: string,
    done?: () => void
  ): boolean {
    const parts = this.lines.get(key)
    if (!parts) return false
    const place = LINES[key]
    const tint = tokenColor(color)
    this.ripple(reverse ? place.to : place.from, tint)
    if (this.calm) {
      this.calmLines.push({ key, t: 0 })
      this.applyFocus()
      this.invalidate()
      done?.()
      return true
    }
    if (this.runs.length >= MAX_RUNS) return false
    const glow = new Sprite(
      new SpriteMaterial({
        map: this.glow,
        color: tint,
        transparent: true,
        opacity: this.palette.dark ? 0.95 : 0.35,
        blending: this.palette.dark ? AdditiveBlending : NormalBlending,
        depthWrite: false,
        fog: false,
      })
    )
    glow.scale.setScalar(this.palette.dark ? 1.5 : 1)
    glow.position.copy(parts.curve.getPoint(reverse ? 1 : 0))
    this.scene.add(glow)
    this.runs.push({
      key,
      curve: parts.curve,
      reverse,
      color: tint,
      t: 0,
      glow,
      done,
      arrived: false,
    })
    this.invalidate()
    return true
  }

  /** Lights a block's members for a moment, for an event that names them. */
  light(block: SceneBlock, ids: string[], color: string) {
    const parts = this.blocks.get(block)
    if (!parts?.fill?.instanceColor) return
    const tint = tokenColor(color)
    ids.forEach((id) => {
      const slot = parts.slots.get(id)
      if (slot === undefined) return
      parts.fill?.setColorAt(slot, tint)
      this.lit.push({ block, slot, t: 0 })
    })
    parts.fill.instanceColor.needsUpdate = true
    this.invalidate()
  }

  /** Goes back to where the view starts. */
  readonly resetView = () => {
    this.yaw = 0
    this.tilt = 0
    this.zoom = 1
    this.noteMoved(false)
    this.invalidate()
  }

  private noteMoved(moved: boolean) {
    if (moved === this.viewMoved) return
    this.viewMoved = moved
    this.callbacks.onViewMoved(moved)
  }

  private readonly onTheme = () => {
    this.buildScene()
    this.resize()
  }

  private buildScene() {
    if (this.disposed) return
    this.palette = readPalette()
    this.renderer.setClearColor(this.palette.bg, 1)
    disposeTree(this.scene)
    this.scene.clear()
    this.blocks.clear()
    this.lines.clear()
    this.runs = []
    this.rings = []
    this.lit = []
    this.calmLines = []
    this.buildFloor()
    this.comets = new InstancedMesh(
      new SphereGeometry(0.11, 10, 8),
      new MeshBasicMaterial({ color: 0xffffff, fog: false }),
      TRAIL * MAX_RUNS
    )
    this.comets.frustumCulled = false
    this.comets.count = 0
    this.comets.setColorAt(0, this.palette.ink)
    this.scene.add(this.comets)
    if (this.data) {
      this.syncBlocks()
      this.syncLines()
    }
  }

  // The floor's lines, and an arrow along its front that names the flow's stages.
  private buildFloor() {
    const { floor, edge } = this.palette
    const group = new Group()
    const grid: Vector3[] = []
    for (let x = -26; x <= 22; x += 2) {
      grid.push(new Vector3(x, 0, -14), new Vector3(x, 0, 14))
    }
    for (let z = -14; z <= 14; z += 2) {
      grid.push(new Vector3(-26, 0, z), new Vector3(22, 0, z))
    }
    group.add(
      new LineSegments(
        new BufferGeometry().setFromPoints(grid),
        new LineBasicMaterial({
          color: floor,
          transparent: true,
          opacity: 0.8,
        })
      )
    )
    const { start, end } = AXIS
    group.add(
      new Line(
        new BufferGeometry().setFromPoints([
          new Vector3(start, 0.01, AXIS.z),
          new Vector3(end, 0.01, AXIS.z),
        ]),
        new LineBasicMaterial({ color: edge })
      )
    )
    const head = new BufferGeometry().setFromPoints([
      new Vector3(end + 0.5, 0.01, AXIS.z),
      new Vector3(end - 0.1, 0.01, AXIS.z - 0.26),
      new Vector3(end - 0.1, 0.01, AXIS.z + 0.26),
    ])
    head.setIndex([0, 1, 2])
    group.add(
      new Mesh(head, new MeshBasicMaterial({ color: edge, side: DoubleSide }))
    )
    this.scene.add(group)
  }

  private syncBlocks() {
    if (!this.data) return
    ;(Object.keys(BLOCKS) as SceneBlock[]).forEach((id) => {
      const view = this.data?.views[id]
      if (!view) return
      const members = [...view.members, ...(view.shortMembers ?? [])]
      const signature = members.map((m) => `${m.id}:${m.tone}`).join(',')
      const held = this.blocks.get(id)
      if (held?.signature === signature) return
      if (held) {
        this.scene.remove(held.group)
        disposeTree(held.group)
      }
      this.blocks.set(id, this.buildBlock(id, members, signature))
    })
  }

  private toneColor(member: Member) {
    const { ink, up, down, muted } = this.palette
    if (member.tone === 'up') return up
    if (member.tone === 'down' || member.tone === 'alert') return down
    return member.tone === 'off' ? muted : ink
  }

  private buildBlock(
    id: SceneBlock,
    members: Member[],
    signature: string
  ): BlockParts {
    const { gate, ink } = this.palette
    const place = BLOCKS[id]
    const { w, d, h } = blockSize(id)
    const group = new Group()

    const pick = new Mesh(
      new BoxGeometry(1.2, h, d),
      new MeshBasicMaterial({ side: DoubleSide })
    )
    pick.position.set(place.x, h / 2, place.z)
    pick.visible = false
    pick.userData.pick = id
    group.add(pick)

    const geometry = new BoxGeometry(w, h, d)
    const panel = new Mesh(
      geometry,
      new MeshBasicMaterial({ color: gate, transparent: true, opacity: 0.94 })
    )
    panel.position.copy(pick.position)
    group.add(panel)
    const lines = new LineSegments(
      new EdgesGeometry(geometry),
      new LineBasicMaterial({ color: ink, transparent: true, opacity: 0.5 })
    )
    lines.position.copy(pick.position)
    group.add(lines)

    const shown = members.slice(0, MAX_DOTS)
    const slots = new Map<string, number>()
    const rest: Color[] = []
    let fill: InstancedMesh | null = null
    if (shown.length) {
      const cols = Math.max(1, Math.ceil(Math.sqrt((shown.length * d) / h)))
      const rows = Math.ceil(shown.length / cols)
      const pitch = Math.min((d - 0.4) / cols, (h - 0.4) / rows, 0.5)
      const radius = pitch * 0.3
      const at = new Object3D()
      const put = (index: number, x: number) => {
        at.position.set(
          x,
          h / 2 + ((rows - 1) * pitch) / 2 - Math.floor(index / cols) * pitch,
          place.z - ((cols - 1) * pitch) / 2 + (index % cols) * pitch
        )
        at.updateMatrix()
      }
      const stopped = shown.filter((member) => member.tone === 'off').length
      const filled = shown.length - stopped
      if (filled) {
        // A short rod through the gate, so it reads from either side.
        const rod = new CylinderGeometry(radius, radius, w + 0.02, 12)
        rod.rotateZ(Math.PI / 2)
        fill = new InstancedMesh(
          rod,
          new MeshBasicMaterial({ color: 0xffffff }),
          filled
        )
      }
      // A ring on each face, so a stopped member reads from either side too.
      let hollow: InstancedMesh | null = null
      if (stopped) {
        const ring = new RingGeometry(radius * 0.6, radius, 18)
        ring.rotateY(Math.PI / 2)
        hollow = new InstancedMesh(
          ring,
          new MeshBasicMaterial({
            color: this.palette.ink,
            side: DoubleSide,
          }),
          stopped * 2
        )
      }
      let on = 0
      let off = 0
      shown.forEach((member, index) => {
        if (member.tone === 'off') {
          ;[-1, 1].forEach((face) => {
            put(index, place.x + face * (w / 2 + 0.004))
            hollow?.setMatrixAt(off++, at.matrix)
          })
          return
        }
        put(index, place.x)
        const color = this.toneColor(member)
        fill?.setMatrixAt(on, at.matrix)
        fill?.setColorAt(on, color)
        slots.set(member.id, on)
        rest[on] = color
        on++
      })
      if (fill) group.add(fill)
      if (hollow) group.add(hollow)
    }
    this.scene.add(group)
    return { group, pick, fill, slots, rest, signature }
  }

  private syncLines() {
    if (!this.data) return
    ;(Object.keys(LINES) as LineKey[]).forEach((key) => {
      const place = LINES[key]
      const height = curtainHeight(this.data?.volumes[key] ?? 0, place.deck)
      const signature = height.toFixed(2)
      const held = this.lines.get(key)
      if (held?.signature === signature) return
      if (held) {
        this.scene.remove(held.group)
        disposeTree(held.group)
      }
      this.lines.set(key, this.buildLine(key, height, signature))
    })
    this.applyFocus()
  }

  private lineColor(key: LineKey) {
    const { muted, ink, down } = this.palette
    if (key.startsWith('market') || key.startsWith('exchange')) return muted
    if (key === 'models>agent_cost' || key === 'accounts>fees') return down
    return ink
  }

  private buildLine(
    key: LineKey,
    height: number,
    signature: string
  ): LineParts {
    const { bg, dark } = this.palette
    const place = LINES[key]
    const color = this.lineColor(key)
    const y0 = DECK_Y[place.deck]
    const curve = lineCurve(key, height)
    const group = new Group()
    const steps = 56
    const positions: number[] = []
    const colors: number[] = []
    const index: number[] = []
    const top: Vector3[] = []
    const bottom: Vector3[] = []
    const edgeColors: number[] = []
    const fade = new Color()
    for (let step = 0; step <= steps; step++) {
      const u = step / steps
      const point = curve.getPoint(u)
      positions.push(point.x, y0, point.z, point.x, y0 + height, point.z)
      top.push(new Vector3(point.x, y0 + height, point.z))
      bottom.push(new Vector3(point.x, y0, point.z))
      // Faint where it leaves and full where it arrives, so a still curtain says which way.
      fade.copy(bg).lerp(color, 0.2 + 0.8 * u)
      colors.push(fade.r, fade.g, fade.b, fade.r, fade.g, fade.b)
      edgeColors.push(fade.r, fade.g, fade.b)
      if (step < steps) {
        const a = step * 2
        index.push(a, a + 1, a + 2, a + 1, a + 3, a + 2)
      }
    }
    const sheet = new BufferGeometry()
    sheet.setAttribute('position', new Float32BufferAttribute(positions, 3))
    sheet.setAttribute('color', new Float32BufferAttribute(colors, 3))
    sheet.setIndex(index)
    const base = dark ? 0.34 : 0.2
    const curtain = new MeshBasicMaterial({
      vertexColors: true,
      transparent: true,
      opacity: base,
      side: DoubleSide,
      depthWrite: false,
    })
    group.add(new Mesh(sheet, curtain))

    const edge = new LineBasicMaterial({
      vertexColors: true,
      transparent: true,
      opacity: 0.9,
    })
    ;[top, bottom].forEach((points) => {
      const rim = new BufferGeometry().setFromPoints(points)
      rim.setAttribute('color', new Float32BufferAttribute(edgeColors, 3))
      group.add(new Line(rim, edge))
    })

    const middle = new BufferGeometry().setFromPoints(curve.getPoints(steps))
    middle.setAttribute('color', new Float32BufferAttribute(edgeColors, 3))
    group.add(new Line(middle, edge))

    // Arrowheads along it, so the direction reads with nothing moving and from any side.
    const count = Math.max(1, Math.floor(curve.getLength() / 6))
    const marks = new MeshBasicMaterial({
      color,
      transparent: true,
      opacity: 0.8,
    })
    const heads = new InstancedMesh(
      new ConeGeometry(0.13, 0.42, 12),
      marks,
      count
    )
    const head = new Object3D()
    const up = new Vector3(0, 1, 0)
    for (let mark = 0; mark < count; mark++) {
      const u = (mark + 0.72) / count
      head.position.copy(curve.getPoint(u))
      head.quaternion.setFromUnitVectors(up, curve.getTangent(u))
      head.updateMatrix()
      heads.setMatrixAt(mark, head.matrix)
    }
    group.add(heads)

    // A taller invisible copy, so a hairline can still be pointed at.
    const pickHeight = Math.max(height, 1.1)
    const pickPositions: number[] = []
    for (let step = 0; step <= steps; step++) {
      const point = curve.getPoint(step / steps)
      pickPositions.push(
        point.x,
        y0,
        point.z,
        point.x,
        y0 + pickHeight,
        point.z
      )
    }
    const pickGeometry = new BufferGeometry()
    pickGeometry.setAttribute(
      'position',
      new Float32BufferAttribute(pickPositions, 3)
    )
    pickGeometry.setIndex(index)
    const pick = new Mesh(
      pickGeometry,
      new MeshBasicMaterial({ side: DoubleSide })
    )
    pick.visible = false
    pick.userData.pick = key
    group.add(pick)

    this.scene.add(group)
    return {
      group,
      curve,
      curtain,
      edge,
      marks,
      pick,
      signature,
      base,
    }
  }

  private applyFocus() {
    this.lines.forEach((parts, key) => {
      const running =
        this.strong === key ||
        this.hovered === key ||
        this.runs.some((run) => run.key === key) ||
        this.calmLines.some((line) => line.key === key)
      const shown = this.active(key) ? 1 : 0.25
      parts.curtain.opacity = parts.base * shown * (running ? 1.8 : 1)
      parts.edge.opacity = 0.9 * shown
      parts.marks.opacity = 0.8 * shown
    })
  }

  private ripple(block: SceneBlock, color: Color) {
    if (this.calm) return
    const place = BLOCKS[block]
    const { d } = blockSize(block)
    const mesh = new Mesh(
      new RingGeometry(0.95, 1, 64),
      new MeshBasicMaterial({
        color,
        transparent: true,
        opacity: 0.8,
        side: DoubleSide,
        depthWrite: false,
      })
    )
    mesh.rotation.x = -Math.PI / 2
    mesh.position.set(place.x, 0.03, place.z)
    mesh.scale.setScalar(d * 0.42)
    this.scene.add(mesh)
    this.rings.push({ mesh, t: 0 })
  }

  private resize() {
    const width = this.host.clientWidth
    const height = this.host.clientHeight
    if (!width || !height) return
    this.renderer.setSize(width, height, false)
    this.renderer.domElement.style.width = `${width}px`
    this.renderer.domElement.style.height = `${height}px`
    this.camera.aspect = width / height
    this.camera.updateProjectionMatrix()
    // A wide stage looks along the floor; a narrower one looks down, to use its height.
    this.pitch = Math.max(0.46, Math.min(0.7, 0.92 - this.camera.aspect * 0.18))
    this.shownPitch = this.pitch + this.tilt
    this.fit()
    this.scene.fog = new Fog(
      this.palette.bg,
      this.distance - 2,
      this.distance + 70
    )
    this.invalidate()
  }

  // The nearest the camera can sit at the start with every gate, its name and the axis in
  // view, and how far that picture sits off centre, which `tick` takes out with the lens.
  // Only a new size changes it: a camera that refits as the view turns makes the picture jump.
  private fit() {
    const corners: Vector3[] = [
      new Vector3(AXIS.start, 0, AXIS.z + 1),
      new Vector3(AXIS.end, 0, AXIS.z + 1),
    ]
    ;(Object.keys(BLOCKS) as SceneBlock[]).forEach((id) => {
      const { x, z } = BLOCKS[id]
      const { d, h } = blockSize(id)
      ;[z - d / 2, z + d / 2].forEach((edge) =>
        [0, h].forEach((y) => corners.push(new Vector3(x, y, edge)))
      )
    })
    const names = (Object.keys(BLOCKS) as SceneBlock[]).map(
      (id) => new Vector3(BLOCKS[id].x, blockSize(id).h, BLOCKS[id].z)
    )
    const nameSide = (2 * LABEL_HALF_PX) / this.host.clientWidth
    const nameUp = (2 * LABEL_TALL_PX) / this.host.clientHeight
    this.camera.clearViewOffset()
    let near = 30
    let far = 400
    for (let i = 0; i < 12; i++) {
      const mid = (near + far) / 2
      this.placeCamera(mid, YAW, this.pitch)
      const box = new Box2()
      const take = (point: Vector3, side = 0, up = 0) => {
        const p = point.clone().project(this.camera)
        box.expandByPoint(new Vector2(p.x - side, p.y))
        box.expandByPoint(new Vector2(p.x + side, p.y + up))
      }
      corners.forEach((corner) => take(corner))
      names.forEach((name) => take(name, nameSide, nameUp))
      const size = box.getSize(new Vector2())
      if (Math.max(size.x, size.y) / 2 <= FRAME) {
        far = mid
        box.getCenter(this.lens)
      } else near = mid
    }
    this.distance = far
  }

  private placeCamera(distance: number, yaw: number, pitch: number) {
    this.camera.position.set(
      TARGET.x + Math.sin(yaw) * Math.cos(pitch) * distance,
      TARGET.y + Math.sin(pitch) * distance,
      TARGET.z + Math.cos(yaw) * Math.cos(pitch) * distance
    )
    this.camera.lookAt(TARGET)
    this.camera.updateMatrixWorld()
  }

  private invalidate() {
    if (this.disposed || this.frame) return
    this.frame = requestAnimationFrame(this.tick)
  }

  private readonly tick = (now: number) => {
    this.frame = 0
    if (this.disposed) return
    const dt = this.last ? Math.min(0.05, (now - this.last) / 1000) : 1 / 60
    this.last = now
    const moving = this.advance(dt)
    this.pickUnderPointer()
    const reach =
      this.zoom * (1 + Math.max(0, this.shownPitch - this.pitch) * TILT_BACK)
    this.placeCamera(this.distance * reach, this.shownYaw, this.shownPitch)
    const { clientWidth: width, clientHeight: height } = this.host
    this.camera.setViewOffset(
      width,
      height,
      ((this.lens.x / reach) * width) / 2,
      ((-this.lens.y / reach) * height) / 2,
      width,
      height
    )
    this.renderer.render(this.scene, this.camera)
    this.callbacks.onFrame(this.project)
    if (moving) this.invalidate()
    else this.last = 0
  }

  private readonly project = (anchor: string) => {
    const at = this.anchor(anchor)
    if (!at) return null
    const p = at.project(this.camera)
    return {
      x: ((p.x + 1) / 2) * this.host.clientWidth,
      y: ((1 - p.y) / 2) * this.host.clientHeight,
    }
  }

  private anchor(name: string) {
    if (name.startsWith('stage:')) {
      const stage = STAGES.find((row) => `stage:${row.key}` === name)
      return stage ? new Vector3(stage.x, 0, AXIS.z + 1) : null
    }
    if (name.includes('>')) {
      const parts = this.lines.get(name as LineKey)
      return parts
        ? parts.curve.getPoint(0.7).add(new Vector3(0, 0.8, 0))
        : null
    }
    const place = BLOCKS[name as SceneBlock]
    if (!place) return null
    return new Vector3(place.x, blockSize(name as SceneBlock).h + 0.12, place.z)
  }

  // Moves everything on by dt, and says whether anything still is.
  private advance(dt: number) {
    let moving = false

    const goalYaw = YAW + this.yaw
    const goalPitch = this.pitch + this.tilt
    if (this.press?.dragging || this.calm) {
      this.shownYaw = goalYaw
      this.shownPitch = goalPitch
    } else {
      const ease = 1 - Math.exp(-dt * 6)
      this.shownYaw += (goalYaw - this.shownYaw) * ease
      this.shownPitch += (goalPitch - this.shownPitch) * ease
      if (
        Math.abs(goalYaw - this.shownYaw) > 1e-4 ||
        Math.abs(goalPitch - this.shownPitch) > 1e-4
      ) {
        moving = true
      }
    }

    const comets = this.comets
    let drawn = 0
    const dummy = new Object3D()
    const trail = new Color()
    const before = this.runs.length
    const arrivals: (() => void)[] = []
    this.runs = this.runs.filter((run) => {
      run.t += dt / TRAVEL_S
      const head = Math.min(1, run.t)
      for (let i = 0; i < TRAIL && comets; i++) {
        const u = head - i * TRAIL_STEP
        // Once it has arrived, its tail catches up.
        if (u < 0 || (run.t > 1 && u < run.t - 1 - 0.1)) continue
        dummy.position.copy(run.curve.getPoint(run.reverse ? 1 - u : u))
        dummy.scale.setScalar(1 - i / TRAIL)
        dummy.updateMatrix()
        comets.setMatrixAt(drawn, dummy.matrix)
        comets.setColorAt(
          drawn,
          trail.copy(run.color).lerp(this.palette.bg, (i / TRAIL) * 0.9)
        )
        drawn++
      }
      run.glow.position.copy(run.curve.getPoint(run.reverse ? 1 - head : head))
      if (run.t >= 1 && !run.arrived) {
        run.arrived = true
        run.glow.visible = false
        const place = LINES[run.key]
        this.ripple(run.reverse ? place.from : place.to, run.color)
        if (run.done) arrivals.push(run.done)
      }
      if (run.t > 1.4) {
        this.scene.remove(run.glow)
        run.glow.material.dispose()
        return false
      }
      return true
    })
    if (comets) {
      comets.count = drawn
      comets.instanceMatrix.needsUpdate = true
      if (comets.instanceColor) comets.instanceColor.needsUpdate = true
    }
    // After the list is settled, as an arrival can send the next one on its way.
    arrivals.forEach((arrived) => arrived())
    if (this.runs.length) moving = true

    this.rings = this.rings.filter((ring) => {
      ring.t += dt / RING_S
      const material = ring.mesh.material as MeshBasicMaterial
      ring.mesh.scale.multiplyScalar(1 + dt * 1.6)
      material.opacity = Math.max(0, 0.8 * (1 - ring.t))
      if (ring.t < 1) return true
      this.scene.remove(ring.mesh)
      ring.mesh.geometry.dispose()
      material.dispose()
      return false
    })
    if (this.rings.length) moving = true

    this.lit = this.lit.filter((row) => {
      row.t += dt / MEMBER_LIT_S
      if (row.t < 1) return true
      const parts = this.blocks.get(row.block)
      const rest = parts?.rest[row.slot]
      if (parts?.fill?.instanceColor && rest) {
        parts.fill.setColorAt(row.slot, rest)
        parts.fill.instanceColor.needsUpdate = true
      }
      return false
    })
    if (this.lit.length) moving = true

    const held = this.calmLines.length
    this.calmLines = this.calmLines.filter((row) => {
      row.t += dt / CALM_S
      return row.t < 1
    })
    if (this.calmLines.length) moving = true
    if (before !== this.runs.length || held !== this.calmLines.length) {
      this.applyFocus()
    }
    return moving
  }

  private pickUnderPointer() {
    if (!this.pointer || this.press?.dragging) return
    const key = this.pick(this.pointer)
    if (key === this.hovered) return
    this.hovered = key
    this.renderer.domElement.style.cursor = key ? 'pointer' : 'grab'
    this.applyFocus()
    this.callbacks.onHover(key)
    this.invalidate()
  }

  private pick(at: { x: number; y: number }): string | null {
    const ndc = new Vector2(
      (at.x / this.host.clientWidth) * 2 - 1,
      -(at.y / this.host.clientHeight) * 2 + 1
    )
    this.raycaster.setFromCamera(ndc, this.camera)
    const targets: Object3D[] = []
    this.blocks.forEach((parts) => targets.push(parts.pick))
    this.lines.forEach((parts) => targets.push(parts.pick))
    const hit = this.raycaster.intersectObjects(targets, false)[0]
    return (hit?.object.userData.pick as string | undefined) ?? null
  }

  private readonly onPointerDown = (event: PointerEvent) => {
    this.press = {
      x: event.clientX,
      y: event.clientY,
      yaw: this.yaw,
      tilt: this.tilt,
      dragging: false,
    }
    this.renderer.domElement.setPointerCapture(event.pointerId)
  }

  private readonly onPointerMove = (event: PointerEvent) => {
    const box = this.host.getBoundingClientRect()
    this.pointer = { x: event.clientX - box.left, y: event.clientY - box.top }
    const press = this.press
    if (press) {
      const dx = event.clientX - press.x
      const dy = event.clientY - press.y
      if (!press.dragging && Math.hypot(dx, dy) > DRAG_START_PX) {
        press.dragging = true
        this.renderer.domElement.style.cursor = 'grabbing'
        this.noteMoved(true)
      }
      if (press.dragging) {
        this.yaw = Math.max(
          YAW_RANGE[0],
          Math.min(YAW_RANGE[1], press.yaw - dx * 0.0025)
        )
        this.tilt = Math.max(
          TILT_RANGE[0],
          Math.min(TILT_RANGE[1], press.tilt + dy * 0.002)
        )
      }
    }
    this.invalidate()
  }

  private readonly onPointerUp = () => {
    const press = this.press
    this.press = null
    this.renderer.domElement.style.cursor = this.hovered ? 'pointer' : 'grab'
    if (press && !press.dragging && this.pointer) {
      this.callbacks.onPick(this.pick(this.pointer))
    }
  }

  // Zoom takes Ctrl or Command, or a pinch, as on the charts: a plain wheel scrolls the page.
  private readonly onWheel = (event: WheelEvent) => {
    if (!event.ctrlKey && !event.metaKey) return
    event.preventDefault()
    this.zoom = Math.max(
      ZOOM_MIN,
      Math.min(ZOOM_MAX, this.zoom * Math.exp(event.deltaY * 0.004))
    )
    this.noteMoved(true)
    this.invalidate()
  }

  private readonly onPointerLeave = () => {
    this.pointer = null
    if (this.hovered) {
      this.hovered = null
      this.applyFocus()
      this.callbacks.onHover(null)
    }
    this.invalidate()
  }
}
