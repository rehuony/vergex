import { act, render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { ConfirmDialogProvider } from '@/components/common/confirm-dialog'
import { confirmToast } from '@/components/common/notify'

it('paints the confirm button red only for destructive confirmations', async () => {
  render(
    <ConfirmDialogProvider>
      <div />
    </ConfirmDialogProvider>
  )

  act(() => {
    void confirmToast('Delete this trader?', {
      okText: 'Delete',
      tone: 'danger',
    })
  })
  expect(await screen.findByRole('button', { name: 'Delete' })).toHaveClass(
    'bg-vergex-danger'
  )
  act(() => screen.getByRole('button', { name: 'Cancel' }).click())

  act(() => {
    void confirmToast('Save changes?', { okText: 'Save' })
  })
  expect(await screen.findByRole('button', { name: 'Save' })).not.toHaveClass(
    'bg-vergex-danger'
  )
})
