import { useState } from 'react'
import { Pencil, Plus, RotateCw, Square, Trash2 } from 'lucide-react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/api'
import type { BacktestRun } from '@/api/forecast'
import {
  RailItem,
  SplitRail,
  SplitView,
  type RailAction,
} from '@/components/common/split-view'
import { confirmToast, notify } from '@/components/common/notify'
import { Panel, PanelState } from '@/components/common/panel'
import { SettingBadge } from '@/components/common/setting-row'
import { BacktestForm } from '@/components/forecast/backtest-form'
import { BacktestReportView } from '@/components/forecast/backtest-report'
import {
  baseAsset,
  fmtDate,
  fmtRate,
  fmtTime,
} from '@/components/forecast/forecast-format'
import { ForecastFrame } from '@/components/forecast/forecast-frame'
import { backtestsQuery } from '@/components/forecast/forecast-queries'
import { EditableTitle, NameInput } from '@/components/common/name-editing'
import { useStrategies } from '@/components/forecast/strategy-select'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AppSelect } from '@/components/ui/select'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'

const CANCELED = 'canceled'

// Past runs beside the report of the one opened, or the form for another. Each run in
// the list shows its result, so runs can be compared at a glance. The address says which
// is open, so a reload stays on it.
export function ForecastBacktestsPage() {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const [params, setParams] = useSearchParams()
  const askedStrategy = params.get('strategy')
  const askedRun = params.get('run')
  const [again, setAgain] = useState<BacktestRun | null>(null)
  const [renaming, setRenaming] = useState<number | null>(null)
  const [renamingHere, setRenamingHere] = useState(false)
  const { data: strategies } = useStrategies(language)
  const client = useQueryClient()
  const { data: runs, refetch } = useQuery({
    ...backtestsQuery,
    refetchInterval: (query) =>
      query.state.data?.backtests.some((run) => run.status === 'running')
        ? 2000
        : false,
  })

  const list: BacktestRun[] = runs?.backtests ?? []
  const strategyName = (run: BacktestRun) =>
    strategies?.strategies.find((item) => item.id === run.strategy_id)?.name ??
    '—'
  const runName = (run: BacktestRun) => run.name || strategyName(run)
  const usableStrategy = (run: BacktestRun) =>
    strategies?.strategies.some(
      (item) => item.id === run.strategy_id && !item.retired
    ) ?? false
  const running = list.some((run) => run.status === 'running')
  const wantsNew = askedRun === 'new' || (Boolean(askedStrategy) && !askedRun)
  const current = wantsNew
    ? undefined
    : (list.find((run) => String(run.id) === askedRun) ?? list[0])
  const showForm = wantsNew || (runs && list.length === 0)

  const { data: whole } = useQuery({
    queryKey: ['forecast-backtest', current?.id],
    queryFn: () => api.getForecastBacktest(current!.id),
    enabled: current?.status === 'finished',
  })

  const open = (id: number) => {
    setAgain(null)
    setParams({ run: String(id) }, { replace: true })
  }
  const openForm = (run: BacktestRun | null = null) => {
    setAgain(run)
    setParams({ run: 'new' }, { replace: true })
  }
  const started = async (run: BacktestRun) => {
    open(run.id)
    await refetch()
  }
  const cancel = async (id: number) => {
    try {
      await api.cancelForecastBacktest(id)
      await refetch()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('runFailed'))
    }
  }
  const rename = async (run: BacktestRun, name: string) => {
    setRenaming(null)
    setRenamingHere(false)
    if (!runs) return
    const renamed = {
      ...runs,
      backtests: runs.backtests.map((other) =>
        other.id === run.id ? { ...other, name } : other
      ),
    }
    client.setQueryData(backtestsQuery.queryKey, renamed)
    try {
      await api.renameForecastBacktest(run.id, name)
    } catch {
      client.setQueryData(backtestsQuery.queryKey, runs)
      notify.error(label('renameFailed'))
    }
  }
  const remove = async (run: BacktestRun) => {
    const name = runName(run)
    const ok = await confirmToast(label('deleteRunMessage', { name }), {
      title: label('deleteRun'),
      okText: label('deleteRun'),
      cancelText: label('deleteCancel'),
      tone: 'danger',
    })
    if (!ok) return
    try {
      await api.deleteForecastBacktest(run.id)
      if (run.id === current?.id) setParams({}, { replace: true })
      await refetch()
      notify.success(label('runDeleted', { name }))
    } catch (err) {
      notify.error(
        err instanceof Error ? err.message : label('deleteRunFailed')
      )
    }
  }

  const coinsText = (run: BacktestRun) =>
    `${run.options.symbols.map(baseAsset).join(', ')} · ${run.options.intervals.join(', ')}`
  const stateBadge = (run: BacktestRun) =>
    run.status === 'running' ? (
      <SettingBadge>
        {run.done} / {run.total}
      </SettingBadge>
    ) : run.status === 'failed' ? (
      <SettingBadge tone={run.error === CANCELED ? 'muted' : 'danger'}>
        {label(run.error === CANCELED ? 'runCanceledShort' : 'runFailedShort')}
      </SettingBadge>
    ) : null
  const actionsFor = (run: BacktestRun): RailAction[] => [
    {
      label: label('rename'),
      icon: Pencil,
      movesFocus: true,
      onSelect: () => setRenaming(run.id),
    },
    ...(usableStrategy(run)
      ? [
          {
            label: label('runAgain'),
            icon: RotateCw,
            onSelect: () => openForm(run),
          },
        ]
      : []),
    ...(run.status === 'running'
      ? [
          {
            label: label('runCancel'),
            icon: Square,
            onSelect: () => void cancel(run.id),
          },
        ]
      : []),
    {
      label: label('deleteRun'),
      icon: Trash2,
      danger: true,
      disabled: run.status === 'running',
      onSelect: () => void remove(run),
    },
  ]

  return (
    <ForecastFrame language={language}>
      <SplitView
        rail={
          <SplitRail
            label={label('runs')}
            addLabel={label('newBacktest')}
            onAdd={() => openForm()}
          >
            {list.length === 0 && (
              <li className="px-3 py-2 text-[13px] text-vergex-muted">
                {runs ? label('noRuns') : label('loading')}
              </li>
            )}
            {list.map((run) => {
              const summary = run.summary
              return (
                <RailItem
                  key={run.id}
                  current={!showForm && current?.id === run.id}
                  title={coinsText(run)}
                  name={runName(run)}
                  details={
                    <>
                      <span className="flex min-w-0 items-center gap-2 text-xs text-vergex-muted">
                        <span className="min-w-0 truncate">
                          {fmtTime(run.created_at)} ·{' '}
                          {run.options.intervals.join(', ')} ·{' '}
                          {label('coinCount', {
                            count: run.options.symbols.length,
                          })}
                        </span>
                        {stateBadge(run)}
                      </span>
                      {summary && summary.samples > 0 && (
                        <span className="truncate font-mono text-[11px] tabular-nums text-vergex-muted">
                          {fmtRate(summary.hit_rate)} /{' '}
                          {fmtRate(summary.chance_rate)} · {label('lift')}{' '}
                          {(summary.lift * 100).toFixed(1)}%
                        </span>
                      )}
                    </>
                  }
                  onOpen={() => open(run.id)}
                  onStartRename={() => setRenaming(run.id)}
                  rename={
                    renaming === run.id && (
                      <NameInput
                        row
                        value={run.name}
                        label={label('strategyName')}
                        placeholder={strategyName(run)}
                        allowEmpty
                        language={language}
                        onSave={(name) => void rename(run, name)}
                        onCancel={() => setRenaming(null)}
                      />
                    )
                  }
                  actions={actionsFor(run)}
                  actionsLabel={label('moreActions')}
                />
              )
            })}
          </SplitRail>
        }
      >
        <div className="flex min-h-0 min-w-0 flex-col gap-4">
          <div className="flex items-center gap-2 lg:hidden">
            <AppSelect
              aria-label={label('runs')}
              className="min-w-0 flex-1 max-sm:h-10"
              value={showForm ? 'new' : String(current?.id ?? 'new')}
              options={[
                { value: 'new', label: label('newBacktest') },
                ...list.map((run) => ({
                  value: String(run.id),
                  label: `${runName(run)} · ${fmtTime(run.created_at)}`,
                })),
              ]}
              onChange={(value) =>
                value === 'new' ? openForm() : open(Number(value))
              }
            />
            <Button
              type="button"
              variant="secondary"
              size="icon"
              aria-label={label('newBacktest')}
              className="shrink-0 max-sm:size-10"
              onClick={() => openForm()}
            >
              <Plus className="size-4" aria-hidden="true" />
            </Button>
          </div>

          {showForm ? (
            <BacktestForm
              key={again?.id ?? 'new'}
              initialStrategy={again?.strategy_id ?? askedStrategy}
              again={again}
              running={running}
              onStarted={(run) => void started(run)}
              language={language}
            />
          ) : !current ? (
            <Panel className="flex-1">
              <PanelState role="status">{label('loading')}</PanelState>
            </Panel>
          ) : (
            <Panel
              title={
                <EditableTitle
                  name={runName(current)}
                  editing={renamingHere}
                  renameLabel={label('rename')}
                  onEdit={() => setRenamingHere(true)}
                  field={
                    <NameInput
                      value={current.name}
                      label={label('strategyName')}
                      placeholder={strategyName(current)}
                      allowEmpty
                      language={language}
                      onSave={(name) => void rename(current, name)}
                      onCancel={() => setRenamingHere(false)}
                      className="max-w-64 md:max-w-96"
                    />
                  }
                />
              }
              meta={
                <span
                  className={cn('block truncate', renamingHere && 'invisible')}
                  title={coinsText(current)}
                >
                  {fmtDate(current.options.from)} –{' '}
                  {fmtDate(current.options.to)} · {coinsText(current)}
                </span>
              }
              actions={
                <>
                  {current.status === 'running' ? (
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      className="max-sm:h-10"
                      onClick={() => void cancel(current.id)}
                    >
                      {label('runCancel')}
                    </Button>
                  ) : (
                    usableStrategy(current) && (
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        className="max-sm:h-10"
                        onClick={() => openForm(current)}
                      >
                        {label('runAgain')}
                      </Button>
                    )
                  )}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="hover:text-vergex-danger max-sm:size-10"
                    aria-label={label('deleteRun')}
                    title={label('deleteRun')}
                    disabled={current.status === 'running'}
                    onClick={() => void remove(current)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </>
              }
              className="flex min-h-0 flex-1 flex-col overflow-hidden"
            >
              <ScrollArea
                key={current.id}
                className="min-h-0 flex-1 rounded-b-2xl border-t border-vergex-line"
              >
                {current.status === 'running' ? (
                  <PanelState role="status">
                    <div
                      role="progressbar"
                      aria-label={label('runRunning')}
                      aria-valuemin={0}
                      aria-valuemax={current.total}
                      aria-valuenow={current.done}
                      className="h-1 w-48 overflow-hidden rounded-full bg-vergex-line"
                    >
                      <div
                        className="h-full rounded-full bg-vergex-ink transition-[width]"
                        style={{
                          width: `${(current.done / Math.max(current.total, 1)) * 100}%`,
                        }}
                      />
                    </div>
                    {label('runProgress', {
                      done: current.done,
                      total: current.total,
                    })}
                  </PanelState>
                ) : current.status === 'failed' ? (
                  current.error === CANCELED ? (
                    <PanelState role="status">
                      {label('runCanceled')}
                    </PanelState>
                  ) : (
                    <PanelState role="alert">
                      {label('runFailedReason', {
                        reason: current.error ?? '—',
                      })}
                    </PanelState>
                  )
                ) : whole?.id === current.id && whole.report ? (
                  <BacktestReportView
                    report={whole.report}
                    language={language}
                  />
                ) : (
                  <PanelState role="status">{label('loading')}</PanelState>
                )}
              </ScrollArea>
            </Panel>
          )}
        </div>
      </SplitView>
    </ForecastFrame>
  )
}
