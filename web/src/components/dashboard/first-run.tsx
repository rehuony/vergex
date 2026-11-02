import { Link } from 'react-router-dom'
import { Check } from 'lucide-react'
import type { Overview } from '@/api/dashboard'
import { Panel } from '@/components/common/panel'
import { SettingsLink } from '@/components/settings/settings-link'
import { buttonVariants } from '@/components/ui/button'
import { ROUTES } from '@/constants/route-paths'
import { cn } from '@/utils/cn'

type Label = (key: string, params?: Record<string, string | number>) => string

// What a new account sees instead of a scene with nothing in it: the three steps
// that give the scene something to show, with the next one to do picked out.
export function FirstRun({
  overview,
  label,
}: {
  overview: Overview
  label: Label
}) {
  const hasModel = overview.models.length > 0
  const hasAccount = overview.accounts.length > 0
  const action = (primary: boolean) =>
    buttonVariants({ variant: primary ? 'primary' : 'secondary', size: 'sm' })
  const steps = [
    {
      name: label('setup.model'),
      done: hasModel,
      action: (
        <SettingsLink section="models" className={action(!hasModel)}>
          {label('setup.addModel')}
        </SettingsLink>
      ),
    },
    {
      name: label('setup.account'),
      done: hasAccount,
      action: (
        <SettingsLink
          section="accounts"
          className={action(hasModel && !hasAccount)}
        >
          {label('setup.addAccount')}
        </SettingsLink>
      ),
    },
    {
      name: label('setup.run'),
      done: false,
      action: (
        <span className="flex gap-2">
          <Link to={ROUTES.traders} className={action(false)}>
            {label('flow.createTrader')}
          </Link>
          <Link to={ROUTES.futures} className={action(false)}>
            {label('flow.planTrade')}
          </Link>
        </span>
      ),
    },
  ]
  return (
    <Panel title={label('setup.title')}>
      <ol className="divide-y divide-vergex-line border-t border-vergex-line">
        {steps.map((step, index) => (
          <li
            key={step.name}
            className="flex min-h-16 flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3"
          >
            <span
              className={cn(
                'flex size-6 shrink-0 items-center justify-center rounded-full text-xs tabular-nums',
                step.done
                  ? 'bg-vergex-success/15 text-vergex-success'
                  : 'bg-vergex-surface-muted text-vergex-muted'
              )}
            >
              {step.done ? (
                <Check className="size-3.5" aria-label={label('setup.done')} />
              ) : (
                index + 1
              )}
            </span>
            <span className="min-w-0 flex-1 text-sm text-vergex-ink max-sm:basis-[calc(100%-36px)]">
              {step.name}
            </span>
            {!step.done && <span className="max-sm:pl-9">{step.action}</span>}
          </li>
        ))}
      </ol>
    </Panel>
  )
}
