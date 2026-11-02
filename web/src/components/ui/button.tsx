import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/utils/cn'

// The app's one button style. Tune a single button with Tailwind classes in
// className: cn() merges them, so they override these defaults reliably.
export const buttonVariants = cva(
  'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-medium transition-colors hover:duration-0 focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-vergex-ink/25 disabled:cursor-default disabled:opacity-45',
  {
    variants: {
      variant: {
        primary:
          'bg-vergex-ink font-semibold text-vergex-on-ink hover:bg-vergex-ink-hover',
        secondary:
          'border border-vergex-line bg-vergex-surface text-vergex-ink hover:bg-vergex-surface-muted',
        danger:
          'border border-vergex-danger-border bg-vergex-surface text-vergex-danger hover:bg-vergex-danger-bg',
        destructive:
          'bg-vergex-danger font-semibold text-white hover:bg-vergex-danger/90 focus-visible:ring-vergex-danger/25',
        success:
          'border border-vergex-success-border bg-vergex-surface text-vergex-success hover:bg-vergex-success-bg',
        soft: 'bg-vergex-ink-tint text-vergex-ink hover:bg-vergex-ink/12',
        ghost:
          'text-vergex-muted hover:bg-vergex-surface-muted hover:text-vergex-ink',
        // Inline text action, e.g. "Manage account" in a card footer.
        link: 'text-vergex-ink underline-offset-4 hover:underline',
        // One choice inside a bordered segmented group; aria-pressed marks it.
        segment:
          'text-vergex-muted hover:text-vergex-ink aria-pressed:bg-vergex-ink-tint aria-pressed:text-vergex-ink',
        // One of several choices; pass aria-pressed for the selected one.
        option:
          'border border-vergex-line bg-vergex-surface text-vergex-muted hover:border-vergex-line-strong aria-pressed:border-vergex-ink aria-pressed:bg-vergex-ink-tint aria-pressed:text-vergex-ink',
        // Holds a value picked from a list, so it has the input look rather than
        // a pill: AppSelect's trigger and any popover that does the same job.
        select:
          'justify-between rounded-md border border-vergex-line bg-vergex-surface text-left font-normal text-vergex-ink hover:border-vergex-line-strong focus-visible:border-vergex-ink/40 focus-visible:ring-1 focus-visible:ring-vergex-ink/10 data-[state=open]:border-vergex-ink/40',
      },
      size: {
        sm: 'h-8 px-3',
        md: 'h-9 px-4',
        lg: 'h-10 px-5',
        icon: 'h-9 w-9',
      },
    },
    // A text link sits in the line: no box from the size.
    compoundVariants: [
      { variant: 'link', class: 'h-auto px-0' },
      { variant: 'select', class: 'px-3' },
    ],
    defaultVariants: {
      variant: 'secondary',
      size: 'md',
    },
  }
)

export interface ButtonProps
  extends
    ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  /** Render the child element (e.g. a router Link) with the button style. */
  asChild?: boolean
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button'
    return (
      <Comp
        ref={ref}
        className={cn(buttonVariants({ variant, size }), className)}
        {...props}
      />
    )
  }
)
Button.displayName = 'Button'
