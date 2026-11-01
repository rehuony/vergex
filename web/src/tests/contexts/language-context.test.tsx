import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LanguageProvider, useLanguage } from '@/contexts/language-context'
import { HeaderBar } from '@/components/common/header-bar'

function LanguageProbe() {
  const { language } = useLanguage()
  return <div>{language}</div>
}

function AppHeader() {
  const { language, setLanguage } = useLanguage()
  return (
    <HeaderBar
      isLoggedIn
      user={{ email: 'me@vergex.test' }}
      language={language}
      onLanguageChange={setLanguage}
    />
  )
}

describe('language preference', () => {
  beforeEach(() => localStorage.clear())
  afterEach(() => vi.restoreAllMocks())

  it('follows the browser until a language has been chosen', () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('zh-TW')
    // A language no longer offered counts as no choice.
    localStorage.setItem('vergex-language', 'id')

    const { unmount } = render(
      <LanguageProvider>
        <LanguageProbe />
      </LanguageProvider>
    )
    expect(screen.getByText('zh-CN')).toBeTruthy()
    expect(document.documentElement.lang).toBe('zh-CN')
    unmount()

    localStorage.setItem('vergex-language', 'en')
    render(
      <LanguageProvider>
        <LanguageProbe />
      </LanguageProvider>
    )
    expect(screen.getByText('en')).toBeTruthy()
    expect(document.documentElement.lang).toBe('en')
  })

  it('switches and remembers the language from the account menu', () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('en-US')

    render(
      <MemoryRouter>
        <LanguageProvider>
          <AppHeader />
        </LanguageProvider>
      </MemoryRouter>
    )

    expect(screen.getByRole('button', { name: 'Traders' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /Account menu/ }))
    fireEvent.click(screen.getByRole('button', { name: /^Language/ }))
    fireEvent.click(screen.getByRole('button', { name: '简体中文' }))

    expect(screen.getByRole('button', { name: '交易员' })).toBeTruthy()
    expect(
      screen
        .getByRole('button', { name: '简体中文' })
        .getAttribute('aria-pressed')
    ).toBe('true')
    expect(screen.getByRole('button', { name: /^语言/ })).toHaveTextContent(
      '简体中文'
    )
    expect(localStorage.getItem('vergex-language')).toBe('zh-CN')
    expect(document.documentElement.lang).toBe('zh-CN')
  })
})
