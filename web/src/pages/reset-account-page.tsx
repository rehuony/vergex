import { RotateCcw } from 'lucide-react'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import {
  RECOVERY_CLI,
  RecoveryCommandPage,
} from '@/components/auth/recovery-command-page'

// Account wipe was removed from the public API (it was an unauthenticated
// destructive endpoint). It now runs as a local CLI command on the server.
export function ResetAccountPage() {
  const { language } = useLanguage()
  return (
    <RecoveryCommandPage
      icon={RotateCcw}
      tone="danger"
      title={t('resetAccountTitle', language)}
      intro={t('resetAccountCliIntro', language)}
      command={`${RECOVERY_CLI} reset-account`}
      note={t('resetAccountCliWarning', language)}
    />
  )
}
