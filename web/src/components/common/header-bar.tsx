import { useState, useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { Check, ChevronRight, Menu, X } from 'lucide-react'
import { t, type Language } from '@/i18n/translations'
import { LANGUAGE_OPTIONS } from '@/contexts/language-context'
import { THEME_OPTIONS, type ThemePreference } from '@/contexts/theme-context'
import {
  getCurrentPageForPath,
  ROUTES,
  type Page,
} from '@/constants/route-paths'
import { BrandLogo } from './brand-logo'
import { UserAvatar } from './user-avatar'
import { Button, type ButtonProps } from '@/components/ui/button'
import { cn } from '@/utils/cn'

interface HeaderBarProps {
  onLoginClick?: () => void
  isLoggedIn?: boolean
  isHomePage?: boolean
  currentPage?: Page
  language?: Language
  onLanguageChange?: (lang: Language) => void
  themePreference?: ThemePreference
  onThemeChange?: (preference: ThemePreference) => void
  user?: { email: string; displayName?: string; avatar?: string } | null
  onLogout?: () => void
  onPageChange?: (page: Page) => void
  onLoginRequired?: (featureName: string) => void
  onOpenSettings?: () => void
}

// One row of the account menu; the submenus use the same rows.
function MenuRow({ className, ...props }: ButtonProps) {
  return (
    <Button
      type="button"
      variant="ghost"
      className={cn(
        'h-10 w-full justify-start rounded-lg px-2.5 font-normal text-vergex-text hover:bg-vergex-bg',
        className
      )}
      {...props}
    />
  )
}

// A menu row with a list of choices that opens to the left on wide screens
// and inline below the row on phones.
function SubmenuRow<T extends string>({
  id,
  label,
  options,
  current,
  open,
  onOpenChange,
  onSelect,
}: {
  id: string
  label: string
  options: { value: T; label: string; lang?: string }[]
  current: T
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelect: (value: T) => void
}) {
  return (
    <div
      className="relative"
      onMouseEnter={() => onOpenChange(true)}
      onMouseLeave={() => onOpenChange(false)}
    >
      <MenuRow
        aria-expanded={open}
        aria-controls={id}
        onClick={() => onOpenChange(!open)}
      >
        {label}
        <span className="ml-auto flex min-w-0 items-center gap-1 text-xs text-vergex-text-muted">
          <span className="truncate">
            {options.find((o) => o.value === current)?.label}
          </span>
          <ChevronRight className="size-4 shrink-0" aria-hidden="true" />
        </span>
      </MenuRow>
      {open && (
        <div
          id={id}
          className="absolute -top-[7px] right-full pr-1.5 max-sm:static max-sm:pr-0"
        >
          <div
            role="group"
            aria-label={label}
            className="w-48 origin-top-right animate-menu-in rounded-xl border border-[var(--line)] bg-[var(--surface)] p-1.5 shadow-[var(--shadow-lg)] max-sm:w-auto max-sm:animate-none max-sm:border-0 max-sm:p-0 max-sm:pl-3 max-sm:shadow-none"
          >
            {options.map((option) => (
              <MenuRow
                key={option.value}
                lang={option.lang}
                aria-pressed={current === option.value}
                onClick={() => onSelect(option.value)}
              >
                {option.label}
                {current === option.value && (
                  <Check
                    className="ml-auto size-4 text-vergex-brand"
                    aria-hidden="true"
                  />
                )}
              </MenuRow>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

export function HeaderBar({
  onLoginClick,
  isLoggedIn = false,
  currentPage,
  language = 'en',
  onLanguageChange,
  themePreference = 'system',
  onThemeChange,
  user,
  onLogout,
  onPageChange,
  onLoginRequired,
  onOpenSettings,
}: HeaderBarProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const [menuOpen, setMenuOpen] = useState(false)
  const [userOpen, setUserOpen] = useState(false)
  const [submenu, setSubmenu] = useState<'theme' | 'language' | null>(null)
  const toggleSubmenu = (name: 'theme' | 'language') => (open: boolean) =>
    setSubmenu((current) => (open ? name : current === name ? null : current))
  const userRef = useRef<HTMLDivElement>(null)
  const resolvedPage = currentPage ?? getCurrentPageForPath(location.pathname)
  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (userRef.current && !userRef.current.contains(event.target as Node))
        setUserOpen(false)
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setUserOpen(false)
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', escape)
    }
  }, [])
  useEffect(() => {
    setMenuOpen(false)
    setUserOpen(false)
  }, [location.pathname])
  useEffect(() => {
    if (!userOpen) setSubmenu(null)
  }, [userOpen])
  const tabs: { page: Page; path: string; label: string; auth: boolean }[] = [
    {
      page: 'dashboard',
      path: ROUTES.dashboard,
      label: t('dashboardNav', language),
      auth: true,
    },
    {
      page: 'forecast',
      path: ROUTES.forecast,
      label: t('forecastNav', language),
      auth: true,
    },
    { page: 'planning', path: ROUTES.planning, label: 'Frontier', auth: true },
    {
      page: 'traders',
      path: ROUTES.traders,
      label: t('tradersNav', language),
      auth: true,
    },
    {
      page: 'strategies',
      path: ROUTES.strategies,
      label: t('strategiesNav', language),
      auth: true,
    },
  ]
  const open = (tab: (typeof tabs)[number]) => {
    if (tab.auth && !isLoggedIn) {
      onLoginRequired?.(tab.label)
      return
    }
    onPageChange?.(tab.page)
    navigate(tab.path)
  }
  return (
    <nav className="header-bar vergex-navigation" aria-label="Main navigation">
      <div className="vergex-navigation-inner">
        <button
          className="vergex-home"
          onClick={() => navigate(ROUTES.home)}
          aria-label="Vergex home"
        >
          <BrandLogo />
        </button>
        <div
          className={`vergex-nav-links lg:flex ${menuOpen ? 'is-open' : ''}`}
        >
          {tabs.map((tab) => (
            <button
              key={tab.page}
              onClick={() => open(tab)}
              aria-current={resolvedPage === tab.page ? 'page' : undefined}
              className="vergex-nav-link whitespace-nowrap"
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="vergex-navigation-actions">
          {/* On narrow screens the menu button sits left of the avatar, which
              keeps the far right like on desktop. */}
          <button
            className="vergex-nav-toggle lg:hidden"
            aria-label={menuOpen ? 'Close navigation' : 'Open navigation'}
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen(!menuOpen)}
          >
            {menuOpen ? <X size={20} /> : <Menu size={20} />}
          </button>
          {isLoggedIn && user ? (
            <div className="vergex-user" ref={userRef}>
              <Button
                variant="ghost"
                size="icon"
                className="size-10 rounded-full hover:bg-[var(--brand-soft)] aria-expanded:bg-[var(--brand-soft)]"
                aria-label={`Account menu for ${user.email}`}
                aria-expanded={userOpen}
                aria-controls="vergex-user-menu"
                onClick={() => setUserOpen(!userOpen)}
              >
                <UserAvatar
                  email={user.email}
                  avatar={user.avatar}
                  className="size-9 text-sm"
                />
              </Button>
              {userOpen && (
                <div
                  id="vergex-user-menu"
                  className="absolute right-0 top-[calc(100%+4px)] z-50 w-72 origin-top-right animate-menu-in rounded-xl border border-[var(--line)] bg-[var(--surface)] p-1.5 shadow-[var(--shadow-lg)]"
                >
                  <div className="flex items-center gap-3 px-2.5 py-2.5">
                    <UserAvatar
                      email={user.email}
                      avatar={user.avatar}
                      className="size-10 text-base"
                    />
                    <div className="min-w-0 flex-1">
                      {user.displayName && (
                        <p className="truncate text-sm font-medium text-vergex-text">
                          {user.displayName}
                        </p>
                      )}
                      <p
                        className={
                          user.displayName
                            ? 'truncate text-xs text-vergex-text-muted'
                            : 'truncate text-sm font-medium text-vergex-text'
                        }
                        title={user.email}
                      >
                        {user.email}
                      </p>
                    </div>
                  </div>
                  <div className="my-1 h-px bg-[var(--line)]" />
                  {onThemeChange && (
                    <SubmenuRow
                      id="vergex-theme-menu"
                      label={t('appearance', language)}
                      options={THEME_OPTIONS.map((value) => ({
                        value,
                        label: t(
                          `theme${value[0].toUpperCase()}${value.slice(1)}`,
                          language
                        ),
                      }))}
                      current={themePreference}
                      open={submenu === 'theme'}
                      onOpenChange={toggleSubmenu('theme')}
                      onSelect={onThemeChange}
                    />
                  )}
                  {onLanguageChange && (
                    <SubmenuRow
                      id="vergex-language-menu"
                      label={t('language', language)}
                      options={LANGUAGE_OPTIONS.map((option) => ({
                        value: option.code,
                        label: option.label,
                        lang: option.code,
                      }))}
                      current={language}
                      open={submenu === 'language'}
                      onOpenChange={toggleSubmenu('language')}
                      onSelect={onLanguageChange}
                    />
                  )}
                  {onOpenSettings && (
                    <MenuRow
                      onClick={() => {
                        onOpenSettings()
                        setUserOpen(false)
                      }}
                    >
                      {t('settingsNav', language)}
                    </MenuRow>
                  )}
                  <MenuRow
                    onClick={() => {
                      navigate(ROUTES.help)
                      setUserOpen(false)
                    }}
                  >
                    {t('helpNav', language)}
                  </MenuRow>
                  {onLogout && (
                    <>
                      <div className="my-1 h-px bg-[var(--line)]" />
                      <MenuRow
                        className="text-vergex-danger hover:bg-vergex-danger/10 hover:text-vergex-danger"
                        onClick={() => {
                          onLogout()
                          setUserOpen(false)
                        }}
                      >
                        {t('exitLogin', language)}
                      </MenuRow>
                    </>
                  )}
                </div>
              )}
            </div>
          ) : (
            resolvedPage !== 'login' &&
            resolvedPage !== 'register' && (
              <button
                className="vergex-sign-in"
                onClick={() =>
                  onLoginClick ? onLoginClick() : navigate(ROUTES.login)
                }
              >
                {t('signIn', language)}
              </button>
            )
          )}
        </div>
      </div>
    </nav>
  )
}
