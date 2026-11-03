import { ArrowRight } from 'lucide-react'
import { Link } from 'react-router-dom'
import type { AssistantLink } from '@/api/assistant'
import { Button } from '@/components/ui/button'

// Where a reply points, on the right under it. They go nowhere until pressed, and only
// within the app: the server names them, and this holds it to that.
export function ReplyLinks({
  links,
  onFollow,
}: {
  links: AssistantLink[]
  onFollow: () => void
}) {
  // A path in the app, or something opened over the page, such as Settings.
  const inside = links.filter(
    (link) =>
      link.to.startsWith('?') ||
      (link.to.startsWith('/') && !link.to.startsWith('//'))
  )
  if (inside.length === 0) return null
  return (
    <div className="flex flex-wrap justify-end gap-2">
      {inside.map((link) => (
        <Button
          key={link.to}
          asChild
          size="sm"
          className="max-w-full max-sm:h-10"
        >
          <Link to={link.to} onClick={onFollow}>
            <span className="truncate">{link.label}</span>
            <ArrowRight className="size-4" aria-hidden="true" />
          </Link>
        </Button>
      ))}
    </div>
  )
}
