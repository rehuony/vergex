import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { RegistrationDisabled } from '@/components/auth/registration-disabled'
import { LanguageProvider } from '@/contexts/language-context'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

// Mock useLanguage hook
vi.mock('@/contexts/language-context', async () => {
  const actual = await vi.importActual('@/contexts/language-context')
  return {
    ...actual,
    useLanguage: () => ({ language: 'en' }),
  }
})

/**
 * RegistrationDisabled Component Tests
 *
 * Tests the component that displays when registration is disabled
 * This is part of the registration toggle feature
 */
describe('RegistrationDisabled Component', () => {
  const renderComponent = () => {
    return render(
      <MemoryRouter>
        <LanguageProvider>
          <RegistrationDisabled />
        </LanguageProvider>
      </MemoryRouter>
    )
  }

  describe('Rendering', () => {
    it('should render the component without errors', () => {
      const { container } = renderComponent()
      expect(container).toBeTruthy()
    })

    it('should display the Vergex logo', () => {
      renderComponent()
      const logo = screen.getByAltText('Vergex Logo')
      expect(logo).toBeTruthy()
      expect(logo.getAttribute('src')).toBe('/icons/vergex-flow.svg')
    })

    it('should display registration closed heading', () => {
      renderComponent()
      const heading = screen.getByText('Account already set up')
      expect(heading).toBeTruthy()
    })

    it('should display registration closed message', () => {
      renderComponent()
      const message = screen.getByText(/this instance already has one/i)
      expect(message).toBeTruthy()
    })

    it('should display back to login button', () => {
      renderComponent()
      const button = screen.getByRole('button', { name: /back to login/i })
      expect(button).toBeTruthy()
    })
  })

  describe('Navigation', () => {
    it('should navigate to login page when button is clicked', () => {
      renderComponent()
      const button = screen.getByRole('button', { name: /back to login/i })

      fireEvent.click(button)

      expect(mockNavigate).toHaveBeenCalledWith('/login')
    })
  })

  describe('Styling', () => {
    it('should take its colours from the theme', () => {
      const { container } = renderComponent()
      const mainDiv = container.firstChild as HTMLElement
      // Theme tokens, so the page follows light and dark mode.
      expect(mainDiv.className).toContain('bg-vergex-bg')
      expect(mainDiv.className).toContain('text-vergex-text')
      expect(mainDiv.getAttribute('style')).toBeNull()
    })

    it('should have centered layout', () => {
      const { container } = renderComponent()
      const mainDiv = container.firstChild as HTMLElement
      expect(mainDiv.className).toContain('flex')
      expect(mainDiv.className).toContain('items-center')
      expect(mainDiv.className).toContain('justify-center')
    })
  })
})
