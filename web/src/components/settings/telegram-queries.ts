import { queryOptions } from '@tanstack/react-query'
import { api } from '@/api'
import { t, type Language } from '@/i18n'
import type { TelegramVisitor } from '@/types'
import { pollEvery } from '@/utils/poll'

// Who tried the bot, read by the row that counts them and the list that names them.
export const telegramVisitorsQuery = queryOptions({
  queryKey: ['telegram-visitors'],
  queryFn: () => api.getTelegramVisitors(),
  ...pollEvery(15000),
})

export const visitorsTotal = (
  visitors: TelegramVisitor[],
  language: Language
) =>
  t('telegram.visitorsTotal', language, {
    people: visitors.length,
    attempts: visitors.reduce((sum, visitor) => sum + visitor.attempts, 0),
  })
