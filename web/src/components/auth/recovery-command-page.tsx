import { useEffect, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'
import { AuthSurface } from '@/components/auth/auth-surface'
import { Button } from '@/components/ui/button'

// Recovery runs only on the server (see cli.go), inside the backend container
// that docker-compose.yml names vergex-backend.
export const RECOVERY_CLI = 'docker exec -it vergex-backend ./vergex'

/** One server command for a forgotten password or account. */
export function RecoveryCommandPage({
  tone,
  title,
  intro,
  command,
  note,
}: {
  tone: 'neutral' | 'danger'
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

  return (
    <AuthSurface title={title} intro={intro} linkHome>
      <div className="flex items-center justify-between gap-3 rounded-md border border-vergex-line bg-vergex-surface px-3 py-2 font-mono text-xs">
        <code className="text-vergex-ink [overflow-wrap:anywhere]">
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
            <Check className="h-4 w-4 text-vergex-ink" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </Button>
      </div>

      <p
        className={cn(
          'mt-4 text-center text-xs leading-relaxed',
          tone === 'danger' ? 'text-vergex-danger' : 'text-vergex-muted'
        )}
      >
        {note}
      </p>
    </AuthSurface>
  )
}
