import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { t, type Language } from '@/i18n'
import type { DecisionRecord } from '@/types'
import { Dialog } from '@/components/common/dialog'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { StoredText } from '@/components/common/stored-text'
import { cn } from '@/utils/cn'
import { LogLine } from './decision-log'
import { actionName, actionSide, fmtDateTime } from './decision-format'
import { baseLabel, fmtPx } from '@/utils/trader-format'

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="pb-4">
      <h3 className="mb-2 text-[13px] font-medium text-vergex-ink">{title}</h3>
      {children}
    </section>
  )
}

const headClass = 'px-2 py-1.5 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-1.5 align-top whitespace-nowrap'

const count = (value: number) => value.toLocaleString('en-US')

// What the cycle's calls to the model came to. A reply cut off at its length limit is worth
// saying plainly, since what follows it can only be part of an answer.
function ModelUse({
  use,
  language,
}: {
  use: NonNullable<DecisionRecord['model_use']>
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderDetail.${key}`, language, params)
  const cutOff =
    use.finish_reason === 'length' || use.finish_reason === 'max_tokens'
  return (
    <>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4">
        {[
          [label('modelName'), use.model],
          [label('modelTime'), `${(use.duration_ms / 1000).toFixed(1)} s`],
          [
            label('modelTokens'),
            `${count(use.prompt_tokens)} / ${count(use.completion_tokens)}`,
          ],
          [
            label('modelCallCost'),
            use.unpriced ? label('unpriced') : `$${use.cost_usd.toFixed(4)}`,
          ],
        ].map(([name, value]) => (
          <div key={name} className="min-w-0" title={value}>
            <dt className="truncate text-xs text-vergex-muted">{name}</dt>
            <dd className="mono mt-0.5 truncate text-[13px] text-vergex-ink">
              {value}
            </dd>
          </div>
        ))}
      </dl>
      {cutOff && (
        <p className="mt-2 text-[13px] text-vergex-warning">
          {label('replyCutOff')}
        </p>
      )}
    </>
  )
}

// DecisionDetailDialog shows one cycle in full: each action and its result,
// the execution log, then the stored prompts and the model's raw response.
export function DecisionDetailDialog({
  traderId,
  decisionId,
  language,
  onClose,
}: {
  traderId: string
  decisionId: number
  language: Language
  onClose: () => void
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderDetail.${key}`, language, params)
  const { data: record, error } = useQuery({
    queryKey: ['decision', traderId, decisionId],
    queryFn: () => api.getDecision(traderId, decisionId, true),
    refetchOnWindowFocus: false,
    retry: false,
  })
  const actions = record?.decisions ?? []
  const logs = record?.execution_log ?? []
  return (
    <Dialog
      size="xl"
      title={
        record
          ? label('cycleNumber', { cycle: record.seq })
          : label('decisionDetail')
      }
      description={record ? fmtDateTime(record.timestamp) : undefined}
      closeLabel={label('close')}
      onClose={onClose}
    >
      {!record ? (
        error ? (
          <PanelState role="alert">{label('decisionsFailed')}</PanelState>
        ) : (
          <PanelLoading />
        )
      ) : (
        <>
          {record.error_message && (
            <p
              role="alert"
              className="mb-4 rounded-md border border-vergex-danger/30 bg-vergex-danger/10 px-3 py-2 text-[13px] break-words text-vergex-danger"
            >
              {record.error_message}
            </p>
          )}
          {record.model_use && (
            <Section title={label('modelUse')}>
              <ModelUse use={record.model_use} language={language} />
            </Section>
          )}
          <Section title={label('actions')}>
            {actions.length === 0 ? (
              <p className="text-[13px] text-vergex-muted">
                {label('noAction')}
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-[13px] tabular-nums">
                  <thead>
                    <tr>
                      <th className={headClass}>{label('symbol')}</th>
                      <th className={headClass}>{label('action')}</th>
                      <th className={cn(headClass, 'text-right')}>
                        {label('price')}
                      </th>
                      <th className={cn(headClass, 'text-right')}>
                        {label('leverage')}
                      </th>
                      <th className={cn(headClass, 'text-right')}>
                        {label('confidence')}
                      </th>
                      <th className={headClass}>{label('result')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {actions.map((action, index) => {
                      const side = actionSide(action.action)
                      const hold =
                        action.action === 'hold' || action.action === 'wait'
                      return (
                        <tr key={index}>
                          <td
                            className={cn(
                              cellClass,
                              'font-medium text-vergex-ink'
                            )}
                          >
                            {baseLabel(action.symbol)}
                          </td>
                          <td
                            className={cn(
                              cellClass,
                              side === 'long'
                                ? 'text-vergex-success'
                                : side === 'short'
                                  ? 'text-vergex-danger'
                                  : 'text-vergex-muted'
                            )}
                          >
                            {actionName(action.action, language)}
                          </td>
                          <td className={cn(cellClass, 'text-right')}>
                            {fmtPx(action.price)}
                          </td>
                          <td className={cn(cellClass, 'text-right')}>
                            {action.leverage ?? 0}×
                          </td>
                          <td className={cn(cellClass, 'text-right')}>
                            {action.confidence ?? 0}
                          </td>
                          <td className={cn(cellClass, 'whitespace-normal')}>
                            {hold ? (
                              '—'
                            ) : action.success ? (
                              <span className="text-vergex-success">
                                {label('filled')}
                              </span>
                            ) : (
                              <span className="break-words text-vergex-danger">
                                {action.error || label('notFilled')}
                              </span>
                            )}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </Section>
          {logs.length > 0 && (
            <Section title={label('executionLog')}>
              <ul className="flex flex-col gap-0.5">
                {logs.map((line, index) => (
                  <LogLine key={index} text={line} language={language} />
                ))}
              </ul>
            </Section>
          )}
          <StoredText title={label('reasoning')} text={record.cot_trace} />
          <StoredText title={label('rawResponse')} text={record.raw_response} />
          <StoredText title={label('inputPrompt')} text={record.input_prompt} />
          <StoredText
            title={label('systemPrompt')}
            text={record.system_prompt}
          />
        </>
      )}
    </Dialog>
  )
}
