import { afterEach, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { AssistantMessage } from '@/components/futures/assistant-message'

afterEach(cleanup)

it('sets the Markdown a model writes as text', () => {
  const { container } = render(
    <AssistantMessage
      text={[
        '## Setup',
        'Entry near **78,000** with `5x`.',
        '',
        '- First target',
        '- Second target',
        '',
        '1. Wait',
        '2. Enter',
        '',
        '| Level | Price |',
        '| --- | --- |',
        '| Stop | 76,500 |',
      ].join('\n')}
    />
  )
  expect(screen.getByRole('heading', { name: 'Setup' })).toBeVisible()
  expect(screen.getByText('78,000').tagName).toBe('STRONG')
  expect(screen.getByText('5x').tagName).toBe('CODE')
  expect(container.querySelectorAll('ul li')).toHaveLength(2)
  expect(container.querySelectorAll('ol li')).toHaveLength(2)
  expect(screen.getByRole('columnheader', { name: 'Price' })).toBeVisible()
  expect(screen.getByRole('cell', { name: '76,500' })).toBeVisible()
})

it('never turns a reply into markup', () => {
  const { container } = render(
    <AssistantMessage text="<img src=x onerror=alert(1)> **<b>bold</b>**" />
  )
  expect(container.querySelector('img')).toBeNull()
  expect(container.querySelector('b')).toBeNull()
  expect(container).toHaveTextContent('<img src=x onerror=alert(1)>')
})
