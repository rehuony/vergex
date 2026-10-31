import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SetupPage } from '@/pages/setup-page'

const mocks = vi.hoisted(() => ({
  register: vi.fn(),
}))

vi.mock('@/contexts/auth-context', () => ({
  useAuth: () => ({ register: mocks.register }),
}))

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))

describe('first-time setup', () => {
  beforeEach(() => {
    mocks.register.mockReset()
    mocks.register.mockResolvedValue({ success: true })
  })

  it('does not offer Advanced mode', () => {
    render(
      <MemoryRouter>
        <SetupPage />
      </MemoryRouter>
    )

    expect(screen.queryByText(/Advanced Mode/i)).toBeNull()
    expect(
      screen.queryByText(/configure models, wallets, and exchanges/i)
    ).toBeNull()
  })

  it('registers the first owner', async () => {
    render(
      <MemoryRouter>
        <SetupPage />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByPlaceholderText('you@example.com'), {
      target: { value: 'owner@example.com' },
    })
    fireEvent.change(screen.getByPlaceholderText('At least 8 characters'), {
      target: { value: 'StrongPass123!' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Get Started' }))

    await waitFor(() => {
      expect(mocks.register).toHaveBeenCalledWith(
        'owner@example.com',
        'StrongPass123!'
      )
    })
  })
})
