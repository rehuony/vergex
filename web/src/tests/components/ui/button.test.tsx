import { render, screen } from '@testing-library/react'
import { MemoryRouter, Link } from 'react-router-dom'
import { expect, it } from 'vitest'
import { Button } from '@/components/ui/button'

it('lets a single button override the shared style with Tailwind classes', () => {
  render(
    <Button variant="primary" size="md" className="h-11 bg-vergex-danger">
      Save
    </Button>
  )
  const button = screen.getByRole('button', { name: 'Save' })
  expect(button).toHaveClass('h-11', 'bg-vergex-danger', 'rounded-full')
  expect(button).not.toHaveClass('h-9')
  expect(button).not.toHaveClass('bg-vergex-ink')
})

it('styles a router link as a button', () => {
  render(
    <MemoryRouter>
      <Button asChild variant="secondary">
        <Link to="/connections">Connections</Link>
      </Button>
    </MemoryRouter>
  )
  const link = screen.getByRole('link', { name: 'Connections' })
  expect(link).toHaveAttribute('href', '/connections')
  expect(link).toHaveClass('bg-vergex-ink-tint', 'h-9')
})

it('keeps a text link free of the size box', () => {
  render(<Button variant="link">Manage account</Button>)
  const button = screen.getByRole('button', { name: 'Manage account' })
  expect(button).toHaveClass('h-auto', 'px-0')
  expect(button).not.toHaveClass('h-9')
})
