interface BrandLogoProps {
  compact?: boolean
  className?: string
}

/**
 * The approved Flow identity, with a vector mark for crisp small-size use.
 * It is a mark to click, not text to select or an image to drag.
 */
export function BrandLogo({ compact = false, className = '' }: BrandLogoProps) {
  return (
    <span
      className={`vergex-brand select-none ${className}`}
      aria-label="Vergex"
    >
      <img
        src="/icons/vergex-flow.svg"
        alt=""
        width="38"
        height="33"
        draggable={false}
      />
      {!compact && (
        <span className="vergex-wordmark" aria-hidden="true">
          Vergex
        </span>
      )}
    </span>
  )
}
