import { Check, TriangleAlert } from 'lucide-react'
import type { AssistantActionResult } from '@/api/assistant'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'

// How each confirmed action went, one row each, as the server says: a check or a warning,
// what it was, and what came with it under that.
export function ActionResults({
  results,
}: {
  results: AssistantActionResult[]
}) {
  const { language } = useLanguage()
  return (
    <ul
      aria-label={t('assistant.results', language)}
      className="space-y-1.5 text-[13px] leading-relaxed"
    >
      {results.map((result, index) => (
        <li key={index} className="flex min-w-0 items-start gap-2">
          <span className="flex h-[20px] shrink-0 items-center">
            {result.ok ? (
              <Check
                className="size-3.5 text-vergex-muted"
                aria-label={t('assistant.done', language)}
              />
            ) : (
              <TriangleAlert
                className="size-3.5 text-vergex-danger"
                aria-label={t('assistant.failed', language)}
              />
            )}
          </span>
          <div className="min-w-0 flex-1">
            <p className="break-words text-vergex-ink">{result.text}</p>
            {result.note && (
              <p
                className={`whitespace-pre-wrap break-words text-xs ${result.ok ? 'text-vergex-muted' : 'text-vergex-danger'}`}
              >
                {result.note}
              </p>
            )}
          </div>
        </li>
      ))}
    </ul>
  )
}
