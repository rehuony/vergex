import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import {
  RECOVERY_CLI,
  RecoveryCommandPage,
} from '@/components/auth/recovery-command-page'

// User wipe was removed from the public API (it was an unauthenticated
// destructive endpoint). It now runs as a local CLI command on the server.
export function ResetUserPage() {
  const { language } = useLanguage()
  return (
    <RecoveryCommandPage
      tone="danger"
      title={t('resetUserTitle', language)}
      intro={t('resetUserCliIntro', language)}
      command={`${RECOVERY_CLI} reset-user`}
      note={t('resetUserCliWarning', language)}
    />
  )
}
