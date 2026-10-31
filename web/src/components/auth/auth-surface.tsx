import type { HTMLAttributes } from 'react'
import { cn } from '@/utils/cn'

// The full-screen backdrop of the sign-in and registration pages.
export function AuthSurface({
  children,
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'relative flex min-h-screen w-full flex-col overflow-hidden bg-vergex-bg text-vergex-text',
        className
      )}
      {...props}
    >
      <div className="relative flex h-full w-full flex-1 flex-col">
        {children}
      </div>
    </div>
  )
}
