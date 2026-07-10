import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

export interface BreadcrumbItem {
  id: string
  label: string
  selectable: boolean
}

interface BreadcrumbProps {
  items: BreadcrumbItem[]
  onSelect?: (id: string) => void
  className?: string
}

export function Breadcrumb({ items, onSelect, className }: BreadcrumbProps) {
  const { t } = useTranslation()
  if (items.length === 0) return null

  const displayItems = items

  return (
    <nav className={cn('flex items-center gap-0.5 text-xs', className)} aria-label={t('settingPanel.actorState.explorer.breadcrumb')}>
      {displayItems.map((item, index) => {
        const isLast = index === displayItems.length - 1
        const isSelectable = item.selectable && !isLast && onSelect

        return (
          <span key={`${item.id}-${index}`} className="flex items-center gap-0.5">
            {index > 0 ? (
              <ChevronRight className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" />
            ) : null}
            {isSelectable ? (
              <button
                type="button"
                className="truncate text-[var(--nova-text-muted)] transition-colors hover:text-[var(--nova-text)]"
                onClick={() => onSelect?.(item.id)}
              >
                {item.label}
              </button>
            ) : (
              <span
                className={cn(
                  'truncate',
                  isLast
                    ? 'font-medium text-[var(--nova-text)]'
                    : 'text-[var(--nova-text-faint)]',
                )}
              >
                {item.label}
              </span>
            )}
          </span>
        )
      })}
    </nav>
  )
}
