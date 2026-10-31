import { OverlayScrollbars } from 'overlayscrollbars'
import 'overlayscrollbars/overlayscrollbars.css'

// The page's scrollbar. Where the platform's scrollbars take up width
// (Windows, macOS with a mouse), OverlayScrollbars draws one over the page
// that shows while scrolling and takes no space. Where they already overlay
// (macOS trackpads, phones) it cancels itself and the native one stays.
let page: OverlayScrollbars | undefined
let locks = 0

/** The look shared by the page's scrollbar and every ScrollArea. */
export const scrollbarOptions = {
  scrollbars: {
    theme: 'os-theme-vergex',
    autoHide: 'scroll',
    autoHideDelay: 800,
    clickScroll: true,
  },
} as const

export function initPageScrollbars() {
  const instance = OverlayScrollbars(
    {
      target: document.body,
      cancel: { nativeScrollbarsOverlaid: true, body: null },
    },
    scrollbarOptions
  )
  page = OverlayScrollbars.valid(instance) ? instance : undefined
}

/**
 * Stops the page scrolling behind a dialog; call the returned function to
 * let it scroll again. Locks nest. With OverlayScrollbars the html element is
 * the scroller, so body's overflow would not reach it; its own option does.
 */
export function lockPageScroll(): () => void {
  if (locks++ === 0) {
    if (page) page.options({ overflow: { y: 'hidden' } })
    else document.body.style.overflow = 'hidden'
  }
  let released = false
  return () => {
    if (released) return
    released = true
    if (--locks > 0) return
    if (page) page.options({ overflow: { y: 'scroll' } })
    else document.body.style.overflow = ''
  }
}
