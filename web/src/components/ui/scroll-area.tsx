import {
  useEffect,
  useRef,
  type HTMLAttributes,
  type MutableRefObject,
} from 'react'
import { OverlayScrollbars } from 'overlayscrollbars'
import { cn } from '@/utils/cn'
import { scrollbarOptions } from '@/utils/page-scroll'

/**
 * A box that scrolls with the page's overlay scrollbar, so a long list takes
 * no width from its rows. Size and pad the box with className (the library
 * carries the padding into the viewport and resets the viewport's own); lay
 * out the children with viewportClassName.
 */
export function ScrollArea({
  className,
  viewportClassName,
  viewportRef,
  onViewportScroll,
  children,
  ...props
}: HTMLAttributes<HTMLDivElement> & {
  viewportClassName?: string
  /** The element that actually scrolls, for callers that follow new content. */
  viewportRef?: MutableRefObject<HTMLDivElement | null>
  onViewportScroll?: () => void
}) {
  const host = useRef<HTMLDivElement>(null)
  const viewport = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!host.current || !viewport.current) return
    // The viewport is ours, so React keeps its children; the library only
    // adds its scrollbars next to it. Where scrollbars already overlay, it
    // steps back and the viewport scrolls natively.
    const instance = OverlayScrollbars(
      {
        target: host.current,
        elements: { viewport: viewport.current },
        cancel: { nativeScrollbarsOverlaid: true },
      },
      scrollbarOptions
    )
    return () => instance.destroy()
  }, [])

  return (
    <div ref={host} className={cn('relative', className)} {...props}>
      <div
        ref={(node) => {
          viewport.current = node
          if (viewportRef) viewportRef.current = node
        }}
        onScroll={onViewportScroll}
        className={cn('h-full overflow-y-auto', viewportClassName)}
      >
        {children}
      </div>
    </div>
  )
}
