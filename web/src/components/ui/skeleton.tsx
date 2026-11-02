import { cn } from '@/utils/cn'

// Where a value will be until it is read, at about its size, so nothing moves when it comes.
export function Skeleton({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'skeleton inline-block h-[1em] w-16 max-w-full animate-pulse align-[-0.125em] motion-reduce:animate-none',
        className
      )}
    />
  )
}
