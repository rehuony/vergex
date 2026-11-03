import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import {
  readAssistantPage,
  useAssistantPage,
  useAssistantPageName,
} from '@/hooks/use-assistant-page'

function Page({ name, id }: { name: string; id: string }) {
  useAssistantPage(name, () => ({ id }))
  return null
}

function Name() {
  return <span>{useAssistantPageName() || 'none'}</span>
}

afterEach(cleanup)

describe('useAssistantPage', () => {
  it('says how the page stands when asked, and nothing once it is gone', () => {
    const { rerender, unmount } = render(<Page name="traders" id="t1" />)
    expect(readAssistantPage()).toEqual({ page: 'traders', id: 't1' })
    rerender(<Page name="traders" id="t2" />)
    expect(readAssistantPage()).toEqual({ page: 'traders', id: 't2' })
    unmount()
    expect(readAssistantPage()).toBeNull()
  })

  it('tells what depends on it when another page takes over', () => {
    const { rerender } = render(
      <>
        <Name />
        <Page name="traders" id="t1" />
      </>
    )
    expect(screen.getByText('traders')).toBeInTheDocument()
    rerender(
      <>
        <Name />
        <Page name="futures" id="p1" />
      </>
    )
    expect(screen.getByText('futures')).toBeInTheDocument()
    expect(readAssistantPage()).toEqual({ page: 'futures', id: 'p1' })
  })
})
