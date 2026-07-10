import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

const TYPE_STYLES: Record<string, string> = {
  number: 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
  string: 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
  bool: 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
  enum: 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
  object: 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
  list: 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
}

export function FieldTypeBadge({ type }: { type: string }) {
  const style = TYPE_STYLES[type] || 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]'

  return (
    <Badge
      variant="outline"
      className={cn(
        'h-5 rounded-full px-1.5 text-[10px] font-medium',
        style,
      )}
    >
      {type}
    </Badge>
  )
}
