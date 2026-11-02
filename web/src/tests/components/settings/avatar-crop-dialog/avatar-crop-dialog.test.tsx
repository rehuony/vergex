import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { AvatarCropDialog } from '@/components/settings/avatar-crop-dialog'

const rotateQuarter = vi.hoisted(() => vi.fn())
vi.mock('@/components/settings/avatar', () => ({
  renderAvatar: vi.fn(),
  rotateQuarter,
}))
vi.mock('@/contexts/language-context', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/contexts/language-context')>()),
  useLanguage: () => ({ language: 'en', setLanguage: vi.fn() }),
}))

const photo = () => document.querySelector('.vergex-avatar-crop img')

const open = () =>
  render(
    <AvatarCropDialog
      src="blob:photo"
      language="en"
      onCancel={vi.fn()}
      onSave={vi.fn()}
    />
  )

it('moves the frame, never the photo under it', () => {
  open()
  expect(photo()).toHaveAttribute('draggable', 'false')
})

it('keeps every control clickable and runs quick clicks in order', async () => {
  URL.revokeObjectURL = vi.fn()
  let made = 0
  rotateQuarter.mockImplementation(async () => `blob:turn-${++made}`)
  open()
  const button = (name: string) => screen.getByRole('button', { name })

  fireEvent.click(button('Rotate right'))
  fireEvent.click(button('Rotate right'))
  fireEvent.click(button('Rotate left'))
  await waitFor(() => expect(photo()).toHaveAttribute('src', 'blob:turn-3'))
  expect(rotateQuarter.mock.calls).toEqual([
    ['blob:photo', 'right'],
    ['blob:turn-1', 'right'],
    ['blob:turn-2', 'left'],
  ])

  fireEvent.click(button('Reset'))
  await waitFor(() => expect(photo()).toHaveAttribute('src', 'blob:photo'))

  // Back at the start, nothing has gone grey or stopped answering.
  for (const name of ['Rotate left', 'Reset', 'Rotate right', 'Next']) {
    expect(button(name)).toBeEnabled()
  }
  fireEvent.click(button('Rotate left'))
  await waitFor(() => expect(photo()).toHaveAttribute('src', 'blob:turn-4'))
})
