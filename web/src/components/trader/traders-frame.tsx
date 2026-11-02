import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/api'
import { confirmToast } from '@/components/common/notify'
import { Page } from '@/components/common/page'
import { Button } from '@/components/ui/button'
import { ROUTES } from '@/constants/route-paths'
import { useAuth } from '@/contexts/auth-context'
import { t, type Language } from '@/i18n'
import { tradersQuery } from './trader-queries'

// Stops only: open positions stay open, as with stopping one trader. It is there only
// while something runs.
function StopAll({ language }: { language: Language }) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderList.${key}`, language, params)
  const { user, token } = useAuth()
  const { data, refetch } = useQuery({
    ...tradersQuery,
    enabled: Boolean(user && token),
  })
  const [stopping, setStopping] = useState(false)
  const running = (data ?? []).filter((trader) => trader.is_running)
  if (running.length === 0) return null

  const stopAll = async () => {
    const ok = await confirmToast(
      label('stopAllConfirm', { count: running.length }),
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
    let ordersLeft = false
    for (const trader of running) {
      try {
        ordersLeft =
          (await api.stopTrader(trader.trader_id)).ordersLeft || ordersLeft
      } catch {
        failed++
      }
    }
    await refetch()
    setStopping(false)
    if (ordersLeft) toast.error(t('agentTradersToast.ordersLeft', language))
    if (failed === 0) toast.success(label('stopAllDone'))
    else toast.error(label('stopAllFailed', { failed, total: running.length }))
  }

  return (
    <Button
      type="button"
      variant="danger"
      className="max-sm:h-10"
      disabled={stopping}
      onClick={() => void stopAll()}
    >
      {label('stopAll')}
    </Button>
  )
}

// What the Auto trading tabs share: the tabs, and stopping everything beside them.
export function TradersFrame({
  language,
  children,
}: {
  language: Language
  children: ReactNode
}) {
  return (
    <Page
      title={t('tradersNav', language)}
      className="vergex-screen animate-page-in"
      tabs={[
        { to: ROUTES.traders, label: t('traderList.tabs', language) },
        {
          to: ROUTES.tradersOverview,
          label: t('traderList.tabOverview', language),
        },
      ]}
      actions={<StopAll language={language} />}
    >
      {children}
    </Page>
  )
}
