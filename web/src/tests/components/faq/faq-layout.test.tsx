import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { faqCategories } from '@/i18n'
import { FAQLayout } from '@/components/faq/faq-layout'

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))

beforeEach(() => {
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      disconnect() {}
    }
  )
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('lists every topic beside a search with no match, and opens one', () => {
  render(
    <MemoryRouter>
      <FAQLayout />
    </MemoryRouter>
  )
  const search = screen.getByRole('textbox', { name: 'Search help docs' })
  fireEvent.change(search, { target: { value: 'no such topic' } })
  expect(screen.getByText(/No matching questions/)).toBeVisible()

  const topics = faqCategories('en').flatMap((category) => category.items)
  const sidebar = screen.getByRole('navigation')
  expect(sidebar.querySelectorAll('button')).toHaveLength(topics.length)

  fireEvent.click(screen.getByRole('button', { name: topics[0].question }))
  expect(search).toHaveValue('')
  expect(screen.queryByText(/No matching questions/)).toBeNull()
  expect(window.scrollTo).toHaveBeenCalledTimes(1)
})
