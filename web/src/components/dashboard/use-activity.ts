import { useEffect, useRef, useState } from 'react'
import {
  streamActivity,
  type ActivityEvent,
  type ActivityStream,
  type ActivityTotals,
} from '@/api/dashboard'

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
  /** How the market streams stand, once the feed has said. */
  stream: ActivityStream | null
}

const initial: Activity = {
  events: [],
  totals: {},
  loaded: false,
  stale: false,
  at: 0,
  stream: null,
}

// Follows the activity stream, reconnecting until the page closes. onNew hears only
// events that just happened, never the ones a connection starts with, and onStream each
// round of what the market streams delivered.
export function useActivity(
  enabled: boolean,
  onNew: (events: ActivityEvent[]) => void,
  onStream: (flow: ActivityStream) => void
): Activity {
  const [activity, setActivity] = useState(initial)
  const listener = useRef(onNew)
  const streamListener = useRef(onStream)
  useEffect(() => {
    listener.current = onNew
    streamListener.current = onStream
  })

  useEffect(() => {
    if (!enabled) return
    const stopped = new AbortController()
    let retry = RETRY_MIN_MS
    let flowAt = 0

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
          const flow = message.stream
          if (flow && flow.at !== flowAt) {
            flowAt = flow.at
            streamListener.current(flow)
          }
          // These come every few seconds, and only what they say about the stream is kept.
          if (message.type === 'stream') {
            setActivity((current) =>
              current.stream?.connected === flow?.connected &&
              current.stream?.topics === flow?.topics
                ? current
                : { ...current, stream: flow ?? null }
            )
            return
          }
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
            stream: flow ?? current.stream,
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
