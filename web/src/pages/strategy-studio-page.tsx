import { useCallback, useEffect, useState } from 'react'
import { Copy, FlaskConical, Loader2, Plus, Trash2 } from 'lucide-react'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import { Page } from '@/components/common/page'
import { Panel, PanelState } from '@/components/common/panel'
import { SectionTabs } from '@/components/common/section-tabs'
import {
  SettingActions,
  SettingBadge,
  SettingRow,
} from '@/components/settings/setting-row'
import { autopilotTabs } from '@/components/trader/autopilot-tabs'
import { t } from '@/i18n/translations'
import { api } from '@/api'
import { confirmToast, notify } from '@/components/common/notify'
import { getAIConfig } from '@/utils/strategy-profiles'
import type {
  AIStrategyConfig,
  CoinSourceConfig,
  GridStrategyConfig,
  Strategy,
  StrategyConfig,
  StrategyType,
} from '@/types'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import {
  StrategyList,
  strategyTypeLabel,
} from '@/components/strategy/strategy-list'
import { CoinSourceSection } from '@/components/strategy/coin-source-section'
import { RiskProfileSection } from '@/components/strategy/risk-profile-section'
import { MarketDataSection } from '@/components/strategy/market-data-section'
import { PromptSection } from '@/components/strategy/prompt-section'
import { GridSection, gridProblem } from '@/components/strategy/grid-section'
import { fieldClass } from '@/components/common/form-fields'
import { NewStrategyDialog } from '@/components/strategy/new-strategy-dialog'
import { StrategyTestDialog } from '@/components/strategy/strategy-test-dialog'
import { useTokenEstimate } from '@/components/strategy/use-token-estimate'

type EditorTab = 'coins' | 'risk' | 'market' | 'prompt'

export function StrategyStudioPage() {
  const { token } = useAuth()
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`strategy.${key}`, language, params)
  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [selected, setSelected] = useState<Strategy | null>(null)
  const [config, setConfig] = useState<StrategyConfig | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [saving, setSaving] = useState(false)
  const [hasChanges, setHasChanges] = useState(false)
  const [tab, setTab] = useState<EditorTab>('coins')
  const [creating, setCreating] = useState(false)
  const [testing, setTesting] = useState(false)

  const ai = config ? getAIConfig(config) : null
  const isGrid = config?.strategy_type === 'grid_trading'
  const grid = isGrid ? config?.grid_config : null
  const estimate = useTokenEstimate(isGrid ? null : config)

  const select = (strategy: Strategy | null) => {
    setSelected(strategy)
    setConfig(strategy ? structuredClone(strategy.config) : null)
    setName(strategy?.name || '')
    setDescription(strategy?.description || '')
    setHasChanges(false)
  }

  const loadStrategies = useCallback(
    async (preferredId?: string) => {
      if (!token) return
      try {
        const result = await api.getStrategies()
        setStrategies(result)
        setFailed(false)
        select(
          (preferredId && result.find((s) => s.id === preferredId)) ||
            result.find((s) => s.is_active) ||
            result[0] ||
            null
        )
      } catch {
        setFailed(true)
      } finally {
        setLoading(false)
      }
    },
    [token]
  )

  useEffect(() => {
    void loadStrategies()
  }, [loadStrategies])

  const edited = () => setHasChanges(true)

  // Edits always write into ai_config; legacy flat fields are migrated there.
  const patchAI = (patch: (ai: AIStrategyConfig) => AIStrategyConfig) => {
    setConfig((prev) => {
      if (!prev) return prev
      const current = getAIConfig(prev)
      if (!current) return prev
      return {
        ...prev,
        strategy_type: prev.strategy_type || 'ai_trading',
        ai_config: patch(structuredClone(current)),
      }
    })
    edited()
  }

  const patchCoinSource = (patch: Partial<CoinSourceConfig>) =>
    patchAI((current) => ({
      ...current,
      coin_source: { ...current.coin_source, ...patch },
    }))

  const patchGrid = (patch: Partial<GridStrategyConfig>) => {
    setConfig((prev) =>
      prev?.grid_config
        ? { ...prev, grid_config: { ...prev.grid_config, ...patch } }
        : prev
    )
    edited()
  }

  // Moving away from unsaved edits asks first; they cannot be brought back.
  const mayDiscard = async () =>
    !hasChanges ||
    confirmToast(label('discardMessage'), {
      title: label('discardTitle'),
      okText: label('discard'),
      cancelText: label('keepEditing'),
      tone: 'danger',
    })

  const choose = async (strategy: Strategy) => {
    if (strategy.id === selected?.id) return
    if (await mayDiscard()) select(strategy)
  }

  const createStrategy = async (newName: string, type: StrategyType) => {
    try {
      const defaults = await api.getDefaultStrategyConfig(type)
      const created = await api.createStrategy({
        name: newName,
        description: '',
        config: defaults,
      })
      setCreating(false)
      await loadStrategies(created.id)
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('createFailed'))
    }
  }

  const saveStrategy = async (activateAfter = false) => {
    if (!selected || !config) return
    setSaving(true)
    try {
      await api.updateStrategy(selected.id, {
        name: name.trim(),
        description,
        config,
      })
      if (activateAfter) await api.activateStrategy(selected.id)
      notify.success(label(activateAfter ? 'savedActivated' : 'saved'))
      await loadStrategies(selected.id)
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const activateStrategy = async () => {
    if (!selected) return
    if (hasChanges) return saveStrategy(true)
    try {
      await api.activateStrategy(selected.id)
      notify.success(label('activated'))
      await loadStrategies(selected.id)
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('activateFailed'))
    }
  }

  const duplicateStrategy = async () => {
    if (!selected) return
    try {
      const copy = await api.duplicateStrategy(
        selected.id,
        label('copyName', { name: selected.name })
      )
      notify.success(label('duplicated'))
      await loadStrategies(copy.id)
    } catch (err) {
      notify.error(
        err instanceof Error ? err.message : label('duplicateFailed')
      )
    }
  }

  const deleteStrategy = async () => {
    if (!selected || selected.is_active) return
    const ok = await confirmToast(
      label('deleteMessage', { name: selected.name }),
      {
        title: label('deleteTitle'),
        okText: label('delete'),
        cancelText: label('cancel'),
        tone: 'danger',
      }
    )
    if (!ok) return
    try {
      await api.deleteStrategy(selected.id)
      notify.success(label('deleted'))
      await loadStrategies()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('deleteFailed'))
    }
  }

  const problem = !name.trim()
    ? label('needName')
    : grid
      ? gridProblem(grid, language)
      : ''
  const pressed = estimate?.model_limits
    ?.filter((limit) => limit.level === 'danger')
    .sort((a, b) => b.usage_pct - a.usage_pct)[0]
  const tokens = estimate?.total.toLocaleString('en-US')
  const message =
    problem ||
    (pressed
      ? label('tokensOver', {
          tokens: tokens ?? '—',
          percent: pressed.usage_pct,
          model: pressed.name,
        })
      : tokens && label('tokens', { tokens }))

  const editorTabs: { id: EditorTab; label: string }[] = [
    { id: 'coins', label: label('tabCoins') },
    { id: 'risk', label: label('tabRisk') },
    { id: 'market', label: label('tabMarket') },
    { id: 'prompt', label: label('tabPrompt') },
  ]

  return (
    <Page
      title={t('autopilotNav', language)}
      description={label('description')}
      tabs={autopilotTabs(language)}
      actions={
        <Button
          variant="primary"
          type="button"
          disabled={loading}
          onClick={async () => {
            if (await mayDiscard()) setCreating(true)
          }}
        >
          <Plus className="size-4" />
          {label('newStrategy')}
        </Button>
      }
    >
      {loading ? (
        <Panel>
          <PanelState role="status">
            <Loader2 className="size-5 animate-spin" />
          </PanelState>
        </Panel>
      ) : failed ? (
        <Panel>
          <PanelState role="alert">
            {label('loadFailed')}
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => void loadStrategies()}
            >
              {label('retry')}
            </Button>
          </PanelState>
        </Panel>
      ) : (
        <div className="grid grid-cols-1 items-start gap-4 md:grid-cols-[260px_minmax(0,1fr)]">
          <StrategyList
            className="hidden md:block"
            strategies={strategies}
            selectedId={selected?.id}
            onSelect={(strategy) => void choose(strategy)}
            language={language}
          />

          {strategies.length === 0 && (
            <Panel className="md:hidden">
              <PanelState role="status">{label('noStrategies')}</PanelState>
            </Panel>
          )}

          {selected && config && (
            <div className="flex min-w-0 flex-col gap-4">
              <AppSelect
                aria-label={label('chooseStrategy')}
                className="md:hidden"
                value={selected.id}
                options={strategies.map((strategy) => ({
                  value: strategy.id,
                  label: strategy.is_active
                    ? `${strategy.name} · ${label('active')}`
                    : strategy.name,
                }))}
                onChange={(id) => {
                  const next = strategies.find((s) => s.id === id)
                  if (next) void choose(next)
                }}
              />

              <Panel
                title={
                  <span className="inline-flex max-w-full items-center gap-2 max-md:hidden">
                    <span className="truncate">{selected.name}</span>
                    {selected.is_active && (
                      <SettingBadge tone="success">
                        {label('active')}
                      </SettingBadge>
                    )}
                  </span>
                }
                meta={strategyTypeLabel(selected, language)}
                actions={
                  <>
                    {!isGrid && (
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        className="max-sm:h-10"
                        onClick={() => setTesting(true)}
                      >
                        <FlaskConical className="size-4" />
                        {label('test')}
                      </Button>
                    )}
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="max-sm:size-10"
                      aria-label={label('duplicate')}
                      title={
                        hasChanges ? label('saveFirst') : label('duplicate')
                      }
                      disabled={hasChanges}
                      onClick={() => void duplicateStrategy()}
                    >
                      <Copy className="size-4" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="hover:text-vergex-danger max-sm:size-10"
                      aria-label={label('delete')}
                      title={
                        selected.is_active
                          ? label('activeNoDelete')
                          : label('delete')
                      }
                      disabled={selected.is_active}
                      onClick={() => void deleteStrategy()}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </>
                }
              >
                <div className="border-t border-[var(--line)] px-4">
                  <SettingRow label={label('name')} htmlFor="strategy-name">
                    <input
                      id="strategy-name"
                      className={fieldClass}
                      value={name}
                      onChange={(event) => {
                        setName(event.target.value)
                        edited()
                      }}
                    />
                  </SettingRow>
                  <SettingRow
                    label={label('descriptionField')}
                    htmlFor="strategy-description"
                  >
                    <input
                      id="strategy-description"
                      className={fieldClass}
                      value={description}
                      onChange={(event) => {
                        setDescription(event.target.value)
                        edited()
                      }}
                    />
                  </SettingRow>
                </div>

                {isGrid ? (
                  <div className="border-t border-[var(--line)] px-4">
                    {grid ? (
                      <GridSection
                        grid={grid}
                        language={language}
                        onPatch={patchGrid}
                      />
                    ) : (
                      <PanelState role="alert">
                        {label('gridMissing')}
                      </PanelState>
                    )}
                  </div>
                ) : ai ? (
                  <>
                    <SectionTabs
                      tabs={editorTabs}
                      value={tab}
                      onChange={setTab}
                      label={label('tabs')}
                      className="mt-0! mb-0! border-t border-t-[var(--line)] px-4"
                    />
                    <div role="tabpanel" className="px-4">
                      {tab === 'coins' && (
                        <CoinSourceSection
                          key={selected.id}
                          ai={ai}
                          language={language}
                          onPatch={patchCoinSource}
                        />
                      )}
                      {tab === 'risk' && (
                        <RiskProfileSection
                          ai={ai}
                          language={language}
                          onPatchAI={patchAI}
                        />
                      )}
                      {tab === 'market' && (
                        <MarketDataSection
                          ai={ai}
                          language={language}
                          onPatchAI={patchAI}
                        />
                      )}
                      {tab === 'prompt' && (
                        <PromptSection
                          ai={ai}
                          language={language}
                          onPatchAI={patchAI}
                        />
                      )}
                    </div>
                  </>
                ) : (
                  <PanelState role="alert">{label('configMissing')}</PanelState>
                )}

                {/* Pinned so a long form can be saved from anywhere in it. */}
                <div className="sticky bottom-0 z-10 rounded-b-xl bg-[var(--surface)] px-4">
                  <SettingActions
                    message={message}
                    messageTone={problem || pressed ? 'danger' : 'muted'}
                  >
                    {!selected.is_active && (
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        className="max-sm:h-10"
                        disabled={saving || Boolean(problem)}
                        onClick={() => void activateStrategy()}
                      >
                        {label('activate')}
                      </Button>
                    )}
                    <Button
                      type="button"
                      variant="primary"
                      size="sm"
                      className="max-sm:h-10"
                      disabled={saving || !hasChanges || Boolean(problem)}
                      onClick={() => void saveStrategy()}
                    >
                      {saving ? label('saving') : label('save')}
                    </Button>
                  </SettingActions>
                </div>
              </Panel>
            </div>
          )}
        </div>
      )}

      {creating && (
        <NewStrategyDialog
          language={language}
          onCreate={createStrategy}
          onClose={() => setCreating(false)}
        />
      )}
      {testing && config && (
        <StrategyTestDialog
          config={config}
          estimate={estimate}
          language={language}
          onClose={() => setTesting(false)}
        />
      )}
    </Page>
  )
}
