import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/utils/cn'

// The app's one button style. Tune a single button with Tailwind classes in
// className: cn() merges them, so they override these defaults reliably.
export const buttonVariants = cva(
  'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium transition-colors hover:duration-0 focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-vergex-accent/25 disabled:cursor-not-allowed disabled:opacity-45',
  {
    variants: {
      variant: {
        primary:
          'bg-vergex-brand font-semibold text-white hover:bg-vergex-brand-hover',
        secondary:
          'border border-[var(--line)] bg-[var(--surface)] text-vergex-text hover:bg-vergex-bg',
        danger:
          'border border-vergex-danger/30 bg-[var(--surface)] text-vergex-danger hover:bg-vergex-danger/10',
        destructive:
          'bg-vergex-danger font-semibold text-white hover:bg-vergex-danger/90 focus-visible:ring-vergex-danger/25',
        success:
          'border border-vergex-success/30 bg-[var(--surface)] text-vergex-success hover:bg-vergex-success/10',
        soft: 'bg-[var(--brand-soft)] text-vergex-brand hover:bg-vergex-brand/15',
        ghost:
          'text-vergex-text-muted hover:bg-vergex-bg hover:text-vergex-text',
        // Inline text action, e.g. "Manage account" in a card footer.
        link: 'text-vergex-brand underline-offset-4 hover:underline',
        // One choice inside a bordered segmented group; aria-pressed marks it.
        segment:
          'text-vergex-text-muted hover:text-vergex-text aria-pressed:bg-[var(--brand-soft)] aria-pressed:font-semibold aria-pressed:text-vergex-brand',
        // One of several choices; pass aria-pressed for the selected one.
        option:
          'border border-[var(--line)] bg-[var(--surface)] text-vergex-text-muted hover:border-[var(--line-strong)] aria-pressed:border-vergex-brand aria-pressed:bg-vergex-brand/10 aria-pressed:text-vergex-text',
      },
      size: {
        sm: 'h-9 px-3',
        md: 'h-10 px-4',
        lg: 'h-11 px-5',
        icon: 'h-9 w-9',
      },
    },
    // A text link sits in the line: no box from the size.
    compoundVariants: [{ variant: 'link', class: 'h-auto px-0' }],
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
