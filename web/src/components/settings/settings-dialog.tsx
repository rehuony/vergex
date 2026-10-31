import {
  forwardRef,
  useEffect,
  useId,
  useRef,
  useState,
  type ComponentProps,
  type FormEvent,
} from 'react'
import { toast } from 'sonner'
import {
  Brain,
  Eye,
  EyeOff,
  Landmark,
  Receipt,
  Send,
  UserRound,
} from 'lucide-react'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import { t, type Language } from '@/i18n/translations'
import { api } from '@/api'
import { cn } from '@/utils/cn'
import { ApiError } from '@/api/http-client'
import { displayWidth } from '@/utils/text'
import { useProfile } from '@/hooks/use-profile'
import { Dialog, dialogInputClass } from '@/components/common/dialog'
import { AICostPanel } from './ai-cost-panel'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AvatarEditor } from './avatar-editor'
import { ExchangeSettings } from './exchange-settings'
import { ModelSettings } from './model-settings'
import { SettingActions, SettingRow } from '@/components/common/setting-row'
import { TelegramSettings } from './telegram-settings'

// Listed shortest label first (ties keep this order), so the column of tabs
// steps out evenly in every language.
const SECTIONS = [
  { id: 'account', label: 'accountTab', icon: UserRound },
  { id: 'models', label: 'modelsTab', icon: Brain },
  { id: 'exchanges', label: 'exchangesTab', icon: Landmark },
  { id: 'costs', label: 'aiCostsTab', icon: Receipt },
  { id: 'telegram', label: 'telegramTab', icon: Send },
] as const

export type SettingsSection = (typeof SECTIONS)[number]['id']

export function isSettingsSection(value: string): value is SettingsSection {
  return SECTIONS.some((section) => section.id === value)
}

const PasswordInput = forwardRef<
  HTMLInputElement,
  Omit<ComponentProps<'input'>, 'type'> & { language: Language }
>(function PasswordInput({ language, ...props }, ref) {
  const [visible, setVisible] = useState(false)
  return (
    <div className="relative w-full">
      <input
        {...props}
        ref={ref}
        type={visible ? 'text' : 'password'}
        className={cn(
          dialogInputClass,
          'h-9 pr-10 aria-[invalid=true]:border-vergex-danger'
        )}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute right-1 top-1/2 size-7 -translate-y-1/2 text-vergex-text-muted"
        onClick={() => setVisible(!visible)}
        aria-label={t(visible ? 'hidePassword' : 'showPassword', language)}
        aria-pressed={visible}
      >
        {visible ? <EyeOff size={15} /> : <Eye size={15} />}
      </Button>
    </div>
  )
})

// A change-password form as password managers expect it: the account name in
// a hidden username field, then the current and the new password with their
// autocomplete roles, each with a real label. Problems show in the action bar,
// not under the fields.
function PasswordSettings({
  language,
  email,
}: {
  language: Language
  email: string
}) {
  const id = useId()
  const messageId = `${id}-message`
  const currentRef = useRef<HTMLInputElement>(null)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [wrongCurrent, setWrongCurrent] = useState(false)
  const [saving, setSaving] = useState(false)
  // Typing the start of the new password is not a mismatch yet.
  const mismatch = confirm !== next && !next.startsWith(confirm)
  const ready = current !== '' && next.length >= 8 && confirm === next
  const message = mismatch
    ? t('passwordMismatch', language)
    : wrongCurrent
      ? t('currentPasswordIncorrect', language)
      : null

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!ready) return
    setSaving(true)
    try {
      await api.changePassword(current, next)
      toast.success(t('passwordUpdated', language))
      setCurrent('')
      setNext('')
      setConfirm('')
    } catch (err) {
      if (
        err instanceof ApiError &&
        err.errorKey === 'current_password_incorrect'
      ) {
        setWrongCurrent(true)
        currentRef.current?.select()
      } else {
        toast.error(
          err instanceof Error
            ? err.message
            : t('passwordUpdateFailed', language)
        )
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={(e) => void submit(e)} aria-label={t('password', language)}>
      <input
        type="email"
        name="username"
        autoComplete="username"
        value={email}
        readOnly
        hidden
      />
      <SettingRow
        label={t('currentPassword', language)}
        htmlFor={`${id}-current`}
      >
        <PasswordInput
          ref={currentRef}
          id={`${id}-current`}
          name="current-password"
          autoComplete="current-password"
          required
          value={current}
          onChange={(e) => {
            setCurrent(e.target.value)
            setWrongCurrent(false)
          }}
          aria-invalid={wrongCurrent}
          aria-describedby={wrongCurrent ? messageId : undefined}
          language={language}
        />
      </SettingRow>
      <SettingRow label={t('newPassword', language)} htmlFor={`${id}-new`}>
        <PasswordInput
          id={`${id}-new`}
          name="new-password"
          autoComplete="new-password"
          required
          minLength={8}
          value={next}
          onChange={(e) => setNext(e.target.value)}
          placeholder={t('passwordMin8', language)}
          language={language}
        />
      </SettingRow>
      <SettingRow
        label={t('confirmNewPassword', language)}
        htmlFor={`${id}-confirm`}
      >
        <PasswordInput
          id={`${id}-confirm`}
          name="confirm-password"
          autoComplete="new-password"
          required
          minLength={8}
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          aria-invalid={mismatch}
          aria-describedby={mismatch ? messageId : undefined}
          language={language}
        />
      </SettingRow>
      <SettingActions message={message} messageId={messageId}>
        <Button
          type="submit"
          variant="primary"
          size="sm"
          disabled={!ready || saving}
        >
          {saving
            ? t('updatingPassword', language)
            : t('updatePassword', language)}
        </Button>
      </SettingActions>
    </form>
  )
}

// The name in the account menu. It saves itself on Enter or when the field
// loses focus; an emptied field goes back to the saved name.
function DisplayNameSettings({ language }: { language: Language }) {
  const id = useId()
  const { data: profile, mutate } = useProfile()
  const saved = profile?.display_name ?? ''
  const [name, setName] = useState<string | null>(null)
  const saving = useRef(false)
  const value = name ?? saved

  const save = async () => {
    const next = value.trim()
    if (next === '' || next === saved) {
      setName(null)
      return
    }
    if (saving.current) return
    saving.current = true
    try {
      const stored = await api.updateDisplayName(next)
      await mutate(profile && { ...profile, display_name: stored }, {
        revalidate: false,
      })
      setName(null)
      toast.success(t('displayNameSaved', language))
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('saveConfigFailed', language)
      )
    } finally {
      saving.current = false
    }
  }

  return (
    <SettingRow label={t('displayName', language)} htmlFor={id}>
      <input
        id={id}
        name="nickname"
        autoComplete="nickname"
        maxLength={32}
        value={value}
        onChange={(e) => setName(e.target.value)}
        onBlur={() => void save()}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            e.currentTarget.blur()
          }
        }}
        className={cn(dialogInputClass, 'h-9')}
      />
    </SettingRow>
  )
}

function AccountSettings({ language }: { language: Language }) {
  const { user } = useAuth()
  return (
    <>
      <SettingRow label={t('avatar', language)}>
        <AvatarEditor language={language} />
      </SettingRow>
      <DisplayNameSettings language={language} />
      <SettingRow label={t('email', language)}>
        <p className="truncate text-sm text-vergex-text">{user?.email}</p>
      </SettingRow>
      <PasswordSettings language={language} email={user?.email ?? ''} />
    </>
  )
}

/** Account, AI model, exchange, AI cost and Telegram settings. */
export function SettingsDialog({
  section,
  onSectionChange,
  onClose,
}: {
  section: SettingsSection
  onSectionChange: (section: SettingsSection) => void
  onClose: () => void
}) {
  const { language } = useLanguage()
  const tabs = useRef<HTMLDivElement>(null)
  // Keep the open tab in view where the tabs scroll sideways.
  useEffect(() => {
    tabs.current
      ?.querySelector('[aria-selected="true"]')
      ?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  }, [section])
  const sections = [...SECTIONS].sort(
    (a, b) =>
      displayWidth(t(a.label, language)) - displayWidth(t(b.label, language))
  )
  return (
    <Dialog
      title={t('settingsNav', language)}
      size="xl"
      onClose={onClose}
      // A fixed height keeps the dialog from jumping when sections switch.
      bodyClassName="flex h-[min(560px,calc(100dvh-9rem))] min-h-0 flex-col sm:flex-row"
    >
      <div
        ref={tabs}
        role="tablist"
        aria-label={t('settingsNav', language)}
        // On a phone the tabs form one row that scrolls sideways.
        className="flex shrink-0 gap-1 overflow-x-auto border-b border-[var(--line)] p-2 [scrollbar-width:none] sm:w-44 sm:flex-col sm:overflow-visible sm:border-b-0 sm:border-r sm:p-3"
      >
        {sections.map(({ id, label, icon: Icon }) => (
          <Button
            key={id}
            type="button"
            role="tab"
            variant="ghost"
            size="sm"
            aria-selected={section === id}
            className="justify-start gap-2 px-2.5 font-normal aria-selected:bg-[var(--brand-soft)] aria-selected:font-medium aria-selected:text-vergex-brand"
            onClick={() => onSectionChange(id)}
          >
            <Icon className="size-4" aria-hidden="true" />
            {t(label, language)}
          </Button>
        ))}
      </div>
      <ScrollArea role="tabpanel" className="min-h-0 flex-1 px-6 py-2">
        {section === 'account' && <AccountSettings language={language} />}
        {section === 'models' && <ModelSettings language={language} />}
        {section === 'exchanges' && <ExchangeSettings language={language} />}
        {section === 'telegram' && <TelegramSettings language={language} />}
        {section === 'costs' && (
          <div className="flex min-h-full flex-col py-4">
            <AICostPanel />
          </div>
        )}
      </ScrollArea>
    </Dialog>
  )
}
