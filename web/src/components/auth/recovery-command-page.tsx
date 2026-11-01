import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Check, Copy, type LucideIcon } from 'lucide-react'
import { toast } from 'sonner'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'
import { ROUTES } from '@/constants/route-paths'
import { BrandLogo } from '@/components/common/brand-logo'
import { Button } from '@/components/ui/button'

// Recovery runs only on the server (see cli.go), inside the backend container
// that docker-compose.yml names vergex-backend.
export const RECOVERY_CLI = 'docker exec -it vergex-backend ./vergex'

/** A centred page with one server command for a forgotten password or account. */
export function RecoveryCommandPage({
  icon: Icon,
  tone,
  title,
  intro,
  command,
  note,
}: {
  icon: LucideIcon
  tone: 'brand' | 'danger'
  title: string
  intro: string
  command: string
  note: string
}) {
  const { language } = useLanguage()
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), 2000)
    return () => clearTimeout(timer)
  }, [copied])

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(command)
      setCopied(true)
    } catch {
      toast.error(t('copyFailed', language))
    }
  }

  // The logo is the way back to sign in.
  const homeLink = (small: boolean) => (
    <Link
      to={ROUTES.login}
      aria-label={t('backToLogin', language)}
      draggable={false}
      className="inline-flex rounded-lg focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-vergex-accent/25"
    >
      <BrandLogo small={small} />
    </Link>
  )

  // The logo sits in the page's corner where the sign-in page has it, at the
  // same size.
  return (
    <div className="flex min-h-screen flex-col bg-vergex-bg">
      <header className="flex p-6 sm:p-12 xl:p-16">
        <span className="flex lg:hidden">{homeLink(true)}</span>
        <span className="hidden lg:flex">{homeLink(false)}</span>
      </header>

      <main className="flex flex-1 items-center justify-center px-6 pb-6 sm:px-12 sm:pb-12 lg:pb-16">
        <div className="w-full max-w-sm">
          <div className="mb-8 text-center">
            <div
              className={cn(
                'mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full',
                tone === 'danger'
                  ? 'bg-vergex-danger/10 text-vergex-danger'
                  : 'bg-[var(--brand-soft)] text-vergex-brand'
              )}
            >
              <Icon className="h-8 w-8" />
            </div>
            <h1 className="text-2xl font-bold text-vergex-text">{title}</h1>
          </div>

          <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-6 shadow-[var(--shadow-sm)]">
            <p className="mb-4 text-sm leading-relaxed text-vergex-text">
              {intro}
            </p>

            <div className="flex items-center justify-between gap-3 rounded-lg border border-[var(--line)] bg-[var(--canvas)] px-3 py-2 font-mono text-xs">
              <code className="text-vergex-brand [overflow-wrap:anywhere]">
                {command}
              </code>
              <Button
                variant="ghost"
                size="icon"
                className="size-8 shrink-0"
                type="button"
                onClick={() => void handleCopy()}
                aria-label={t(copied ? 'copied' : 'copy', language)}
              >
                {copied ? (
                  <Check className="h-4 w-4 text-vergex-success" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </Button>
            </div>

            <p
              className={cn(
                'mt-4 text-xs leading-relaxed',
                tone === 'danger'
                  ? 'text-vergex-danger'
                  : 'text-vergex-text-muted'
              )}
            >
              {note}
            </p>
          </div>
        </div>
      </main>
    </div>
  )
}
