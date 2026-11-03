import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { MemoryRouter } from 'react-router-dom'
import type { AssistantProposal } from '@/api/assistant'
import { ProposalCard } from '@/components/assistant/proposal-card'
import {
  mergeFormValue,
  readAssistantPage,
  useAssistantForm,
  useAssistantPage,
} from '@/hooks/use-assistant-page'

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
afterEach(cleanup)

// A page with a form, whose values only change through what it was given.
function Form({ long = '' }: { long?: string }) {
  useAssistantPage('test')
  const [interval, setInterval] = useState('1h')
  const [config, setConfig] = useState<unknown>({
    risk: { max: 2, on: true },
    note: long,
  })
  useAssistantForm({
    id: 'test.form',
    title: 'Test form',
    fields: [
      {
        name: 'interval',
        label: 'Interval',
        type: 'choice',
        options: [
          { value: '1h', label: 'One hour' },
          { value: '4h', label: 'Four hours' },
        ],
        value: interval,
      },
      { name: 'config', label: 'Config', type: 'object', value: config },
    ],
    apply: (values) => {
      if (typeof values.interval === 'string') setInterval(values.interval)
      if (values.config) setConfig(mergeFormValue(config, values.config))
    },
  })
  return (
    <output>
      {interval} {JSON.stringify(config)}
    </output>
  )
}

const proposal: AssistantProposal = {
  form: 'test.form',
  title: 'Test form',
  values: { interval: '4h', config: { risk: { max: 3, on: true } } },
  reason: 'Longer is steadier',
  route: '/somewhere',
}

function card(handlers: Partial<Record<'onFill' | 'onDismiss', () => void>>) {
  return (
    <MemoryRouter>
      <ProposalCard
        proposal={proposal}
        onFill={handlers.onFill ?? (() => {})}
        onDismiss={handlers.onDismiss ?? (() => {})}
        onFollow={() => {}}
      />
    </MemoryRouter>
  )
}

describe('ProposalCard', () => {
  it('shows what would change, by label and path, and nothing else', () => {
    render(
      <>
        <Form />
        {card({})}
      </>
    )
    expect(screen.getByText('Interval')).toBeInTheDocument()
    expect(screen.getByText('One hour')).toBeInTheDocument()
    expect(screen.getByText(/Four hours/)).toBeInTheDocument()
    expect(screen.getByText('risk.max')).toBeInTheDocument()
    expect(screen.queryByText('risk.on')).toBeNull()
    expect(screen.getByText('Longer is steadier')).toBeInTheDocument()
    // Nothing goes in by being shown.
    expect(screen.getByRole('status')).toHaveTextContent('1h')
  })

  it('says when the form has gone, and leads back to where it was', () => {
    const fill = vi.fn()
    render(card({ onFill: fill }))
    expect(screen.getByText('That form is no longer open')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Fill in' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Open it again' })).toHaveAttribute(
      'href',
      '/somewhere'
    )
  })

  it('fills in only when pressed, through what the page gave', async () => {
    const { openAssistantForm } = await import('@/hooks/use-assistant-page')
    render(
      <>
        <Form />
        {card({
          onFill: () => openAssistantForm('test.form')!.apply(proposal.values),
        })}
      </>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Fill in' }))
    const output = screen.getByRole('status')
    expect(output).toHaveTextContent('4h')
    expect(output).toHaveTextContent('"max":3')
    expect(output).toHaveTextContent('"on":true')
  })
})

describe('what a page says about its forms', () => {
  it('names them with their fields and leaves out how they apply', () => {
    render(<Form />)
    const page = readAssistantPage() as { forms: Record<string, unknown>[] }
    expect(page.forms[0]).toMatchObject({ id: 'test.form', title: 'Test form' })
    expect(page.forms[0]).not.toHaveProperty('apply')
    cleanup()
    expect(readAssistantPage()).toBeNull()
  })

  it('cuts a long text short, which the form keeps whole', () => {
    render(<Form long={'x'.repeat(2000)} />)
    expect(JSON.stringify(readAssistantPage()).length).toBeLessThan(1500)
    expect(screen.getByRole('status').textContent!.length).toBeGreaterThan(2000)
  })
})
