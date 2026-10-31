import { useState } from 'react'
import useSWR from 'swr'
import { Plus } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/api'
import { confirmToast } from '@/components/common/notify'
import { Page } from '@/components/common/page'
import { autopilotTabs } from '@/components/trader/autopilot-tabs'
import { TraderConfigModal } from '@/components/trader/trader-config-modal'
import { TradersList } from '@/components/trader/traders-list'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import {
  configuredExchanges as savedExchanges,
  configuredModels as savedModels,
  useExchangeConfigs,
  useModelConfigs,
} from '@/hooks/use-connections'
import { t } from '@/i18n/translations'
import type { CreateTraderRequest, TraderInfo } from '@/types'

// The Autopilot list. AI models and exchange accounts are set up in settings,
// and each trader is run from its own page.
export function AITradersPage() {
  const { language } = useLanguage()
  const { user, token } = useAuth()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderList.${key}`, language, params)
  const [creating, setCreating] = useState(false)
  const [stopping, setStopping] = useState(false)
  const models = useModelConfigs()
  const exchanges = useExchangeConfigs()
  const allModels = models.data ?? []
  const allExchanges = exchanges.data ?? []
  const configsLoading = models.isLoading || exchanges.isLoading
  const configError = Boolean(models.error || exchanges.error)

  const {
    data: traders,
    error: tradersError,
    mutate: mutateTraders,
  } = useSWR<TraderInfo[]>(
    user && token ? 'traders' : null,
    () => api.getTraders(true),
    { refreshInterval: 5000 }
  )
  const { data: metrics } = useSWR(
    user && token ? 'trader-metrics' : null,
    () => api.getTraderMetrics(true),
    { refreshInterval: 30000, shouldRetryOnError: false }
  )

  const configuredModels = savedModels(allModels)
  const configuredExchanges = savedExchanges(allExchanges)
  const running = (traders ?? []).filter((trader) => trader.is_running)

  const handleCreateTrader = async (data: CreateTraderRequest) => {
    try {
      const model = allModels.find((m) => m.id === data.ai_model_id)
      const exchange = allExchanges.find((e) => e.id === data.exchange_id)
      if (!model?.enabled) {
        toast.error(t('modelNotConfigured', language))
        return
      }
      if (!exchange?.enabled) {
        toast.error(t('exchangeNotConfigured', language))
        return
      }
      await api.createTrader(data)
      toast.success(t('aiTradersToast.created', language))
      setCreating(false)
      await mutateTraders()
    } catch {
      toast.error(t('createTraderFailed', language))
    }
  }

  // Stops only: open positions stay open, as with stopping one trader.
  const stopAll = async () => {
    const targets = running
    const ok = await confirmToast(
      label('stopAllConfirm', { count: targets.length }),
      {
        title: label('stopAll'),
        okText: label('stopAll'),
        cancelText: t('cancel', language),
        tone: 'danger',
      }
    )
    if (!ok) return
    setStopping(true)
    let failed = 0
    for (const trader of targets) {
      try {
        await api.stopTrader(trader.trader_id)
      } catch {
        failed++
      }
    }
    await mutateTraders()
    setStopping(false)
    if (failed === 0) toast.success(label('stopAllDone'))
    else toast.error(label('stopAllFailed', { failed, total: targets.length }))
  }

  return (
    <>
      <Page
        title={t('autopilotNav', language)}
        description={label('description')}
        tabs={autopilotTabs(language)}
        actions={
          <>
            <Button
              variant="danger"
              disabled={running.length === 0 || stopping}
              onClick={() => void stopAll()}
            >
              {label('stopAll')}
            </Button>
            <Button
              variant="primary"
              onClick={() => setCreating(true)}
              disabled={
                configsLoading ||
                !configuredModels.length ||
                !configuredExchanges.length
              }
            >
              <Plus size={14} aria-hidden="true" />
              {t('createTrader', language)}
            </Button>
          </>
        }
      >
        {configError && (
          <p
            role="alert"
            className="mb-4 rounded-xl border border-vergex-danger/30 bg-vergex-danger/10 px-4 py-3 text-sm text-vergex-danger"
          >
            {label('connectionsFailed')}
          </p>
        )}
        <TradersList
          traders={traders}
          metrics={metrics}
          failed={Boolean(tradersError)}
          exchanges={allExchanges}
          modelCount={configuredModels.length}
          exchangeCount={configuredExchanges.length}
          language={language}
        />
      </Page>
      {creating && (
        <TraderConfigModal
          isOpen
          isEditMode={false}
          availableModels={allModels.filter((m) => m.enabled)}
          availableExchanges={allExchanges.filter((e) => e.enabled)}
          onSave={handleCreateTrader}
          onClose={() => setCreating(false)}
        />
      )}
    </>
  )
}
