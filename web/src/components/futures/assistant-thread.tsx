import { useEffect, useRef, type MutableRefObject } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AssistantMessage } from './assistant-message'
import { FuturesActivity, type ActivityItem } from './futures-activity'
import type { ChatMessage } from './use-futures-assistant'

interface AssistantThreadProps {
  messages: ChatMessage[]
  trace: ActivityItem[]
  futuresBusy: boolean
  // Whether the thread keeps scrolling to the newest message.
  followRef: MutableRefObject<boolean>
}

export function Message({ message }: { message: ChatMessage }) {
  if (message.role === 'user')
    return (
      <div className="max-w-[85%] self-end whitespace-pre-wrap break-words rounded-2xl bg-vergex-ink-tint px-3 py-2 text-[13px] leading-relaxed text-vergex-ink">
        {message.content}
      </div>
    )
  if (message.role === 'error')
    return (
      <p className="whitespace-pre-wrap break-words text-xs leading-relaxed text-vergex-danger">
        {message.content}
      </p>
    )
  return <AssistantMessage text={message.content} />
}

// AssistantThread is the assistant conversation. What was asked reads on the right, what the
// model wrote runs the width of the column as text, and how the current turn got there sits
// between the two. It follows new messages unless the reader scrolled up.
export function AssistantThread({
  messages,
  trace,
  futuresBusy,
  followRef,
}: AssistantThreadProps) {
  const box = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (box.current && followRef.current)
      box.current.scrollTop =
        messages.length || trace.length ? box.current.scrollHeight : 0
  }, [messages, trace])
  const shown = messages.filter(
    (m, i) =>
      m.role !== 'user' ||
      i === 0 ||
      messages[i - 1].role !== 'user' ||
      messages[i - 1].content !== m.content
  )
  // The steps belong to the latest question, before the answer they led to.
  const answerAt = shown.map((m) => m.role).lastIndexOf('user') + 1
  return (
    <ScrollArea
      className="h-full"
      viewportRef={box}
      onViewportScroll={() => {
        if (box.current)
          followRef.current =
            box.current.scrollHeight -
              box.current.scrollTop -
              box.current.clientHeight <
            80
      }}
    >
      <div className="flex flex-col gap-3 px-4 py-3" id="futures-chat-panel">
        {shown.slice(0, answerAt).map((message, index) => (
          <Message key={index} message={message} />
        ))}
        <FuturesActivity items={trace} busy={futuresBusy} />
        {shown.slice(answerAt).map((message, index) => (
          <Message key={answerAt + index} message={message} />
        ))}
      </div>
    </ScrollArea>
  )
}
