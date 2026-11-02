import { ForecastFrame } from '@/components/forecast/forecast-frame'
import { ForecastSettings } from '@/components/forecast/forecast-settings'
import { useLanguage } from '@/contexts/language-context'

export function ForecastSettingsPage() {
  const { language } = useLanguage()
  return (
    <ForecastFrame language={language}>
      <ForecastSettings language={language} />
    </ForecastFrame>
  )
}
