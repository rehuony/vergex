import { Panel } from '@/components/common/panel'
import { ForecastFrame } from '@/components/forecast/forecast-frame'
import { ForecastSettings } from '@/components/forecast/forecast-settings'
import { useLanguage } from '@/contexts/language-context'

export function ForecastSettingsPage() {
  const { language } = useLanguage()
  return (
    <ForecastFrame language={language}>
      <Panel className="px-4">
        <ForecastSettings language={language} />
      </Panel>
    </ForecastFrame>
  )
}
