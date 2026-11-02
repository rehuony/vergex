import { ScrollArea } from '@/components/ui/scroll-area'

// A long text such as a prompt, folded away under its title and scrolling
// inside a capped box once opened. Nothing shows when there is no text.
export function StoredText({ title, text }: { title: string; text?: string }) {
  if (!text) return null
  return (
    <details className="border-t border-vergex-line py-3">
      <summary className="text-[13px] font-medium text-vergex-ink">
        {title}
      </summary>
      <ScrollArea className="mt-2 max-h-80 rounded-md bg-vergex-surface-muted">
        <pre className="whitespace-pre-wrap break-words p-3 font-mono text-xs text-vergex-ink">
          {text}
        </pre>
      </ScrollArea>
    </details>
  )
}
