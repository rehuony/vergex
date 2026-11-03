import { useState } from 'react'
import { User } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { PanelLoading } from '@/components/common/panel'
import { SearchField } from '@/components/common/search-field'
import {
  SettingBack,
  SettingBadge,
  SettingItem,
  SettingList,
  SettingState,
} from '@/components/common/setting-row'
import { t, type Language } from '@/i18n'
import type { TelegramVisitor } from '@/types'
import { fmtTime } from '@/utils/trader-format'
import { telegramVisitorsQuery, visitorsTotal } from './telegram-queries'

// Past a screenful the list can be searched.
const SEARCH_FROM = 8

const handle = (visitor: TelegramVisitor) =>
  visitor.username ? `@${visitor.username}` : ''

const idOf = (visitor: TelegramVisitor) =>
  `ID ${visitor.sender_id || visitor.chat_id}`

// Who they are by the most readable thing Telegram gave for them: a name and a @username are
// both optional there, and only the id is always known.
const nameOf = (visitor: TelegramVisitor) =>
  visitor.name || handle(visitor) || idOf(visitor)

// What the name does not already say.
const detailOf = (visitor: TelegramVisitor) =>
  [
    visitor.name ? handle(visitor) : '',
    nameOf(visitor) === idOf(visitor) ? '' : idOf(visitor),
    visitor.chat_title,
    visitor.last_text,
  ]
    .filter(Boolean)
    .join(' · ')

/** Everyone who tried to use the bot and was refused, the latest first. */
export function TelegramVisitors({
  onBack,
  language,
}: {
  onBack: () => void
  language: Language
}) {
  const { data: visitors, error } = useQuery(telegramVisitorsQuery)
  const [search, setSearch] = useState('')
  const term = search.trim().toLowerCase()
  const shown = (visitors ?? []).filter(
    (visitor) =>
      !term ||
      `${nameOf(visitor)} ${detailOf(visitor)}`.toLowerCase().includes(term)
  )

  return (
    <SettingList
      head={
        <>
          <SettingBack
            onBack={onBack}
            backLabel={t('back', language)}
            title={t('telegram.visitors', language)}
            detail={
              visitors?.length ? visitorsTotal(visitors, language) : undefined
            }
          />
          {(visitors?.length ?? 0) > SEARCH_FROM && (
            <SearchField
              label={t('telegram.visitorsSearch', language)}
              value={search}
              onChange={setSearch}
              className="mb-3"
            />
          )}
        </>
      }
    >
      {!visitors ? (
        error ? (
          <SettingState role="alert">
            {t('telegram.visitorsFailed', language)}
          </SettingState>
        ) : (
          <PanelLoading className="flex-1" />
        )
      ) : shown.length === 0 ? (
        <SettingState role="status">
          {t(
            term ? 'telegram.visitorsNoMatch' : 'telegram.visitorsEmpty',
            language
          )}
        </SettingState>
      ) : (
        shown.map((visitor) => {
          const detail = detailOf(visitor)
          return (
            <SettingItem
              key={`${visitor.chat_id}:${visitor.sender_id}`}
              icon={<User className="size-5" />}
              title={<span title={nameOf(visitor)}>{nameOf(visitor)}</span>}
              badge={
                <>
                  <SettingBadge>
                    {t('telegram.visitorAttempts', language, {
                      count: visitor.attempts,
                    })}
                  </SettingBadge>
                  {visitor.chat_type !== 'private' && (
                    <SettingBadge>
                      {t(
                        visitor.chat_type === 'channel'
                          ? 'telegram.visitorChannel'
                          : 'telegram.visitorGroup',
                        language
                      )}
                    </SettingBadge>
                  )}
                </>
              }
              // Always a line, so every row is as tall.
              detail={<span title={detail}>{detail || ' '}</span>}
            >
              <span className="mono shrink-0 whitespace-nowrap text-xs text-vergex-muted">
                {fmtTime(visitor.last_at)}
              </span>
            </SettingItem>
          )
        })
      )}
    </SettingList>
  )
}
