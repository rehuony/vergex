import { useState } from 'react'
import { api } from '@/api'
import { t, type Language } from '@/i18n'
import { Dialog, DialogField, DialogNote } from '@/components/common/dialog'
import { StoredText } from '@/components/common/stored-text'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import { useModelConfigs } from '@/hooks/use-connections'
import type {
  AgentConfig,
  DryRun,
  GridConfig,
  TokenEstimate,
  TraderKind,
} from '@/types'

// Tries the config being edited, saved or not, on live public market data.
// Previewing builds the prompts only; running also sends them to a model on
// the user's key, which is why the cost is stated first. Neither places orders.
export function DryRunDialog({
  kind,
  config,
  estimate,
  language,
  onClose,
}: {
  kind: TraderKind
  config: AgentConfig | GridConfig
  estimate?: TokenEstimate
  language: Language
  onClose: () => void
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderSettings.${key}`, language, params)
  const { data: allModels } = useModelConfigs()
  const models = (allModels ?? []).filter((model) => model.enabled)
  const [chosen, setChosen] = useState('')
  const modelId = chosen || models[0]?.id || ''
  const model = models.find((m) => m.id === modelId)

  const [running, setRunning] = useState<'preview' | 'model' | null>(null)
  const [result, setResult] = useState<DryRun | null>(null)
  const [failure, setFailure] = useState('')

  const run = async (withModel: boolean) => {
    setRunning(withModel ? 'model' : 'preview')
    setFailure('')
    try {
      setResult(
        await api.dryRun({
          kind,
          config,
          agent_model_id: withModel ? modelId : undefined,
        })
      )
    } catch (err) {
      setResult(null)
      setFailure(err instanceof Error ? err.message : label('testFailed'))
    } finally {
      setRunning(null)
    }
  }

  const candidates = result?.candidates ?? []

  return (
    <Dialog
      size="lg"
      title={label('testTitle')}
      onClose={onClose}
      closeLabel={label('close')}
      // A set height, so the buttons stay put when the results come in.
      bodyClassName="flex h-[min(64vh,560px)] flex-col gap-4 overflow-y-auto px-6 py-5"
      footer={
        <>
          <Button
            type="button"
            variant="secondary"
            disabled={running !== null}
            onClick={() => void run(false)}
          >
            {running === 'preview' ? label('building') : label('preview')}
          </Button>
          <Button
            type="button"
            variant="primary"
            disabled={running !== null || !model}
            onClick={() => void run(true)}
          >
            {running === 'model' ? label('running') : label('runTest')}
          </Button>
        </>
      }
    >
      <>
        {allModels && models.length === 0 ? (
          <DialogNote>
            {label('testNoModel')}{' '}
            <SettingsLink section="models" className="font-medium underline">
              {label('openSettings')}
            </SettingsLink>
          </DialogNote>
        ) : (
          <>
            <DialogField label={label('testModel')} htmlFor="dry-run-model">
              <AppSelect
                id="dry-run-model"
                value={modelId}
                disabled={running !== null}
                options={models.map((m) => ({
                  value: m.id,
                  label: m.customModelName
                    ? `${m.name} · ${m.customModelName}`
                    : m.name,
                }))}
                onChange={setChosen}
              />
            </DialogField>
            <DialogNote tone="warning">
              {estimate
                ? label('testCost', {
                    tokens: estimate.total.toLocaleString('en-US'),
                    model: model?.name ?? '—',
                  })
                : label('testCostUnknown', { model: model?.name ?? '—' })}
            </DialogNote>
          </>
        )}

        {failure && (
          <p role="alert" className="text-[13px] text-vergex-danger">
            {failure}
          </p>
        )}

        {!result && !failure && (
          <p className="flex flex-1 items-center justify-center text-center text-sm text-vergex-muted">
            {label('testEmpty')}
          </p>
        )}

        {result && (
          <div>
            <dl className="flex flex-wrap gap-x-6 gap-y-1 pb-3 text-[13px]">
              <div className="flex gap-2">
                <dt className="text-vergex-muted">{label('testCandidates')}</dt>
                <dd className="font-medium text-vergex-ink">
                  {candidates.length > 0
                    ? candidates.map((candidate) => candidate.symbol).join(', ')
                    : '—'}
                </dd>
              </div>
            </dl>
            {result.agent_error && (
              <p role="alert" className="pb-3 text-[13px] text-vergex-danger">
                {label('testModelFailed')} {result.agent_error}
              </p>
            )}
            <StoredText
              title={label('modelAnswer')}
              text={result.agent_response}
            />
            <StoredText
              title={label('systemPrompt')}
              text={result.system_prompt}
            />
            <StoredText title={label('userPrompt')} text={result.user_prompt} />
          </div>
        )}
      </>
    </Dialog>
  )
}
