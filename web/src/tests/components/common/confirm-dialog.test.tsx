import { act, render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { ConfirmDialogProvider } from '@/components/common/confirm-dialog'
import { confirmToast } from '@/components/common/notify'

it('marks only destructive confirmations, and says what is asked in the language in use', async () => {
  render(
    <ConfirmDialogProvider>
      <div />
    </ConfirmDialogProvider>
  )

  act(() => {
    void confirmToast('Delete this trader?', {
      title: 'Delete trader',
      okText: 'Delete',
      tone: 'danger',
    })
  })
  expect(await screen.findByRole('button', { name: 'Delete' })).toHaveClass(
    'bg-vergex-danger'
  )
  expect(
    screen.getByRole('alertdialog', { name: 'Delete trader' })
  ).toHaveAccessibleDescription('Delete this trader?')
  act(() => screen.getByRole('button', { name: 'Cancel' }).click())

  act(() => {
    void confirmToast('Save changes?', { title: 'Save', okText: 'Save' })
  })
  expect(await screen.findByRole('button', { name: 'Save' })).not.toHaveClass(
    'bg-vergex-danger'
  )
})
