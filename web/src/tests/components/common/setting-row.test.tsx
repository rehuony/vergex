import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { SettingList } from '@/components/common/setting-row'

const scrolls = (element: HTMLElement) =>
  element.closest('.overflow-y-auto') !== null

it('scrolls what a window holds between its head and its bar', () => {
  render(
    <SettingList
      head={<p>Head</p>}
      message="Note"
      actions={<button>Save</button>}
    >
      <p>Row</p>
    </SettingList>
  )
  expect(scrolls(screen.getByText('Row'))).toBe(true)
  expect(scrolls(screen.getByText('Head'))).toBe(false)
  expect(scrolls(screen.getByRole('button', { name: 'Save' }))).toBe(false)
  expect(screen.getByText('Note')).toBeVisible()
})

it('has no bar with nothing to do', () => {
  render(
    <SettingList>
      <p>Row</p>
    </SettingList>
  )
  expect(document.querySelector('[aria-live]')).toBeNull()
})
