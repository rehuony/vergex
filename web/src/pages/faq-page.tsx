import { FAQLayout } from '@/components/faq/faq-layout'
import { useAssistantPage } from '@/hooks/use-assistant-page'

/**
 * FAQ page — layout, search, sidebar and content live in components/faq/*;
 * the questions themselves are data in i18n/locales/<language>/faq.ts.
 */
export function FAQPage() {
  useAssistantPage('help')
  return <FAQLayout />
}
