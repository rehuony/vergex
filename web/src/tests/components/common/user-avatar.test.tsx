import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { UserAvatar } from '@/components/common/user-avatar'

it('shows the uploaded image, or the first letter when there is none', () => {
  const { container, rerender } = render(
    <UserAvatar email="trader@example.test" />
  )
  expect(screen.getByText('T')).toBeInTheDocument()

  rerender(
    <UserAvatar
      email="trader@example.test"
      avatar="data:image/webp;base64,AA"
    />
  )
  expect(container.querySelector('img')).toHaveAttribute(
    'src',
    'data:image/webp;base64,AA'
  )
  expect(screen.queryByText('T')).not.toBeInTheDocument()
})
