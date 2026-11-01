import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { SWRConfig } from 'swr'
import { afterEach, expect, it, vi } from 'vitest'
import { LoginPage } from '@/pages/login-page'
import { ResetAccountPage } from '@/pages/reset-account-page'

const login = vi.fn()

vi.mock('@/contexts/auth-context', () => ({
  useAuth: () => ({ login }),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({
  api: {
    getSupportedExchanges: async () => [{ id: 'a' }, { id: 'b' }],
    getSupportedModels: async () => [{ id: 'a' }, { id: 'b' }, { id: 'c' }],
  },
}))

afterEach(() => {
  cleanup()
  sessionStorage.clear()
  login.mockReset()
})

function renderPage() {
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <MemoryRouter initialEntries={['/login']}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/reset-account" element={<ResetAccountPage />} />
        </Routes>
      </MemoryRouter>
    </SWRConfig>
  )
  return {
    email: screen.getByLabelText('Email'),
    password: screen.getByLabelText('Password'),
  }
}

async function signIn(email: HTMLElement, password: HTMLElement) {
  fireEvent.change(email, { target: { value: 'me@example.test' } })
  fireEvent.change(password, { target: { value: 'secret' } })
  fireEvent.click(screen.getByRole('button', { name: 'Sign In' }))
}

it('shows wrong credentials in the line above the button until the next edit', async () => {
  login.mockResolvedValue({ success: false, message: 'nope', status: 401 })
  const { email, password } = renderPage()
  await signIn(email, password)

  const message = await screen.findByText('Invalid email or password')
  expect(message).toHaveAttribute('aria-live', 'polite')
  expect(password).toHaveAttribute('aria-invalid', 'true')
  expect(screen.queryByText('nope')).toBeNull()

  fireEvent.change(password, { target: { value: 'secret2' } })
  expect(screen.queryByText('Invalid email or password')).toBeNull()
})

it('shows what the server said for other failures', async () => {
  login.mockResolvedValue({
    success: false,
    message: 'Too many attempts',
    status: 429,
  })
  const { email, password } = renderPage()
  await signIn(email, password)
  expect(await screen.findByText('Too many attempts')).toBeVisible()
})

it('says the session expired after a 401 sent the user here', () => {
  sessionStorage.setItem('from401', 'true')
  renderPage()
  expect(screen.getByText(/Session expired/)).toBeVisible()
  expect(sessionStorage.getItem('from401')).toBeNull()
})

it('counts the exchanges and Model providers the server supports', async () => {
  renderPage()
  expect(
    (await screen.findByText('Exchanges')).previousSibling
  ).toHaveTextContent('2')
  expect(screen.getByText('Model providers').previousSibling).toHaveTextContent(
    '3'
  )
})

it('opens the reset-account page from "Forgot account?"', () => {
  renderPage()
  fireEvent.click(screen.getByRole('button', { name: 'Forgot account?' }))
  expect(
    screen.getByRole('heading', { name: 'Reset your account' })
  ).toBeVisible()
  expect(screen.getByText(/\.\/vergex reset-account$/)).toBeVisible()
})
