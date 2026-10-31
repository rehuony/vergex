import { useState, useEffect } from 'react'
import type {
  AIModel,
  Exchange,
  CreateTraderRequest,
  Strategy,
  TraderConfigData,
} from '@/types'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import { coinSourceLabel } from '@/utils/coin-sources'
import { Sparkles } from 'lucide-react'
import { Dialog } from '@/components/common/dialog'
import { httpClient } from '@/api/http-client'
import { AppSelect } from '@/components/ui/select'
import { Button } from '@/components/ui/button'

// Extract the name part after the underscore
function getShortName(fullName: string): string {
  const parts = fullName.split('_')
  return parts.length > 1 ? parts[parts.length - 1] : fullName
}

function getStrategyAIConfig(strategy: Strategy) {
  return (
    strategy.config.ai_config ||
    (strategy.config.coin_source && strategy.config.risk_control
      ? {
          coin_source: strategy.config.coin_source,
          risk_control: strategy.config.risk_control,
        }
      : null)
  )
}

// Internal form state type
interface FormState {
  trader_id?: string
  trader_name: string
  ai_model: string
  exchange_id: string
  strategy_id: string
  is_cross_margin: boolean
  scan_interval_minutes: number
}

interface TraderConfigModalProps {
  isOpen: boolean
  onClose: () => void
  traderData?: TraderConfigData | null
  isEditMode?: boolean
  availableModels?: AIModel[]
  availableExchanges?: Exchange[]
  onSave?: (data: CreateTraderRequest) => Promise<void>
}

export function TraderConfigModal({
  isOpen,
  onClose,
  traderData,
  isEditMode = false,
  availableModels = [],
  availableExchanges = [],
  onSave,
}: TraderConfigModalProps) {
  const { language } = useLanguage()
  const [formData, setFormData] = useState<FormState>({
    trader_name: '',
    ai_model: '',
    exchange_id: '',
    strategy_id: '',
    is_cross_margin: true,
    scan_interval_minutes: 15,
  })
  const [isSaving, setIsSaving] = useState(false)
  const [strategies, setStrategies] = useState<Strategy[]>([])

  // Fetch the user's strategy list
  useEffect(() => {
    const fetchStrategies = async () => {
      try {
        const result = await httpClient.get<{ strategies: Strategy[] }>(
          '/api/strategies'
        )
        if (result.success && result.data?.strategies) {
          const strategyList = result.data.strategies
          setStrategies(strategyList)
          // If no strategy is selected, default to the active strategy
          if (!formData.strategy_id && !isEditMode) {
            const activeStrategy = strategyList.find((s) => s.is_active)
            if (activeStrategy) {
              setFormData((prev) => ({
                ...prev,
                strategy_id: activeStrategy.id,
              }))
            } else if (strategyList.length > 0) {
              setFormData((prev) => ({
                ...prev,
                strategy_id: strategyList[0].id,
              }))
            }
          }
        }
      } catch (error) {
        console.error('Failed to fetch strategies:', error)
      }
    }
    if (isOpen) {
      fetchStrategies()
    }
  }, [isOpen])

  useEffect(() => {
    if (traderData) {
      setFormData({
        ...traderData,
        strategy_id: traderData.strategy_id || '',
      })
    } else if (!isEditMode) {
      setFormData({
        trader_name: '',
        ai_model: availableModels[0]?.id || '',
        exchange_id: availableExchanges[0]?.id || '',
        strategy_id: '',
        is_cross_margin: true,
        scan_interval_minutes: 15,
      })
    }
  }, [traderData, isEditMode, availableModels, availableExchanges])

  if (!isOpen) return null

  const handleInputChange = (field: keyof FormState, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }))
  }

  const handleExchangeChange = (exchangeId: string) => {
    setFormData((prev) => ({ ...prev, exchange_id: exchangeId }))
  }

  const handleSave = async () => {
    if (!onSave) return

    setIsSaving(true)
    try {
      const saveData: CreateTraderRequest = {
        name: formData.trader_name,
        ai_model_id: formData.ai_model,
        exchange_id: formData.exchange_id,
        strategy_id: formData.strategy_id,
        is_cross_margin: formData.is_cross_margin,
        scan_interval_minutes: formData.scan_interval_minutes,
      }

      await onSave(saveData)
    } catch (error) {
      console.error(t('saveFailed', language) + ':', error)
    } finally {
      setIsSaving(false)
    }
  }

  const selectedStrategy = strategies.find((s) => s.id === formData.strategy_id)

  return (
    <Dialog
      title={
        isEditMode ? t('editTrader', language) : t('createTrader', language)
      }
      description={
        isEditMode
          ? t('editTraderConfig', language)
          : t('selectStrategyAndConfigParams', language)
      }
      onClose={onClose}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('cancel', language)}
          </Button>
          {onSave && (
            <Button
              variant="primary"
              onClick={handleSave}
              disabled={
                isSaving ||
                !formData.trader_name ||
                !formData.ai_model ||
                !formData.exchange_id
              }
            >
              {isSaving
                ? t('saving', language)
                : isEditMode
                  ? t('editTrader', language)
                  : t('createTraderButton', language)}
            </Button>
          )}
        </>
      }
    >
      <div className="space-y-5">
        {/* Basic Info */}
        <div className="rounded-xl border border-[var(--line)] p-5">
          <h3 className="text-lg font-semibold text-vergex-text mb-5 flex items-center gap-2">
            <span className="text-vergex-brand">1</span>{' '}
            {t('basicConfig', language)}
          </h3>
          <div className="space-y-4">
            <div>
              <label className="text-sm text-vergex-text block mb-2">
                {t('traderNameRequired', language)}
              </label>
              <input
                type="text"
                value={formData.trader_name}
                onChange={(e) =>
                  handleInputChange('trader_name', e.target.value)
                }
                className="w-full px-3 py-2 bg-vergex-bg-lighter border border-[var(--line)] rounded-sm text-vergex-text focus:border-vergex-brand focus:outline-hidden"
                placeholder={t('enterTraderNamePlaceholder', language)}
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-sm text-vergex-text block mb-2">
                  {t('aiModelRequired', language)}
                </label>
                <AppSelect
                  value={formData.ai_model}
                  onChange={(val) => handleInputChange('ai_model', val)}
                  className="w-full px-3 py-2 bg-vergex-bg-lighter border border-[var(--line)] rounded-sm text-vergex-text"
                  options={availableModels.map((model) => ({
                    value: model.id,
                    label: getShortName(model.name || model.id).toUpperCase(),
                  }))}
                />
              </div>
              <div>
                <label className="text-sm text-vergex-text block mb-2">
                  {t('exchangeRequired', language)}
                </label>
                <AppSelect
                  value={formData.exchange_id}
                  onChange={handleExchangeChange}
                  className="w-full px-3 py-2 bg-vergex-bg-lighter border border-[var(--line)] rounded-sm text-vergex-text"
                  options={availableExchanges.map((exchange) => ({
                    value: exchange.id,
                    label:
                      getShortName(
                        exchange.name || exchange.exchange_type || exchange.id
                      ).toUpperCase() +
                      (exchange.account_name
                        ? ` - ${exchange.account_name}`
                        : ''),
                  }))}
                />
              </div>
            </div>
          </div>
        </div>

        {/* Strategy Selection */}
        <div className="rounded-xl border border-[var(--line)] p-5">
          <h3 className="text-lg font-semibold text-vergex-text mb-5 flex items-center gap-2">
            <span className="text-vergex-brand">2</span>{' '}
            {t('selectTradingStrategy', language)}
            <Sparkles className="w-4 h-4 text-vergex-brand" />
          </h3>
          <div className="space-y-4">
            <div>
              <label className="text-sm text-vergex-text block mb-2">
                {t('useStrategy', language)}
              </label>
              <AppSelect
                value={formData.strategy_id}
                onChange={(val) => handleInputChange('strategy_id', val)}
                className="w-full px-3 py-2 bg-vergex-bg-lighter border border-[var(--line)] rounded-sm text-vergex-text"
                options={[
                  { value: '', label: t('noStrategyManual', language) },
                  ...strategies.map((strategy) => ({
                    value: strategy.id,
                    label:
                      strategy.name +
                      (strategy.is_active
                        ? t('strategyActive', language)
                        : '') +
                      (strategy.is_default
                        ? t('strategyDefault', language)
                        : ''),
                  })),
                ]}
              />
              {strategies.length === 0 && (
                <p className="text-xs text-vergex-text-muted mt-2">
                  {t('noStrategyHint', language)}
                </p>
              )}
            </div>

            {/* Strategy Preview */}
            {selectedStrategy && (
              <div className="mt-3 p-4 bg-vergex-bg-lighter border border-[var(--line)] rounded-lg">
                <div className="flex items-center gap-2 mb-2">
                  <span className="text-vergex-brand text-sm font-medium">
                    {t('strategyDetails', language)}
                  </span>
                  {selectedStrategy.is_active && (
                    <span className="px-2 py-0.5 bg-vergex-success/20 text-vergex-success text-xs rounded-sm">
                      {t('strategyIsDefault', language)}
                    </span>
                  )}
                </div>
                <p className="text-sm text-vergex-text-muted mb-2">
                  {selectedStrategy.description ||
                    (language === 'zh' ? 'No description' : 'No description')}
                </p>
                {selectedStrategy.config.strategy_type === 'grid_trading' &&
                selectedStrategy.config.grid_config ? (
                  <div className="grid grid-cols-2 gap-2 text-xs text-vergex-text-muted">
                    <div>
                      {language === 'zh' ? 'Symbol' : 'Symbol'}:{' '}
                      {selectedStrategy.config.grid_config.symbol || '-'}
                    </div>
                    <div>
                      {language === 'zh' ? 'Grids' : 'Grids'}:{' '}
                      {selectedStrategy.config.grid_config.grid_count}
                    </div>
                  </div>
                ) : (
                  (() => {
                    const aiConfig = getStrategyAIConfig(selectedStrategy)
                    if (!aiConfig) return null
                    return (
                      <div className="grid grid-cols-2 gap-2 text-xs text-vergex-text-muted">
                        <div>
                          {t('coinSource', language)}:{' '}
                          {coinSourceLabel(
                            aiConfig.coin_source.source_type,
                            language
                          )}
                        </div>
                        <div>
                          {t('marginLimit', language)}:{' '}
                          {(
                            (aiConfig.risk_control?.max_margin_usage || 0.9) *
                            100
                          ).toFixed(0)}
                          %
                        </div>
                      </div>
                    )
                  })()
                )}
              </div>
            )}
          </div>
        </div>

        {/* Trading Parameters */}
        <div className="rounded-xl border border-[var(--line)] p-5">
          <h3 className="text-lg font-semibold text-vergex-text mb-5 flex items-center gap-2">
            <span className="text-vergex-brand">3</span>{' '}
            {t('tradingParams', language)}
          </h3>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-sm text-vergex-text block mb-2">
                  {t('marginMode', language)}
                </label>
                <div className="flex gap-2">
                  <Button
                    variant="option"
                    className="flex-1"
                    aria-pressed={formData.is_cross_margin}
                    type="button"
                    onClick={() => handleInputChange('is_cross_margin', true)}
                  >
                    {t('crossMargin', language)}
                  </Button>
                  <Button
                    variant="option"
                    className="flex-1"
                    aria-pressed={!formData.is_cross_margin}
                    type="button"
                    onClick={() => handleInputChange('is_cross_margin', false)}
                  >
                    {t('isolatedMargin', language)}
                  </Button>
                </div>
              </div>
              <div>
                <label className="text-sm text-vergex-text block mb-2">
                  {t('aiScanInterval', language)}
                </label>
                <input
                  type="number"
                  value={formData.scan_interval_minutes}
                  onChange={(e) => {
                    const parsedValue = Number(e.target.value)
                    const safeValue = Number.isFinite(parsedValue)
                      ? Math.max(3, parsedValue)
                      : 3
                    handleInputChange('scan_interval_minutes', safeValue)
                  }}
                  className="w-full px-3 py-2 bg-vergex-bg-lighter border border-[var(--line)] rounded-sm text-vergex-text focus:border-vergex-brand focus:outline-hidden"
                  min="3"
                  max="60"
                  step="1"
                />
                <p className="text-xs text-vergex-text-muted mt-1">
                  {t('scanIntervalRecommend', language)}
                </p>
              </div>
            </div>

            <div className="p-3 bg-vergex-bg-lighter border border-[var(--line)] rounded-sm flex items-center gap-2">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                className="w-4 h-4 text-vergex-brand"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <circle cx="12" cy="12" r="10" />
                <line x1="12" x2="12" y1="8" y2="12" />
                <line x1="12" x2="12.01" y1="16" y2="16" />
              </svg>
              <span className="text-sm text-vergex-text-muted">
                {t('autoFetchBalanceInfo', language)}
              </span>
            </div>
          </div>
        </div>
      </div>
    </Dialog>
  )
}
