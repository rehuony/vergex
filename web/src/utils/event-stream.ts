// The most one event may hold, so a stream that never ends a frame cannot grow without end.
const MAX_FRAME = 1 << 20

// Reads a stream of server-sent events, handing each to onEvent until the stream ends.
export async function readEventStream<E>(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: E) => void
): Promise<void> {
  const reader = body.getReader(),
    decoder = new TextDecoder()
  let buffer = ''
  try {
    for (;;) {
      const { done, value } = await reader.read()
      buffer += decoder.decode(value, { stream: !done })
      buffer = buffer.replace(/\r\n/g, '\n')
      let at: number
      while ((at = buffer.indexOf('\n\n')) >= 0) {
        const frame = buffer.slice(0, at)
        buffer = buffer.slice(at + 2)
        for (const line of frame.split('\n'))
          if (line.startsWith('data: ')) onEvent(JSON.parse(line.slice(6)) as E)
      }
      if (buffer.length > MAX_FRAME)
        throw new Error('An event exceeds the size limit')
      if (done) return
    }
  } finally {
    await reader.cancel().catch(() => {})
  }
}
