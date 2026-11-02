import { describe, expect, it } from 'vitest'
import { faqCategories, t, type FAQBlock } from '@/i18n'
import { en } from '@/i18n/locales/en'
import { zhCN } from '@/i18n/locales/zh-cn'

function leaves(value: object, prefix = ''): [string, string][] {
  return Object.entries(value).flatMap(([key, entry]) =>
    typeof entry === 'string'
      ? ([[prefix + key, entry]] as [string, string][])
      : leaves(entry as object, `${prefix}${key}.`)
  )
}

const placeholders = (text: string) =>
  [...text.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort()

// The shape of a block, so a translation cannot drop a step from an answer.
const blockShape = (block: FAQBlock) =>
  'items' in block ? `${block.type}:${block.items.length}` : block.type

describe('locales', () => {
  const english = new Map(leaves(en))
  const chinese = new Map(leaves(zhCN))

  it('translate every key of the English template, and nothing else', () => {
    expect([...chinese.keys()].sort()).toEqual([...english.keys()].sort())
  })

  it('keep the same placeholders in every translation', () => {
    for (const [key, text] of english) {
      expect(placeholders(chinese.get(key) ?? ''), key).toEqual(
        placeholders(text)
      )
    }
  })

  it('answer the same questions in the same shape', () => {
    const shape = (language: 'en' | 'zh-CN') =>
      faqCategories(language).map((category) => [
        category.id,
        category.items.map((item) => [item.id, item.blocks.map(blockShape)]),
      ])
    expect(shape('zh-CN')).toEqual(shape('en'))
  })
})

describe('t', () => {
  it('fills in every use of a parameter', () => {
    const params = { seconds: 2, count: 3 }
    expect(t('futures.activity.thought', 'en', params)).toBe(
      'Thought for 2s · 3 steps'
    )
    expect(t('futures.activity.thought', 'zh-CN', params)).toBe(
      '思考了 2 秒 · 3 步'
    )
  })

  it('shows the key rather than nothing for one that does not exist', () => {
    expect(t('no.such.key', 'zh-CN')).toBe('no.such.key')
  })
})
