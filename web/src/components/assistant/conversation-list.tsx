import { useMemo, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { confirmToast } from '@/components/common/notify'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { SearchField } from '@/components/common/search-field'
import { RailItem } from '@/components/common/split-view'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { fmtTime } from '@/utils/trader-format'
import type { Assistant } from './use-assistant'

// Earlier conversations, the window's other view. The search box is always there, so the
// view looks the same however many there are.
export function ConversationList({
  assistant,
  onOpened,
}: {
  assistant: Assistant
  onOpened: () => void
}) {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`assistant.${key}`, language, params)
  const [search, setSearch] = useState('')
  const { conversations, conversationId } = assistant
  const rows = useMemo(() => {
    const term = search.trim().toLowerCase()
    return (conversations.data ?? []).filter(
      (row) => !term || row.title.toLowerCase().includes(term)
    )
  }, [conversations.data, search])

  const remove = async (id: string, title: string) => {
    const yes = await confirmToast(label('deleteMessage', { title }), {
      title: label('deleteTitle'),
      okText: label('delete'),
      cancelText: label('cancel'),
      tone: 'danger',
    })
    if (yes) await assistant.deleteConversation(id)
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="shrink-0 p-2">
        <SearchField
          label={label('search')}
          value={search}
          onChange={setSearch}
        />
      </div>
      <div className="min-h-0 flex-1">
        {conversations.isError && !conversations.data ? (
          <PanelState role="alert" className="h-full">
            <span>{label('listFailed')}</span>
            <Button
              type="button"
              size="sm"
              onClick={() => void conversations.refetch()}
            >
              {label('retry')}
            </Button>
          </PanelState>
        ) : !conversations.data ? (
          <PanelLoading />
        ) : rows.length === 0 ? (
          <PanelState className="h-full">
            {label(search.trim() ? 'noMatches' : 'noConversations')}
          </PanelState>
        ) : (
          <ScrollArea
            className="h-full px-2 pb-2"
            viewportClassName="overscroll-contain"
          >
            <ul className="space-y-0.5">
              {rows.map((row) => (
                <RailItem
                  key={row.id}
                  current={row.id === conversationId}
                  title={row.title}
                  name={row.title}
                  details={
                    <time className="mono text-xs font-normal text-vergex-muted">
                      {fmtTime(row.updated_at)}
                    </time>
                  }
                  onOpen={() => {
                    if (assistant.openConversation(row.id)) onOpened()
                  }}
                  actionsLabel={label('conversationActions')}
                  actions={[
                    {
                      label: label('delete'),
                      icon: Trash2,
                      danger: true,
                      onSelect: () => void remove(row.id, row.title),
                    },
                  ]}
                />
              ))}
            </ul>
          </ScrollArea>
        )}
      </div>
    </div>
  )
}
