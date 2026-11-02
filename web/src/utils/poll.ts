// Options for data read on a timer. A failed read tries again at the same pace rather than
// straight away, so a page left open recovers on its own, and the exchange is never asked
// more often than usual.
export function pollEvery(ms: number) {
  return { refetchInterval: ms, retry: false }
}
