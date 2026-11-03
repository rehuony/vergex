import { ForecastFrame } from '@/components/forecast/forecast-frame'
import { ForecastSettings } from '@/components/forecast/forecast-settings'
import { useAssistantPage } from '@/hooks/use-assistant-page'
import { useLanguage } from '@/contexts/language-context'

export function ForecastSettingsPage() {
  const { language } = useLanguage()
  useAssistantPage('forecast_settings')
  return (
    <ForecastFrame language={language}>
      <ForecastSettings language={language} />
    </ForecastFrame>
  )
}
