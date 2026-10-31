import { Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { dialogInputClass } from '@/components/common/dialog'
import { cn } from '@/utils/cn'

interface FAQSearchBarProps {
  searchTerm: string
  onSearchChange: (value: string) => void
  placeholder: string
  clearLabel: string
}

export function FAQSearchBar({
  searchTerm,
  onSearchChange,
  placeholder,
  clearLabel,
}: FAQSearchBarProps) {
  return (
    <div className="group relative">
      <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-vergex-text-muted transition-colors group-focus-within:text-vergex-brand" />
      <input
        type="text"
        value={searchTerm}
        onChange={(e) => onSearchChange(e.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
        className={cn(dialogInputClass, 'pl-9 pr-9')}
      />
      {searchTerm && (
        <Button
          variant="ghost"
          size="icon"
          className="absolute right-1 top-1/2 size-8 -translate-y-1/2 text-vergex-text-muted"
          aria-label={clearLabel}
          onClick={() => onSearchChange('')}
        >
          <X className="size-4" />
        </Button>
      )}
    </div>
  )
}
