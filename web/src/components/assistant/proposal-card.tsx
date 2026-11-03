import { Check, TriangleAlert } from 'lucide-react'
import { Link } from 'react-router-dom'
import type { AssistantProposal } from '@/api/assistant'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import {
  openAssistantForm,
  useAssistantFormOpen,
  type AssistantForm,
} from '@/hooks/use-assistant-page'
import { t } from '@/i18n'

interface Row {
  name: string
  from: string
  to: string
}

const isPlainObject = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)

const asText = (value: unknown, empty: string): string => {
  if (value === undefined || value === null || value === '') return empty
  if (Array.isArray(value)) return value.length ? value.join(', ') : empty
  return typeof value === 'object' ? JSON.stringify(value) : String(value)
}

// What would change, one line each: a field of type object by the path to each key in it.
function changes(
  form: AssistantForm,
  values: Record<string, unknown>,
  empty: string
) {
  const rows: Row[] = []
  const walk = (name: string, from: unknown, to: unknown) => {
    if (isPlainObject(to)) {
      const before = isPlainObject(from) ? from : {}
      for (const [key, value] of Object.entries(to))
        walk(`${name}.${key}`, before[key], value)
      return
    }
    const [was, will] = [asText(from, empty), asText(to, empty)]
    if (was !== will) rows.push({ name, from: was, to: will })
  }
  for (const field of form.fields) {
    if (!(field.name in values)) continue
    const value = values[field.name]
    if (field.type === 'object') {
      const before = isPlainObject(field.value) ? field.value : {}
      const after = isPlainObject(value) ? value : {}
      for (const [key, inner] of Object.entries(after))
        walk(key, before[key], inner)
      continue
    }
    const shown = (raw: unknown) =>
      field.options?.find((option) => option.value === raw)?.label ??
      asText(raw, empty)
    const [was, will] = [shown(field.value), shown(value)]
    if (was !== will) rows.push({ name: field.label, from: was, to: will })
  }
  return rows
}

// What the assistant suggests for a form, beside what the form holds now. Nothing goes in
// until the button is pressed, and that only fills the form: saving stays on the page.
export function ProposalCard({
  proposal,
  onFill,
  onDismiss,
  onFollow,
}: {
  proposal: AssistantProposal
  onFill: () => void
  onDismiss: () => void
  onFollow: () => void
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`assistant.${key}`, language)
  const open = useAssistantFormOpen(proposal.form)
  const form = open ? openAssistantForm(proposal.form) : undefined

  if (proposal.filled)
    return (
      <p className="flex items-center gap-1.5 text-xs text-vergex-muted">
        <Check className="size-3.5 shrink-0" aria-hidden="true" />
        {label('filled')}
      </p>
    )

  const rows = form ? changes(form, proposal.values, label('emptyValue')) : []
  return (
    <section
      aria-label={proposal.title}
      className="rounded-md bg-vergex-surface-muted p-3 text-[13px] leading-relaxed text-vergex-ink"
    >
      <h3 className="truncate font-medium" title={proposal.title}>
        {proposal.title}
      </h3>
      {form && rows.length > 0 && (
        <dl className="mt-2 space-y-1">
          {rows.map((row) => (
            <div key={row.name} className="min-w-0">
              <dt
                className="truncate text-xs text-vergex-muted"
                title={row.name}
              >
                {row.name}
              </dt>
              <dd className="break-words">
                <span className="text-vergex-muted">{row.from}</span>
                {' → '}
                {row.to}
              </dd>
            </div>
          ))}
        </dl>
      )}
      {form && rows.length === 0 && (
        <p className="mt-2 text-vergex-muted">{label('noChange')}</p>
      )}
      {!form && (
        <p className="mt-2 flex items-center gap-1.5 text-vergex-muted">
          <TriangleAlert
            className="size-3.5 shrink-0 text-vergex-warning"
            aria-hidden="true"
          />
          {label('formGone')}
        </p>
      )}
      {proposal.reason && (
        <p className="mt-2 text-xs text-vergex-muted">{proposal.reason}</p>
      )}
      <div className="mt-3 flex justify-end gap-2">
        <Button
          type="button"
          size="sm"
          className="max-sm:h-10"
          onClick={onDismiss}
        >
          {label('dismissProposal')}
        </Button>
        {form ? (
          <Button
            type="button"
            variant="primary"
            size="sm"
            className="max-sm:h-10"
            onClick={onFill}
          >
            {label('fillIn')}
          </Button>
        ) : (
          proposal.route && (
            <Button asChild variant="primary" size="sm" className="max-sm:h-10">
              <Link to={proposal.route} onClick={onFollow}>
                {label('openForm')}
              </Link>
            </Button>
          )
        )}
      </div>
    </section>
  )
}
