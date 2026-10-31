import { useEffect, useRef, useState } from 'react'
import {
  streamActivity,
  type ActivityEvent,
  type ActivityTotals,
} from '@/api/activity'

/** How many events the list holds, newest first. */
const KEEP_EVENTS = 200
/** The stream sends totals every 20 seconds, so this long means it has gone quiet. */
const SILENCE_MS = 50_000
const RETRY_MIN_MS = 1_000
const RETRY_MAX_MS = 30_000

export interface Activity {
  events: ActivityEvent[]
  totals: ActivityTotals
  /** False until the first message arrives. */
  loaded: boolean
  /** The stream is down. What is shown is the last that arrived. */
  stale: boolean
  /** When the last message arrived, in epoch milliseconds. */
  at: number
}

const initial: Activity = {
  events: [],
  totals: {},
  loaded: false,
  stale: false,
  at: 0,
}

// Follows the activity stream, reconnecting until the page closes. onNew hears only
// events that just happened, never the ones a connection starts with.
export function useActivity(
  enabled: boolean,
  onNew: (events: ActivityEvent[]) => void
): Activity {
  const [activity, setActivity] = useState(initial)
  const listener = useRef(onNew)
  useEffect(() => {
    listener.current = onNew
  })

  useEffect(() => {
    if (!enabled) return
    const stopped = new AbortController()
    let retry = RETRY_MIN_MS

    const connect = async () => {
      const connection = new AbortController()
      const abort = () => connection.abort()
      stopped.signal.addEventListener('abort', abort)
      let silence = setTimeout(abort, SILENCE_MS)
      try {
        await streamActivity(connection.signal, (message) => {
          clearTimeout(silence)
          silence = setTimeout(abort, SILENCE_MS)
          retry = RETRY_MIN_MS
          setActivity((current) => ({
            events:
              message.type === 'snapshot'
                ? [...message.events].reverse().slice(0, KEEP_EVENTS)
                : [...[...message.events].reverse(), ...current.events].slice(
                    0,
                    KEEP_EVENTS
                  ),
            totals: message.totals,
            loaded: true,
            stale: false,
            at: message.at,
          }))
          if (message.type === 'events') listener.current(message.events)
        })
      } catch {
        // Handled below: a failed connection is retried like one that ended.
      } finally {
        clearTimeout(silence)
        stopped.signal.removeEventListener('abort', abort)
      }
    }

    const run = async () => {
      while (!stopped.signal.aborted) {
        await connect()
        if (stopped.signal.aborted) return
        setActivity((current) => ({ ...current, stale: true }))
        await new Promise((resolve) => setTimeout(resolve, retry))
        retry = Math.min(retry * 2, RETRY_MAX_MS)
      }
    }
    void run()
    return () => stopped.abort()
  }, [enabled])

  return activity
}
