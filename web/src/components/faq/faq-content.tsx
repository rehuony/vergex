import { Fragment, useEffect, useRef } from 'react'
import { ExternalLink } from 'lucide-react'
import type { FAQBlock, FAQCategory } from '@/i18n'

interface FAQContentProps {
  categories: FAQCategory[]
  onActiveItemChange: (itemId: string) => void
}

/** Renders text with inline `code` spans (backtick syntax). */
function InlineText({ text }: { text: string }) {
  const parts = text.split(/(`[^`]+`)/g)
  return (
    <>
      {parts.map((part, i) =>
        part.startsWith('`') && part.endsWith('`') ? (
          <code
            key={i}
            className="rounded bg-vergex-surface-muted border border-vergex-line px-1.5 py-0.5 font-mono text-xs text-vergex-ink break-all"
          >
            {part.slice(1, -1)}
          </code>
        ) : (
          <Fragment key={i}>{part}</Fragment>
        )
      )}
    </>
  )
}

function Block({ block }: { block: FAQBlock }) {
  switch (block.type) {
    case 'p':
      return (
        <p className="text-sm leading-6 text-vergex-muted">
          <InlineText text={block.text} />
        </p>
      )
    case 'list':
      return (
        <ul className="space-y-1.5">
          {block.items.map((item, i) => (
            <li
              key={i}
              className="flex gap-2 text-sm leading-6 text-vergex-muted"
            >
              <span className="mt-[9px] h-1 w-1 shrink-0 rounded-full bg-vergex-ink" />
              <span>
                <InlineText text={item} />
              </span>
            </li>
          ))}
        </ul>
      )
    case 'steps':
      return (
        <ol className="space-y-1.5">
          {block.items.map((item, i) => (
            <li
              key={i}
              className="flex gap-3 text-sm leading-6 text-vergex-muted"
            >
              <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-vergex-line bg-vergex-ink-tint font-sans text-[11px] font-semibold text-vergex-ink">
                {i + 1}
              </span>
              <span>
                <InlineText text={item} />
              </span>
            </li>
          ))}
        </ol>
      )
    case 'note':
      return (
        <div className="border-l-2 border-vergex-ink bg-vergex-ink-tint px-3 py-2 text-sm leading-6 text-vergex-ink">
          <InlineText text={block.text} />
        </div>
      )
    case 'links':
      return (
        <div className="flex flex-wrap gap-2">
          {block.links.map((link) => (
            <a
              key={link.href}
              href={link.href}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1.5 rounded-full border border-vergex-line bg-vergex-surface-muted px-2.5 py-1 font-sans text-xs font-medium text-vergex-ink hover:border-vergex-line-strong hover:bg-vergex-ink-tint"
            >
              {link.label}
              <ExternalLink className="h-3 w-3" />
            </a>
          ))}
        </div>
      )
  }
}

export function FAQContent({
  categories,
  onActiveItemChange,
}: FAQContentProps) {
  const sectionRefs = useRef<Map<string, HTMLElement>>(new Map())

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const itemId = entry.target.getAttribute('data-item-id')
            if (itemId) onActiveItemChange(itemId)
          }
        })
      },
      { rootMargin: '-100px 0px -80% 0px', threshold: 0 }
    )

    sectionRefs.current.forEach((ref) => observer.observe(ref))
    return () => {
      sectionRefs.current.forEach((ref) => observer.unobserve(ref))
    }
  }, [onActiveItemChange, categories])

  const setRef = (itemId: string, element: HTMLElement | null) => {
    if (element) sectionRefs.current.set(itemId, element)
    else sectionRefs.current.delete(itemId)
  }

  return (
    <div className="space-y-8">
      {categories.map((category) => (
        <div
          key={category.id}
          id={category.id}
          className="overflow-hidden rounded-2xl border border-vergex-line bg-vergex-surface"
        >
          <div className="flex items-center gap-2.5 border-b border-vergex-line bg-vergex-surface-muted px-5 py-3 md:px-6">
            <category.icon className="h-4 w-4 text-vergex-ink" />
            <h2 className="text-sm font-semibold text-vergex-ink">
              {category.title}
            </h2>
          </div>

          <div className="divide-y divide-vergex-line">
            {category.items.map((item) => (
              <section
                key={item.id}
                id={item.id}
                data-item-id={item.id}
                ref={(el) => setRef(item.id, el)}
                className="scroll-mt-24 px-5 py-5 md:px-6"
              >
                <h3 className="mb-3 text-sm font-semibold leading-6 text-vergex-ink">
                  {item.question}
                </h3>
                <div className="space-y-3">
                  {item.blocks.map((block, i) => (
                    <Block key={i} block={block} />
                  ))}
                </div>
              </section>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
