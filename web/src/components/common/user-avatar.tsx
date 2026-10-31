import { cn } from '@/utils/cn'

interface UserAvatarProps {
  email: string
  avatar?: string
  /** Size and text classes, e.g. "size-14 text-xl"; defaults to 32px. */
  className?: string
}

/** The uploaded image, or the email's first letter when none is set. */
export function UserAvatar({ email, avatar, className }: UserAvatarProps) {
  const base = 'size-8 shrink-0 rounded-full text-[13px]'
  return avatar ? (
    <img src={avatar} alt="" className={cn(base, 'object-cover', className)} />
  ) : (
    <span
      aria-hidden="true"
      className={cn(
        base,
        'inline-grid place-items-center bg-[var(--brand-soft)] font-semibold text-vergex-brand',
        className
      )}
    >
      {email.charAt(0).toUpperCase()}
    </span>
  )
}
