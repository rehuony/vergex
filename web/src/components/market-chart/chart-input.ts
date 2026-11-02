export type WheelIntent = 'page' | 'pan' | 'zoom'

// What a turn of the wheel over the chart means. The chart sits in a page that scrolls,
// so an upright scroll belongs to the page, even when a trackpad adds a little sideways
// drift. A sideways scroll moves along the time axis. Ctrl or Command with the wheel
// zooms, as does a trackpad's pinch, which browsers report as Ctrl and the wheel.
export function wheelIntent(event: {
  ctrlKey: boolean
  metaKey: boolean
  deltaX: number
  deltaY: number
}): WheelIntent {
  if (event.ctrlKey || event.metaKey) return 'zoom'
  return Math.abs(event.deltaX) > Math.abs(event.deltaY) ? 'pan' : 'page'
}
