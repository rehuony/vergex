// How long an event takes along a line, however long the line is.
const TRAVEL_MS = 560
const TRAIL_DOTS = 8
const REDUCED_MS = 500
/** Past this many at once, an event is counted but not drawn. */
const MAX_RUNNING = 12

/** How much larger a dot draws while an event passes through it. */
const LIT_SCALE = 1.8

interface Run {
  dots: SVGCircleElement[]
  color: string
  startAt: number
  duration: number
  /** Dots before this index have already been lit and put back. */
  cleared: number
  lit: number
  done?: () => void
}

// Each dot keeps its resting size in data-rest, as the size follows the scene's grid.
function light(dot: SVGCircleElement, color: string) {
  dot.style.fill = color
  dot.setAttribute('r', String(Number(dot.dataset.rest) * LIT_SCALE))
}

function rest(dot: SVGCircleElement) {
  dot.style.removeProperty('fill')
  dot.setAttribute('r', dot.dataset.rest ?? '1')
}

// Lights dots in sequence. It draws directly on the elements, as React would only
// repeat this every frame, and it only asks for frames while something is running.
export function createFlowRunner() {
  let runs: Run[] = []
  let frame = 0

  const tick = (now: number) => {
    frame = 0
    runs = runs.filter((run) => {
      if (now < run.startAt) return true
      const progress = (now - run.startAt) / run.duration
      const head = Math.min(
        Math.ceil(progress * (run.dots.length + TRAIL_DOTS)),
        run.dots.length + TRAIL_DOTS
      )
      for (; run.lit < Math.min(head, run.dots.length); run.lit++) {
        light(run.dots[run.lit], run.color)
      }
      for (; run.cleared < head - TRAIL_DOTS; run.cleared++) {
        rest(run.dots[run.cleared])
      }
      if (run.cleared < run.dots.length) return true
      run.done?.()
      return false
    })
    if (runs.length > 0) frame = requestAnimationFrame(tick)
  }

  return {
    /** Returns false when too much is already running to draw this. */
    fire(
      dots: SVGCircleElement[],
      color: string,
      options: { delay?: number; done?: () => void } = {}
    ) {
      if (dots.length === 0 || runs.length >= MAX_RUNNING) return false
      if (
        typeof matchMedia === 'function' &&
        matchMedia('(prefers-reduced-motion: reduce)').matches
      ) {
        dots.forEach((dot) => light(dot, color))
        setTimeout(() => {
          dots.forEach(rest)
          options.done?.()
        }, REDUCED_MS)
        return true
      }
      runs.push({
        dots,
        color,
        startAt: performance.now() + (options.delay ?? 0),
        duration: TRAVEL_MS,
        cleared: 0,
        lit: 0,
        done: options.done,
      })
      if (!frame) frame = requestAnimationFrame(tick)
      return true
    },
    stop() {
      if (frame) cancelAnimationFrame(frame)
      frame = 0
      runs.forEach((run) => run.dots.forEach(rest))
      runs = []
    },
  }
}

export type FlowRunner = ReturnType<typeof createFlowRunner>
