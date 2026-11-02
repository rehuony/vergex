import { useCallback, useRef, useState, useSyncExternalStore } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import type { ActivityEvent, ActivityStream } from '@/api/dashboard'
import { AppChrome } from '@/components/common/app-chrome'
import { Page } from '@/components/common/page'
import { Panel, PanelLoading, PanelState } from '@/components/common/panel'
import { AttentionList } from '@/components/dashboard/attention-list'
import { FirstRun } from '@/components/dashboard/first-run'
import { FlowEvents, type FlowKind } from '@/components/dashboard/flow-events'
import { FlowGroups } from '@/components/dashboard/flow-groups'
import type { LineKey } from '@/components/dashboard/flow-layout'
import {
  FlowScene,
  type FlowSceneHandle,
} from '@/components/dashboard/flow-scene'
import type { FlowSelection } from '@/components/dashboard/flow-text'
import { OverviewFigures } from '@/components/dashboard/overview-figures'
import { PositionsOverview } from '@/components/dashboard/positions-overview'
import { useActivity } from '@/components/dashboard/use-activity'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { pollEvery } from '@/utils/poll'

// While it loads the page already holds the room the scene will take.
const FIRST_SCREEN =
  'h-[clamp(480px,calc(56.25vw-28px),calc(100dvh-190px))] xl:h-[max(620px,calc(100dvh-121px))]'

// A phone gets the list, and never loads what draws the stage.
const WIDE_ENOUGH = '(min-width: 768px)'
const watchWidth = (changed: () => void) => {
  const query = window.matchMedia(WIDE_ENOUGH)
  query.addEventListener('change', changed)
  return () => query.removeEventListener('change', changed)
}
const wideEnough = () => window.matchMedia(WIDE_ENOUGH).matches

function clock(at: number) {
  return new Date(at).toLocaleTimeString('en-GB', { hour12: false })
}

// The home page: a read-only view of the whole system, built around where things
// are going right now. Everything that changes something lives on the page a row
// leads to.
export function DashboardPage() {
  const { user, token } = useAuth()
  const { language } = useLanguage()
  const label = useCallback(
    (key: string, params?: Record<string, string | number>) =>
      t(`dashboard.${key}`, language, params),
    [language]
  )
  const signedIn = Boolean(user && token)
  const { data: overview, error } = useQuery({
    queryKey: ['overview'],
    queryFn: () => api.getOverview(true),
    enabled: signedIn,
    ...pollEvery(20000),
  })
  const scene = useRef<FlowSceneHandle>(null)
  const play = useCallback((events: ActivityEvent[]) => {
    scene.current?.play(events)
  }, [])
  const pulse = useCallback((flow: ActivityStream) => {
    scene.current?.pulse(flow)
  }, [])
  const activity = useActivity(signedIn, play, pulse)
  const [kind, setKind] = useState<FlowKind>('all')
  const [selection, setSelection] = useState<FlowSelection | null>(null)
  const [pointed, setPointed] = useState<LineKey | null>(null)
  // Set when this browser cannot draw the stage. The list says the same things.
  const [flat, setFlat] = useState(false)
  const stageUnavailable = useCallback(() => setFlat(true), [])
  const wide = useSyncExternalStore(watchWidth, wideEnough)

  const staleAt = activity.stale
    ? activity.at
    : error && overview
      ? new Date(overview.generated_at).getTime()
      : 0
  const staleText = staleAt ? label('stale', { time: clock(staleAt) }) : ''

  return (
    <AppChrome currentPage="dashboard" animateContent>
      <Page className="vergex-screen" title={t('dashboardNav', language)}>
        {!overview ? (
          <Panel className={FIRST_SCREEN}>
            {error ? (
              <PanelState role="alert" className="h-full">
                {label('loadFailed')}
              </PanelState>
            ) : (
              <PanelLoading className="h-full" />
            )}
          </Panel>
        ) : overview.models.length === 0 || overview.accounts.length === 0 ? (
          <FirstRun overview={overview} label={label} />
        ) : (
          <div className="grid min-w-0 gap-3 xl:h-[max(620px,calc(100dvh-121px))] xl:grid-cols-[minmax(0,1fr)_380px]">
            <Panel className="flex min-h-0 flex-col overflow-hidden">
              <OverviewFigures
                overview={overview}
                attention={
                  <AttentionList overview={overview} language={language} />
                }
                label={label}
              />
              {wide && !flat ? (
                <div className="min-h-0 flex-1">
                  <FlowScene
                    ref={scene}
                    overview={overview}
                    totals={activity.totals}
                    stream={activity.stream}
                    kind={kind === 'positions' ? 'all' : kind}
                    selection={selection}
                    onSelect={setSelection}
                    pointed={pointed}
                    staleText={staleText}
                    onUnavailable={stageUnavailable}
                    label={label}
                  />
                </div>
              ) : (
                <FlowGroups
                  overview={overview}
                  totals={activity.totals}
                  stream={activity.stream}
                  selection={selection}
                  onSelect={setSelection}
                  staleText={staleText}
                  label={label}
                />
              )}
            </Panel>
            <Panel className="h-[440px] min-h-0 overflow-hidden xl:h-auto">
              <FlowEvents
                overview={overview}
                events={activity.events}
                loaded={activity.loaded}
                stale={activity.stale}
                kind={kind}
                onKind={setKind}
                selection={selection}
                onPoint={setPointed}
                positions={
                  <PositionsOverview overview={overview} language={language} />
                }
                label={label}
              />
            </Panel>
          </div>
        )}
      </Page>
    </AppChrome>
  )
}
