import {
  BookOpen,
  Crosshair,
  Layers,
  LineChart,
  Monitor,
  Send,
  Shield,
  TrendingUp,
  Wrench,
  Zap,
  type LucideIcon,
} from 'lucide-react'
import { en } from '@/i18n/locales/en'
import { faq as enFaq } from '@/i18n/locales/en/faq'
import { zhCN } from '@/i18n/locales/zh-cn'
import { faq as zhCNFaq } from '@/i18n/locales/zh-cn/faq'
import type { FAQCategory, FAQContent } from '@/i18n/types'

export type { FAQBlock, FAQCategory, FAQItem } from '@/i18n/types'

export type Language = 'en' | 'zh-CN'

export const DEFAULT_LANGUAGE: Language = 'en'

// Each option is labelled in its own language so it can be found from any UI.
export const LANGUAGES: { code: Language; label: string }[] = [
  { code: 'en', label: 'English' },
  { code: 'zh-CN', label: '简体中文' },
]

const messages: Record<Language, typeof en> = { en, 'zh-CN': zhCN }

const faqIcons: Record<string, LucideIcon> = {
  'getting-started': BookOpen,
  'launch-accounts': Zap,
  trading: TrendingUp,
  strategies: Layers,
  frontier: Crosshair,
  telegram: Send,
  forecast: LineChart,
  dashboard: Monitor,
  security: Shield,
  'self-hosting': Wrench,
}

function lookup(key: string, language: Language): string | undefined {
  // Nested keys look like 'frontier.account.title'.
  let value: unknown = messages[language]
  for (const part of key.split('.')) {
    value = (value as Record<string, unknown> | undefined)?.[part]
  }
  return typeof value === 'string' ? value : undefined
}

/** The text for a key, with `{name}` parameters filled in. */
export function t(
  key: string,
  language: Language,
  params?: Record<string, string | number>
): string {
  let text = lookup(key, language) ?? lookup(key, DEFAULT_LANGUAGE) ?? key
  if (params) {
    for (const [name, value] of Object.entries(params)) {
      text = text.split(`{${name}}`).join(String(value))
    }
  }
  return text
}

const withIcons = (content: FAQContent): FAQCategory[] =>
  content.map((section) => ({
    ...section,
    icon: faqIcons[section.id] ?? BookOpen,
  }))

const faqs: Record<Language, FAQCategory[]> = {
  en: withIcons(enFaq),
  'zh-CN': withIcons(zhCNFaq),
}

export function faqCategories(language: Language): FAQCategory[] {
  return faqs[language]
}

/**
 * The page's language, for code without one of its own to hand, such as dates and
 * charts built outside React. LanguageProvider keeps it on the page. Its codes are
 * locale tags too. Numbers stay in en-US everywhere: they read the same in both
 * languages, and columns should not change with it.
 */
export function activeLanguage(): Language {
  const lang = document.documentElement.lang
  return lang in messages ? (lang as Language) : DEFAULT_LANGUAGE
}
