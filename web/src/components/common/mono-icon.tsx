import { cn } from '@/utils/cn'

interface MonoIconProps {
  src: string
  label: string
  width?: number
  height?: number
  className?: string
}

// A venue's or a model's mark in ink: the SVG only gives the shape, so the mark
// follows the theme and carries no brand colour.
export function MonoIcon({
  src,
  label,
  width = 24,
  height = 24,
  className,
}: MonoIconProps) {
  return (
    <span
      role="img"
      aria-label={label}
      className={cn('inline-block shrink-0 bg-vergex-ink', className)}
      style={{
        width,
        height,
        maskImage: `url(${src})`,
        maskPosition: 'center',
        maskRepeat: 'no-repeat',
        maskSize: 'contain',
      }}
    />
  )
}
