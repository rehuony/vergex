import type { ReactNode, RefObject } from 'react'

// What a conversation's next message is written in, with its controls in a row under the
// text. Enter sends, Shift+Enter breaks the line, and a word being composed is never sent.
export function ChatComposer({
  label,
  placeholder,
  value,
  onChange,
  onSubmit,
  disabled,
  inputRef,
  controls,
}: {
  label: string
  placeholder: string
  value: string
  onChange: (value: string) => void
  onSubmit: () => void
  disabled?: boolean
  inputRef?: RefObject<HTMLTextAreaElement>
  controls: ReactNode
}) {
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        onSubmit()
      }}
    >
      <div className="rounded-md border border-vergex-line transition-colors focus-within:border-vergex-ink/40">
        <textarea
          ref={inputRef}
          aria-label={label}
          className="block max-h-40 min-h-10 w-full resize-none border-0 bg-transparent px-3 py-2 text-[13px] leading-relaxed text-vergex-ink shadow-none outline-none [field-sizing:content] placeholder:text-vergex-faint focus:bg-transparent focus:shadow-none max-sm:text-base"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          maxLength={8000}
          disabled={disabled}
          onKeyDown={(event) => {
            // Enter while composing Chinese picks a word; it does not send.
            if (
              event.key === 'Enter' &&
              !event.shiftKey &&
              !event.nativeEvent.isComposing
            ) {
              event.preventDefault()
              onSubmit()
            }
          }}
        />
        <div className="flex items-center gap-2 p-1.5">{controls}</div>
      </div>
    </form>
  )
}
