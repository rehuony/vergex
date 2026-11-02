import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { FlaskConical, Loader2 } from 'lucide-react'
import { api } from '@/api'
import { notify } from '@/components/common/notify'
import { PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import { SettingActions } from '@/components/common/setting-row'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useAccounts, useModelConfigs } from '@/hooks/use-connections'
import { t, type Language } from '@/i18n'
import type {
  AgentConfig,
  CoinSourceConfig,
  GridConfig,
  TraderKind,
} from '@/types'
import { BasicSection, type BasicSettings } from './basic-section'
import { CoinSourceSection } from './coin-source-section'
import { DryRunDialog } from './dry-run-dialog'
import { GridSection, gridProblem } from './grid-section'
import { MarketDataSection } from './market-data-section'
import { PromptSection } from './prompt-section'
import { RiskProfileSection } from './risk-profile-section'
import { useTokenEstimate } from './use-token-estimate'

type SettingsPart = 'basic' | 'coins' | 'risk' | 'market' | 'prompt' | 'grid'

// A trader's own settings, edited in place: what it runs with, and its config. A draft stays
// until it is saved, so moving to another tab and back loses nothing.
export function TraderSettings({
  traderId,
  running,
  language,
  onDirty,
}: {
  traderId: string
  running: boolean
  language: Language
  // Says whether there is a draft worth asking about before leaving this trader.
  onDirty?: (dirty: boolean) => void
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderSettings.${key}`, language, params)
  const client = useQueryClient()
  const { data, error, refetch } = useQuery({
    queryKey: ['trader-config', traderId],
    queryFn: () => api.getTraderConfig(traderId, true),
  })
  const models = useModelConfigs()
  const accounts = useAccounts()
  const [draft, setDraft] = useState<AgentConfig | GridConfig | null>(null)
  const [basicDraft, setBasicDraft] = useState<BasicSettings | null>(null)
  const [chosenPart, setPart] = useState<SettingsPart | null>(null)
  const [saving, setSaving] = useState(false)
  const [dryRunning, setDryRunning] = useState(false)

  const kind: TraderKind | undefined = data?.kind
  const config = draft ?? data?.config ?? null
  const agent = kind === 'agent' ? (config as AgentConfig | null) : null
  const grid = kind === 'grid' ? (config as GridConfig | null) : null
  const estimate = useTokenEstimate(agent)
  const part: SettingsPart = chosenPart ?? (grid ? 'grid' : 'coins')

  const savedBasic: BasicSettings | null = data
    ? {
        model: data.agent_model,
        account: data.account_id,
        interval: data.scan_interval_minutes,
        cross: data.is_cross_margin,
      }
    : null
  const basic = basicDraft ?? savedBasic
  const basicChanged =
    Boolean(basicDraft && savedBasic) &&
    (basicDraft!.model !== savedBasic!.model ||
      basicDraft!.account !== savedBasic!.account ||
      basicDraft!.interval !== savedBasic!.interval ||
      basicDraft!.cross !== savedBasic!.cross)
  const changed = Boolean(draft) || basicChanged

  useEffect(() => {
    onDirty?.(changed)
    return () => onDirty?.(false)
  }, [changed])

  const patchAgent = (patch: (agent: AgentConfig) => AgentConfig) => {
    if (agent) setDraft(patch(structuredClone(agent)))
  }
  const patchCoinSource = (patch: Partial<CoinSourceConfig>) =>
    patchAgent((current) => ({
      ...current,
      coin_source: { ...current.coin_source, ...patch },
    }))
  const patchGrid = (patch: Partial<GridConfig>) => {
    if (grid) setDraft({ ...grid, ...patch })
  }

  const problem = grid ? gridProblem(grid, language) : ''

  // Pressing it with nothing changed, or with a problem the bar is already naming, does
  // nothing.
  const save = async () => {
    if (saving || !changed || problem) return
    setSaving(true)
    try {
      await api.updateTrader(traderId, {
        ...(draft ? { config: draft } : {}),
        ...(basicChanged && basicDraft
          ? {
              agent_model_id: basicDraft.model,
              account_id: basicDraft.account,
              scan_interval_minutes: basicDraft.interval,
              is_cross_margin: basicDraft.cross,
            }
          : {}),
      })
      await refetch()
      if (basicChanged)
        await client.invalidateQueries({ queryKey: ['traders'] })
      setDraft(null)
      setBasicDraft(null)
      notify.success(label('saved'))
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  if (!data || !config || !basic) {
    return error ? (
      <PanelState role="alert">
        {label('loadFailed')}
        <Button
          type="button"
          variant="secondary"
          size="sm"
          onClick={() => void refetch()}
        >
          {label('retry')}
        </Button>
      </PanelState>
    ) : (
      <PanelState role="status">
        <Loader2 className="size-5 animate-spin" />
      </PanelState>
    )
  }

  const pressed = estimate?.model_limits
    ?.filter((limit) => limit.level === 'danger')
    .sort((a, b) => b.usage_pct - a.usage_pct)[0]
  const tokens = estimate?.total.toLocaleString('en-US')
  const runningNote =
    draft && running
      ? label(kind === 'grid' ? 'runningRestarts' : 'runningNextCycle')
      : ''
  const lockedNote =
    part === 'basic' && running ? t('traderDetail.stopToEdit', language) : ''
  const message =
    problem ||
    lockedNote ||
    (pressed
      ? label('tokensOver', {
          tokens: tokens ?? '—',
          percent: pressed.usage_pct,
          model: pressed.name,
        })
      : runningNote || (tokens && label('tokens', { tokens })))
  const messageTone =
    problem || (pressed && !lockedNote)
      ? 'danger'
      : runningNote && !lockedNote
        ? 'warning'
        : 'muted'

  const parts: { value: SettingsPart; label: string }[] = [
    { value: 'basic', label: label('tabBasic') },
    ...(grid
      ? [{ value: 'grid' as const, label: label('tabGrid') }]
      : [
          { value: 'coins' as const, label: label('tabCoins') },
          { value: 'risk' as const, label: label('tabRisk') },
          { value: 'market' as const, label: label('tabMarket') },
          { value: 'prompt' as const, label: label('tabPrompt') },
        ]),
  ]

  return (
    <>
      <div className="shrink-0 overflow-x-auto border-b border-vergex-line px-4 py-2">
        <Segmented
          aria-label={label('tabs')}
          value={part}
          options={parts}
          onChange={setPart}
        />
      </div>
      <ScrollArea key={part} className="min-h-0 flex-1 px-4">
        {part === 'basic' && (
          <BasicSection
            kind={data.kind}
            basic={basic}
            models={(models.data ?? []).filter((model) => model.enabled)}
            accounts={accounts.data ?? []}
            locked={running}
            language={language}
            onPatch={(patch) => setBasicDraft({ ...basic, ...patch })}
          />
        )}
        {part === 'grid' && grid && (
          <GridSection grid={grid} language={language} onPatch={patchGrid} />
        )}
        {agent && part === 'coins' && (
          <CoinSourceSection
            agent={agent}
            language={language}
            onPatch={patchCoinSource}
          />
        )}
        {agent && part === 'risk' && (
          <RiskProfileSection
            agent={agent}
            language={language}
            onPatchAgent={patchAgent}
          />
        )}
        {agent && part === 'market' && (
          <MarketDataSection
            agent={agent}
            language={language}
            onPatchAgent={patchAgent}
          />
        )}
        {agent && part === 'prompt' && (
          <PromptSection
            agent={agent}
            language={language}
            onPatchAgent={patchAgent}
          />
        )}
      </ScrollArea>
      <div className="shrink-0 rounded-b-2xl bg-vergex-surface px-4">
        <SettingActions message={message} messageTone={messageTone}>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="max-sm:h-10"
            onClick={() => {
              if (!problem) setDryRunning(true)
            }}
          >
            <FlaskConical className="size-4" />
            {label('dryRun')}
          </Button>
          <Button
            type="button"
            variant="primary"
            size="sm"
            className="max-sm:h-10"
            onClick={() => void save()}
          >
            {saving ? label('saving') : label('save')}
          </Button>
        </SettingActions>
      </div>
      {dryRunning && kind && (
        <DryRunDialog
          kind={kind}
          config={config}
          estimate={estimate}
          language={language}
          onClose={() => setDryRunning(false)}
        />
      )}
    </>
  )
}
