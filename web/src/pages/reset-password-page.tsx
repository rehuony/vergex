import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import {
  RECOVERY_CLI,
  RecoveryCommandPage,
} from '@/components/auth/recovery-command-page'

export function ResetPasswordPage() {
  const { language } = useLanguage()
  return (
    <RecoveryCommandPage
      tone="neutral"
      title={t('resetPasswordTitle', language)}
      intro={t('resetPasswordCliIntro', language)}
      command={`${RECOVERY_CLI} reset-password --email you@example.com`}
      note={t('resetPasswordCliSecurityNote', language)}
    />
  )
}
