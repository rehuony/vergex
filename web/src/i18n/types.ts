import type { LucideIcon } from 'lucide-react'

/**
 * Answers are composed from typed blocks so the renderer stays generic, with no
 * per-question JSX special cases. Inline `code` spans are written with backticks
 * and parsed by the renderer.
 */
export type FAQBlock =
  | { type: 'p'; text: string }
  | { type: 'list'; items: string[] }
  | { type: 'steps'; items: string[] }
  | { type: 'note'; text: string }
  | { type: 'links'; links: { label: string; href: string }[] }

export interface FAQItem {
  id: string
  question: string
  blocks: FAQBlock[]
}

interface FAQSection {
  id: string
  title: string
  items: FAQItem[]
}

/** The help pages as each language writes them. */
export type FAQContent = FAQSection[]

export interface FAQCategory extends FAQSection {
  icon: LucideIcon
}
