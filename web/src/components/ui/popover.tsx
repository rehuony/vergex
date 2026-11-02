import {
  forwardRef,
  type ComponentPropsWithoutRef,
  type ElementRef,
} from 'react'
import * as PopoverPrimitive from '@radix-ui/react-popover'
import { cn } from '@/utils/cn'

export const Popover = PopoverPrimitive.Root
export const PopoverTrigger = PopoverPrimitive.Trigger
// What it lines up with, where that is more than its trigger.
export const PopoverAnchor = PopoverPrimitive.Anchor

// A floating panel styled like the app's menus. Radix supplies focus handling,
// Escape and closing on an outside click; it grows from its trigger.
export const PopoverContent = forwardRef<
  ElementRef<typeof PopoverPrimitive.Content>,
  ComponentPropsWithoutRef<typeof PopoverPrimitive.Content>
>(({ className, align = 'start', sideOffset = 4, ...props }, ref) => (
  <PopoverPrimitive.Portal>
    <PopoverPrimitive.Content
      ref={ref}
      align={align}
      sideOffset={sideOffset}
      className={cn(
        'z-[60] origin-[var(--radix-popover-content-transform-origin)] animate-menu-in rounded-2xl border border-vergex-line bg-vergex-surface shadow-(--shadow-float) outline-none',
        className
      )}
      {...props}
    />
  </PopoverPrimitive.Portal>
))
PopoverContent.displayName = 'PopoverContent'
