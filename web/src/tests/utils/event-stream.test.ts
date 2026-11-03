import { describe, expect, it } from 'vitest'
import { readEventStream } from '@/utils/event-stream'

function body(chunks: string[]) {
  const encoder = new TextEncoder()
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
      controller.close()
    },
  })
}

describe('readEventStream', () => {
  it('puts events back together across chunks, whatever ends their lines', async () => {
    const heard: unknown[] = []
    await readEventStream(
      body(['data: {"a":', '1}\n', '\nignored: x\r\n\r\ndata: {"a":2}\n\n']),
      (event) => heard.push(event)
    )
    expect(heard).toEqual([{ a: 1 }, { a: 2 }])
  })

  it('refuses a frame that never ends', async () => {
    await expect(
      readEventStream(body(['x'.repeat((1 << 20) + 1)]), () => {})
    ).rejects.toThrow('size limit')
  })
})
