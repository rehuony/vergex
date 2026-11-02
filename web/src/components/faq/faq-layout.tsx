import { useEffect, useMemo, useState } from 'react'
import { Page } from '@/components/common/page'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { FAQSearchBar } from './faq-search-bar'
import { FAQSidebar } from './faq-sidebar'
import { FAQContent } from './faq-content'
import { faqCategories, type FAQCategory, type FAQItem } from '@/i18n'
import { Button } from '@/components/ui/button'

/** Plain text of an item, used by the search filter. */
function faqItemSearchText(item: FAQItem): string {
  const parts: string[] = [item.question]
  for (const block of item.blocks) {
    if (block.type === 'p' || block.type === 'note') parts.push(block.text)
    else if (block.type === 'list' || block.type === 'steps')
      parts.push(block.items.join(' '))
    else if (block.type === 'links')
      parts.push(block.links.map((l) => l.label).join(' '))
  }
  return parts.join(' ').toLowerCase()
}

// Scrolls to a shown topic; false when the current search hides it.
function scrollToItem(itemId: string) {
  const element = document.getElementById(itemId)
  if (!element) return false
  const offset = 100
  const top = element.getBoundingClientRect().top + window.pageYOffset - offset
  window.scrollTo({ top, behavior: 'smooth' })
  return true
}

export function FAQLayout() {
  const { language } = useLanguage()
  const categories = faqCategories(language)
  const [searchTerm, setSearchTerm] = useState('')
  const [activeItemId, setActiveItemId] = useState<string | null>(null)

  const filteredCategories = useMemo(() => {
    if (!searchTerm.trim()) return categories

    const term = searchTerm.toLowerCase()
    const filtered: FAQCategory[] = []
    categories.forEach((category) => {
      const matchingItems = category.items.filter((item) =>
        faqItemSearchText(item).includes(term)
      )
      if (matchingItems.length > 0) {
        filtered.push({ ...category, items: matchingItems })
      }
    })
    return filtered
  }, [searchTerm, categories])

  // With no match the sidebar lists every topic instead of standing empty.
  const sidebarCategories =
    filteredCategories.length > 0 ? filteredCategories : categories

  // A topic the search hides clears the search, then scrolls once it shows.
  const [pendingItemId, setPendingItemId] = useState<string | null>(null)
  useEffect(() => {
    if (!pendingItemId) return
    scrollToItem(pendingItemId)
    setPendingItemId(null)
  }, [pendingItemId])

  const handleItemClick = (_categoryId: string, itemId: string) => {
    if (scrollToItem(itemId)) return
    setSearchTerm('')
    setPendingItemId(itemId)
  }

  return (
    <Page title={t('helpNav', language)}>
      <div className="flex flex-col gap-4 lg:flex-row lg:gap-8">
        {/* Stays in view beside the topics. It ends 48px above the viewport, the page's bottom
            padding, so at the end of the page the column never pushes it up. A phone has no
            room for the topics and keeps the search. */}
        <aside className="flex shrink-0 flex-col gap-4 lg:sticky lg:top-24 lg:h-[calc(100dvh-9rem)] lg:w-64 lg:self-start">
          <FAQSearchBar
            searchTerm={searchTerm}
            onSearchChange={setSearchTerm}
            placeholder={t('helpSearch', language)}
            clearLabel={t('clearSearch', language)}
          />
          <FAQSidebar
            categories={sidebarCategories}
            activeItemId={activeItemId}
            onItemClick={handleItemClick}
          />
        </aside>

        <div className="min-w-0 flex-1">
          {filteredCategories.length > 0 ? (
            <FAQContent
              categories={filteredCategories}
              onActiveItemChange={setActiveItemId}
            />
          ) : (
            <div className="rounded-2xl border border-vergex-line bg-vergex-surface py-16 text-center">
              <p className="font-sans text-sm text-vergex-muted">
                {t('helpNoMatch', language, { term: searchTerm })}
              </p>
              <Button
                variant="soft"
                size="sm"
                className="mt-4"
                onClick={() => setSearchTerm('')}
              >
                {t('clearSearch', language)}
              </Button>
            </div>
          )}
        </div>
      </div>
    </Page>
  )
}
