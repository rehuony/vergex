import {
  useEffect,
  useRef,
  type DependencyList,
  type MutableRefObject,
  type ReactNode,
} from 'react'
import { MarkdownText } from '@/components/common/markdown-text'
import { ScrollArea } from '@/components/ui/scroll-area'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

interface ChatThreadProps<M extends ChatMessage> {
  messages: M[]
  // How the current turn got there, between the latest question and its answer.
  steps: ReactNode
  // The reply as the model writes it.
  streaming: string
  // What else moves the thread on, such as a new step.
  changes: DependencyList
  // What comes after everything said, such as what waits for an answer.
  after?: ReactNode
  // Whether the thread keeps scrolling to the newest message.
  followRef: MutableRefObject<boolean>
  // How a reply of the surface's own kind reads, where it is one.
  renderReply?: (message: M) => ReactNode
}

export function Message<M extends ChatMessage>({
  message,
  renderReply,
}: {
  message: M
  renderReply?: (message: M) => ReactNode
}) {
  if (message.role === 'user')
    return (
      <div className="max-w-[85%] self-end whitespace-pre-wrap break-words rounded-2xl bg-vergex-ink-tint px-3 py-2 text-[13px] leading-relaxed text-vergex-ink">
        {message.content}
      </div>
    )
  return renderReply?.(message) ?? <MarkdownText text={message.content} />
}

// ChatThread is a conversation with a model. What was asked reads on the right, what the
// model wrote runs the width of the column as text, and how the current turn got there sits
// between the two. It follows new messages unless the reader scrolled up.
export function ChatThread<M extends ChatMessage>({
  messages,
  steps,
  streaming,
  changes,
  after,
  followRef,
  renderReply,
}: ChatThreadProps<M>) {
  const box = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (box.current && followRef.current)
      box.current.scrollTop = box.current.scrollHeight
  }, [messages, streaming, ...changes])
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
      viewportClassName="overscroll-contain"
      onViewportScroll={() => {
        if (box.current)
          followRef.current =
            box.current.scrollHeight -
              box.current.scrollTop -
              box.current.clientHeight <
            80
      }}
    >
      <div className="flex flex-col gap-3 px-4 py-3">
        {shown.slice(0, answerAt).map((message, index) => (
          <Message key={index} message={message} renderReply={renderReply} />
        ))}
        {steps}
        {shown.slice(answerAt).map((message, index) => (
          <Message
            key={answerAt + index}
            message={message}
            renderReply={renderReply}
          />
        ))}
        {streaming && <MarkdownText text={streaming} />}
        {after}
      </div>
    </ScrollArea>
  )
}
