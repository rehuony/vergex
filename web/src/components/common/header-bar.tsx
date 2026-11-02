import { useState, useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { Check, ChevronRight } from 'lucide-react'
import { LANGUAGES, t, type Language } from '@/i18n'
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
import { lockPageScroll } from '@/utils/page-scroll'

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
        'h-10 w-full justify-start rounded-md px-2.5 font-normal text-vergex-ink hover:bg-vergex-surface-muted',
        className
      )}
      {...props}
    />
  )
}

// A menu row with a list of choices that opens to the left of the account menu,
// or below the row in the phone's sheet, where there is no hover.
function SubmenuRow<T extends string>({
  id,
  label,
  options,
  current,
  open,
  inline,
  onOpenChange,
  onSelect,
}: {
  id: string
  label: string
  options: { value: T; label: string; lang?: string }[]
  current: T
  open: boolean
  inline: boolean
  onOpenChange: (open: boolean) => void
  onSelect: (value: T) => void
}) {
  const choices = options.map((option) => (
    <MenuRow
      key={option.value}
      lang={option.lang}
      aria-pressed={current === option.value}
      onClick={() => onSelect(option.value)}
    >
      {option.label}
      {current === option.value && (
        <Check className="ml-auto size-4 text-vergex-ink" aria-hidden="true" />
      )}
    </MenuRow>
  ))
  return (
    <div
      className="relative"
      onMouseEnter={inline ? undefined : () => onOpenChange(true)}
      onMouseLeave={inline ? undefined : () => onOpenChange(false)}
    >
      <MenuRow
        aria-expanded={open}
        aria-controls={id}
        onClick={() => onOpenChange(!open)}
      >
        {label}
        <span className="ml-auto flex min-w-0 items-center gap-1 text-xs text-vergex-muted">
          <span className="truncate">
            {options.find((o) => o.value === current)?.label}
          </span>
          <ChevronRight
            className={cn(
              'size-4 shrink-0 transition-transform',
              inline && open && 'rotate-90'
            )}
            aria-hidden="true"
          />
        </span>
      </MenuRow>
      {open &&
        (inline ? (
          <div id={id} role="group" aria-label={label} className="pl-3">
            {choices}
          </div>
        ) : (
          <div id={id} className="absolute -top-[7px] right-full pr-1.5">
            <div
              role="group"
              aria-label={label}
              className="w-48 origin-top-right animate-menu-in rounded-2xl border border-vergex-line bg-vergex-surface p-1.5 shadow-(--shadow-float)"
            >
              {choices}
            </div>
          </div>
        ))}
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
  // Mounted on the first open and kept, so the sheet can fold away with its rows.
  const [sheetUsed, setSheetUsed] = useState(false)
  const [userOpen, setUserOpen] = useState(false)
  const [submenu, setSubmenu] = useState<'theme' | 'language' | null>(null)
  const toggleSubmenu = (name: 'theme' | 'language') => (open: boolean) =>
    setSubmenu((current) => (open ? name : current === name ? null : current))
  const userRef = useRef<HTMLDivElement>(null)
  const toggleRef = useRef<HTMLButtonElement>(null)
  const resolvedPage = currentPage ?? getCurrentPageForPath(location.pathname)
  const closeMenus = () => {
    setUserOpen(false)
    setMenuOpen(false)
  }
  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (userRef.current && !userRef.current.contains(event.target as Node))
        setUserOpen(false)
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setUserOpen(false)
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', escape)
    }
  }, [])
  // The sheet covers the page, so the page holds still and is out of reach. A
  // phone turned sideways has no sheet, so nothing may stay locked behind it.
  useEffect(() => {
    if (!menuOpen) return
    const unlock = lockPageScroll()
    const main = document.querySelector('main')
    main?.setAttribute('inert', '')
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setMenuOpen(false)
      toggleRef.current?.focus()
    }
    const resize = () => {
      if (window.innerWidth >= 640) setMenuOpen(false)
    }
    document.addEventListener('keydown', escape)
    window.addEventListener('resize', resize)
    return () => {
      unlock()
      main?.removeAttribute('inert')
      document.removeEventListener('keydown', escape)
      window.removeEventListener('resize', resize)
    }
  }, [menuOpen])
  useEffect(() => {
    setMenuOpen(false)
    setUserOpen(false)
  }, [location.pathname])
  useEffect(() => {
    if (!userOpen && !menuOpen) setSubmenu(null)
  }, [userOpen, menuOpen])
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
    {
      page: 'futures',
      path: ROUTES.futures,
      label: t('futuresNav', language),
      auth: true,
    },
    {
      page: 'traders',
      path: ROUTES.traders,
      label: t('tradersNav', language),
      auth: true,
    },
  ]
  const currentTab = tabs.find((tab) => tab.page === resolvedPage)
  const open = (tab: (typeof tabs)[number]) => {
    setMenuOpen(false)
    if (tab.auth && !isLoggedIn) {
      onLoginRequired?.(tab.label)
      return
    }
    onPageChange?.(tab.page)
    navigate(tab.path)
  }
  // The same account rows in the avatar's menu and in the phone's sheet.
  const account = (inSheet: boolean) =>
    user && (
      <>
        <div className="flex items-center gap-3 px-2.5 py-2.5">
          <UserAvatar
            email={user.email}
            avatar={user.avatar}
            className="size-10 text-base"
          />
          <div className="min-w-0 flex-1">
            {user.displayName && (
              <p className="truncate text-sm font-medium text-vergex-ink">
                {user.displayName}
              </p>
            )}
            <p
              className={
                user.displayName
                  ? 'truncate text-xs text-vergex-muted'
                  : 'truncate text-sm font-medium text-vergex-ink'
              }
              title={user.email}
            >
              {user.email}
            </p>
          </div>
        </div>
        <div className="my-1 h-px bg-vergex-line" />
        {onThemeChange && (
          <SubmenuRow
            id={inSheet ? 'vergex-sheet-theme-menu' : 'vergex-theme-menu'}
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
            inline={inSheet}
            onOpenChange={toggleSubmenu('theme')}
            onSelect={onThemeChange}
          />
        )}
        {onLanguageChange && (
          <SubmenuRow
            id={inSheet ? 'vergex-sheet-language-menu' : 'vergex-language-menu'}
            label={t('language', language)}
            options={LANGUAGES.map((option) => ({
              value: option.code,
              label: option.label,
              lang: option.code,
            }))}
            current={language}
            open={submenu === 'language'}
            inline={inSheet}
            onOpenChange={toggleSubmenu('language')}
            onSelect={onLanguageChange}
          />
        )}
        {onOpenSettings && (
          <MenuRow
            onClick={() => {
              onOpenSettings()
              closeMenus()
            }}
          >
            {t('settingsNav', language)}
          </MenuRow>
        )}
        <MenuRow
          onClick={() => {
            navigate(ROUTES.help)
            closeMenus()
          }}
        >
          {t('helpNav', language)}
        </MenuRow>
        {onLogout && (
          <>
            <div className="my-1 h-px bg-vergex-line" />
            <MenuRow
              className="text-vergex-danger hover:bg-vergex-danger/10 hover:text-vergex-danger"
              onClick={() => {
                onLogout()
                closeMenus()
              }}
            >
              {t('exitLogin', language)}
            </MenuRow>
          </>
        )}
      </>
    )
  return (
    <nav
      className={cn('vergex-navigation', menuOpen && 'is-open')}
      aria-label={t('mainNavigation', language)}
    >
      <div className="vergex-navigation-inner">
        <button
          className="vergex-home"
          onClick={() => {
            setMenuOpen(false)
            navigate(ROUTES.home)
          }}
          aria-label={t('vergexHome', language)}
        >
          <BrandLogo />
        </button>
        {currentTab && (
          <span className="vergex-nav-current" aria-hidden="true">
            {currentTab.label}
          </span>
        )}
        {/* Before the menu, so on a phone the sheet's rows come right after it. */}
        <button
          ref={toggleRef}
          type="button"
          className="vergex-nav-toggle"
          aria-label={t(
            menuOpen ? 'closeNavigation' : 'openNavigation',
            language
          )}
          aria-expanded={menuOpen}
          onClick={() => {
            setMenuOpen(!menuOpen)
            setSheetUsed(true)
          }}
        >
          <span aria-hidden="true" />
          <span aria-hidden="true" />
        </button>
        <div className="vergex-nav-menu">
          <div className="vergex-nav-links">
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
          {sheetUsed && isLoggedIn && (
            <div className="vergex-nav-account">{account(true)}</div>
          )}
        </div>
        <div className="vergex-navigation-actions">
          {isLoggedIn && user ? (
            <div className="vergex-user" ref={userRef}>
              <Button
                variant="ghost"
                size="icon"
                className="size-10 rounded-full hover:bg-vergex-ink-tint aria-expanded:bg-vergex-ink-tint"
                aria-label={t('accountMenuFor', language, {
                  email: user.email,
                })}
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
                  className="absolute right-0 top-[calc(100%+4px)] z-50 w-72 origin-top-right animate-menu-in rounded-2xl border border-vergex-line bg-vergex-surface p-1.5 shadow-(--shadow-float)"
                >
                  {account(false)}
                </div>
              )}
            </div>
          ) : (
            resolvedPage !== 'login' && (
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
