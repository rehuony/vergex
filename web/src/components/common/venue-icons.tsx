import React from 'react'
import { cn } from '@/utils/cn'
import { MonoIcon } from './mono-icon'

interface IconProps {
  width?: number
  height?: number
  className?: string
}

// Local icon path mapping
const ICON_PATHS: Record<string, string> = {
  binance: '/venue-icons/binance.svg',
  okx: '/venue-icons/okx.svg',
}

// Fallback icon
const FallbackIcon: React.FC<IconProps & { label: string }> = ({
  width = 24,
  height = 24,
  className,
  label,
}) => (
  <div
    className={cn(
      'flex shrink-0 items-center justify-center rounded-md bg-vergex-surface-muted text-[11px] font-semibold text-vergex-ink',
      className
    )}
    style={{ width, height }}
  >
    {label[0]?.toUpperCase() || '?'}
  </div>
)

// Returns the icon for an exchange
export const getVenueIcon = (venue: string, props: IconProps = {}) => {
  const lowerType = venue.toLowerCase()
  const type = lowerType.includes('binance')
    ? 'binance'
    : lowerType.includes('okx')
      ? 'okx'
      : lowerType

  const iconProps = {
    width: props.width || 24,
    height: props.height || 24,
    className: props.className,
  }

  const path = ICON_PATHS[type]
  if (path) {
    return <MonoIcon {...iconProps} src={path} label={type} />
  }

  return <FallbackIcon {...iconProps} label={type} />
}
