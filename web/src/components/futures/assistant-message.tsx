import type { ReactNode } from 'react'

// What the assistant wrote, set as text. Models write Markdown, so the few forms they use
// are read here: headings, lists, quotes, tables, code, bold and inline code. It only ever
// makes text nodes, so nothing in a reply can become markup.

function inline(text: string): ReactNode[] {
  return text.split(/(\*\*[^*\n]+\*\*|`[^`\n]+`)/).map((part, index) => {
    if (part.startsWith('**') && part.endsWith('**') && part.length > 4)
      return (
        <strong key={index} className="font-medium">
          {part.slice(2, -2)}
        </strong>
      )
    if (part.startsWith('`') && part.endsWith('`') && part.length > 2)
      return (
        <code
          key={index}
          className="mono rounded bg-vergex-surface-muted px-1 text-xs"
        >
          {part.slice(1, -1)}
        </code>
      )
    return part
  })
}

const cells = (line: string) =>
  line
    .trim()
    .replace(/^\||\|$/g, '')
    .split('|')
    .map((cell) => cell.trim())

const isTableRule = (line: string) =>
  /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/.test(line)

function blocks(text: string): ReactNode[] {
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  const out: ReactNode[] = []
  let i = 0
  const collect = (matches: (line: string) => boolean) => {
    const found: string[] = []
    while (i < lines.length && matches(lines[i])) found.push(lines[i++])
    return found
  }
  while (i < lines.length) {
    const line = lines[i]
    const key = out.length
    if (!line.trim()) {
      i++
    } else if (line.trimStart().startsWith('```')) {
      i++
      const code = collect((next) => !next.trimStart().startsWith('```'))
      i++
      out.push(
        <pre
          key={key}
          className="mono overflow-x-auto rounded-md bg-vergex-surface-muted p-2 text-xs"
        >
          {code.join('\n')}
        </pre>
      )
    } else if (/^#{1,6}\s/.test(line)) {
      i++
      out.push(
        <h4 key={key} className="font-medium text-vergex-ink">
          {inline(line.replace(/^#{1,6}\s+/, ''))}
        </h4>
      )
    } else if (/^\s*[-*•]\s/.test(line)) {
      const items = collect((next) => /^\s*[-*•]\s/.test(next))
      out.push(
        <ul key={key} className="list-disc space-y-1 pl-4">
          {items.map((item, index) => (
            <li key={index}>{inline(item.replace(/^\s*[-*•]\s+/, ''))}</li>
          ))}
        </ul>
      )
    } else if (/^\s*\d+[.)]\s/.test(line)) {
      const items = collect((next) => /^\s*\d+[.)]\s/.test(next))
      out.push(
        <ol key={key} className="list-decimal space-y-1 pl-5">
          {items.map((item, index) => (
            <li key={index}>{inline(item.replace(/^\s*\d+[.)]\s+/, ''))}</li>
          ))}
        </ol>
      )
    } else if (line.trimStart().startsWith('>')) {
      const quoted = collect((next) => next.trimStart().startsWith('>'))
      out.push(
        <blockquote
          key={key}
          className="border-l-2 border-vergex-line-strong pl-3 text-vergex-muted"
        >
          {inline(quoted.map((q) => q.replace(/^\s*>\s?/, '')).join('\n'))}
        </blockquote>
      )
    } else if (line.includes('|') && isTableRule(lines[i + 1] ?? '')) {
      const head = cells(line)
      i += 2
      const rows = collect((next) => next.includes('|') && Boolean(next.trim()))
      out.push(
        <div key={key} className="overflow-x-auto">
          <table className="w-full border-separate border-spacing-0 text-left text-xs tabular-nums">
            <thead>
              <tr>
                {head.map((cell, index) => (
                  <th
                    key={index}
                    className="h-7 whitespace-nowrap border-b border-vergex-line px-2 py-0 font-medium text-vergex-muted first:pl-0"
                  >
                    {inline(cell)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, rowIndex) => (
                <tr key={rowIndex}>
                  {cells(row).map((cell, index) => (
                    <td
                      key={index}
                      className="border-b border-vergex-line px-2 py-1 first:pl-0"
                    >
                      {inline(cell)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )
    } else {
      const paragraph = collect(
        (next) =>
          Boolean(next.trim()) &&
          !/^(#{1,6}\s|\s*[-*•]\s|\s*\d+[.)]\s|\s*>|\s*```)/.test(next)
      )
      out.push(
        <p key={key} className="whitespace-pre-wrap">
          {inline(paragraph.join('\n'))}
        </p>
      )
    }
  }
  return out
}

export function AssistantMessage({ text }: { text: string }) {
  return (
    <div className="space-y-2 break-words text-[13px] leading-relaxed text-vergex-ink">
      {blocks(text)}
    </div>
  )
}
