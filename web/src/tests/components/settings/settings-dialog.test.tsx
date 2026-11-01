import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { ApiError } from '@/api/http-client'
import { SettingsDialog } from '@/components/settings/settings-dialog'

const mocks = vi.hoisted(() => ({
  changePassword: vi.fn(),
  updateDisplayName: vi.fn(),
  mutateProfile: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    changePassword: mocks.changePassword,
    updateDisplayName: mocks.updateDisplayName,
  },
}))
vi.mock('@/contexts/auth-context', () => ({
  useAuth: () => ({ user: { id: 'owner', email: 'owner@vergex.test' } }),
}))
vi.mock('@/hooks/use-profile', () => ({
  useProfile: () => ({
    data: { avatar: '', display_name: 'Admin' },
    mutate: mocks.mutateProfile,
  }),
}))
vi.mock('@/contexts/language-context', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/contexts/language-context')>()),
  useLanguage: () => ({ language: 'en', setLanguage: vi.fn() }),
}))

it('changes the password only with the current one and a confirmed new one', async () => {
  mocks.changePassword.mockResolvedValue(undefined)
  render(
    <SettingsDialog
      section="account"
      onSectionChange={vi.fn()}
      onClose={vi.fn()}
    />
  )
  const submit = screen.getByRole('button', { name: 'Update password' })
  fireEvent.change(screen.getByLabelText('Current password'), {
    target: { value: 'old-secret' },
  })
  fireEvent.change(screen.getByLabelText('New password'), {
    target: { value: 'new-secret-1' },
  })
  const confirm = screen.getByLabelText('Confirm new password')
  // Part-way through typing it again is not a mismatch yet.
  fireEvent.change(confirm, { target: { value: 'new-sec' } })
  expect(confirm).not.toHaveAccessibleDescription()
  fireEvent.change(confirm, { target: { value: 'new-secret-2' } })
  expect(submit).toBeDisabled()
  expect(confirm).toHaveAccessibleDescription('Passwords do not match')

  fireEvent.change(confirm, { target: { value: 'new-secret-1' } })
  fireEvent.click(submit)

  await waitFor(() =>
    expect(mocks.changePassword).toHaveBeenCalledWith(
      'old-secret',
      'new-secret-1'
    )
  )
  await waitFor(() =>
    expect(screen.getByLabelText('Current password')).toHaveValue('')
  )
})

it('flags a wrong current password on the field until it is edited', async () => {
  mocks.changePassword.mockRejectedValue(
    new ApiError('Current password is incorrect', 'current_password_incorrect')
  )
  render(
    <SettingsDialog
      section="account"
      onSectionChange={vi.fn()}
      onClose={vi.fn()}
    />
  )
  const current = screen.getByLabelText('Current password')
  fireEvent.change(current, { target: { value: 'not-it' } })
  fireEvent.change(screen.getByLabelText('New password'), {
    target: { value: 'new-secret-1' },
  })
  fireEvent.change(screen.getByLabelText('Confirm new password'), {
    target: { value: 'new-secret-1' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Update password' }))

  await waitFor(() =>
    expect(current).toHaveAccessibleDescription('Current password is incorrect')
  )
  fireEvent.change(current, { target: { value: 'another-try' } })
  expect(current).not.toHaveAccessibleDescription()
})

it('saves the name on Enter or blur, and restores an emptied one', async () => {
  mocks.updateDisplayName.mockResolvedValue('Trader Joe')
  render(
    <SettingsDialog
      section="account"
      onSectionChange={vi.fn()}
      onClose={vi.fn()}
    />
  )
  const name = screen.getByLabelText('Name')
  expect(name).toHaveValue('Admin')
  expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()

  // Leaving it empty saves nothing and puts the saved name back.
  name.focus()
  fireEvent.change(name, { target: { value: '   ' } })
  fireEvent.blur(name)
  expect(name).toHaveValue('Admin')
  expect(mocks.updateDisplayName).not.toHaveBeenCalled()

  name.focus()
  fireEvent.change(name, { target: { value: '  Trader Joe ' } })
  fireEvent.keyDown(name, { key: 'Enter' })
  await waitFor(() =>
    expect(mocks.updateDisplayName).toHaveBeenCalledWith('Trader Joe')
  )
  expect(mocks.updateDisplayName).toHaveBeenCalledTimes(1)
  await waitFor(() =>
    expect(mocks.mutateProfile).toHaveBeenCalledWith(
      { avatar: '', display_name: 'Trader Joe' },
      { revalidate: false }
    )
  )
})
