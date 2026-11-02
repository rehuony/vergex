import { ScrollArea } from '@/components/ui/scroll-area'
import type { FAQCategory } from '@/i18n'

interface FAQSidebarProps {
  categories: FAQCategory[]
  activeItemId: string | null
  onItemClick: (categoryId: string, itemId: string) => void
}

export function FAQSidebar({
  categories,
  activeItemId,
  onItemClick,
}: FAQSidebarProps) {
  return (
    <nav className="min-h-0 flex-1 max-lg:hidden">
      <ScrollArea className="h-full">
        <div className="space-y-5">
          {categories.map((category) => (
            <div key={category.id}>
              <div className="mb-2 flex items-center gap-2 px-2">
                <category.icon className="h-3.5 w-3.5 text-vergex-ink" />
                <h3 className="font-sans text-[11px] font-semibold tracking-normal text-vergex-ink">
                  {category.title}
                </h3>
              </div>
              <ul className="space-y-0.5 border-l border-vergex-line">
                {category.items.map((item) => {
                  const isActive = activeItemId === item.id
                  return (
                    <li key={item.id}>
                      <button
                        onClick={() => onItemClick(category.id, item.id)}
                        className={`-ml-px w-full border-l-2 py-1.5 pl-3 pr-2 text-left text-[13px] leading-5 transition-colors ${
                          isActive
                            ? 'border-vergex-ink bg-vergex-ink-tint font-medium text-vergex-ink'
                            : 'border-transparent text-vergex-muted hover:border-vergex-line-strong hover:text-vergex-ink'
                        }`}
                      >
                        {item.question}
                      </button>
                    </li>
                  )
                })}
              </ul>
            </div>
          ))}
        </div>
      </ScrollArea>
    </nav>
  )
}
