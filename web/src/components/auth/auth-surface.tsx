import { useState, type ComponentProps, type ReactNode } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'
import { ROUTES } from '@/constants/route-paths'
import { BrandLogo } from '@/components/common/brand-logo'
import { dialogInputClass } from '@/components/common/dialog'
import { Button } from '@/components/ui/button'

export const authInputClass = cn(
  dialogInputClass,
  'h-10 max-sm:text-base aria-[invalid=true]:border-vergex-danger'
)

// The one scene of the sign-in family: the wordmark, a large title and a 400px
// column centred on the canvas. linkHome makes the wordmark the way back to
// sign in.
export function AuthSurface({
  title,
  intro,
  linkHome = false,
  children,
}: {
  title: string
  intro?: string
  linkHome?: boolean
  children: ReactNode
}) {
  const { language } = useLanguage()
  return (
    <main className="vergex-auth">
      <div className="vergex-auth-column animate-page-in">
        <div className="flex justify-center">
          {linkHome ? (
            <Link
              to={ROUTES.login}
              aria-label={t('backToLogin', language)}
              draggable={false}
              className="inline-flex rounded-md focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-vergex-ink/25"
            >
              <BrandLogo />
            </Link>
          ) : (
            <BrandLogo />
          )}
        </div>
        <h1>{title}</h1>
        {intro && (
          <p className="mt-4 text-center text-sm text-vergex-muted">{intro}</p>
        )}
        <div className="mt-8">{children}</div>
      </div>
    </main>
  )
}

// A label over its control, with room on the label's line for an action.
export function AuthField({
  label,
  htmlFor,
  action,
  children,
}: {
  label: string
  htmlFor: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <label
          htmlFor={htmlFor}
          className="text-[13px] font-medium text-vergex-ink"
        >
          {label}
        </label>
        {action}
      </div>
      {children}
    </div>
  )
}

export function AuthPassword({
  className,
  ...props
}: Omit<ComponentProps<'input'>, 'type'>) {
  const { language } = useLanguage()
  const [shown, setShown] = useState(false)
  return (
    <div className="relative">
      <input
        {...props}
        type={shown ? 'text' : 'password'}
        className={cn(authInputClass, 'pr-10', className)}
      />
      <Button
        variant="ghost"
        size="icon"
        className="absolute right-1 top-1/2 size-8 -translate-y-1/2"
        type="button"
        onClick={() => setShown(!shown)}
        aria-label={t(shown ? 'hidePassword' : 'showPassword', language)}
        aria-pressed={shown}
      >
        {shown ? <EyeOff size={16} /> : <Eye size={16} />}
      </Button>
    </div>
  )
}

// One line above the button holds a failure or a notice, so a message never
// pushes the form around.
export function AuthMessage({
  id,
  error = false,
  children,
}: {
  id?: string
  error?: boolean
  children: string
}) {
  return (
    <p
      id={id}
      aria-live="polite"
      title={children || undefined}
      className={cn(
        'mt-2 h-5 truncate text-xs leading-5',
        error ? 'text-vergex-danger' : 'text-vergex-muted'
      )}
    >
      {children}
    </p>
  )
}
