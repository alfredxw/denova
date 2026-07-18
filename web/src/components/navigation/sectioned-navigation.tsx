import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface SectionedNavigationItem<TID extends string = string> {
  id: TID
  title: ReactNode
  description?: ReactNode
  icon?: LucideIcon
}

export interface SectionedNavigationGroup<TID extends string = string> {
  id: string
  title: ReactNode
  items: SectionedNavigationItem<TID>[]
}

interface SectionedNavigationProps<TID extends string> {
  groups: SectionedNavigationGroup<TID>[]
  activeId: TID
  onSelect: (id: TID) => void
  className?: string
  groupClassName?: string
  itemClassName?: string
}

/** Shared presentation for fixed or scroll-spy navigation; selection behavior stays with the caller. */
export function SectionedNavigation<TID extends string>({
  groups,
  activeId,
  onSelect,
  className,
  groupClassName,
  itemClassName,
}: SectionedNavigationProps<TID>) {
  return (
    <nav className={cn('flex flex-col gap-4', className)}>
      {groups.map((group) => (
        <div key={group.id} className={groupClassName}>
          <div className="mb-1.5 px-2 text-[11px] font-medium text-muted-foreground">{group.title}</div>
          <div className="flex flex-col gap-1">
            {group.items.map((item) => (
              <SectionedNavigationButton
                key={item.id}
                item={item}
                active={activeId === item.id}
                onSelect={onSelect}
                itemClassName={itemClassName}
              />
            ))}
          </div>
        </div>
      ))}
    </nav>
  )
}

function SectionedNavigationButton<TID extends string>({
  item,
  active,
  onSelect,
  itemClassName,
}: {
  item: SectionedNavigationItem<TID>
  active: boolean
  onSelect: (id: TID) => void
  itemClassName?: string
}) {
  const Icon = item.icon
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      onClick={() => onSelect(item.id)}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'nova-nav-item h-auto w-full justify-start gap-2 rounded-[var(--nova-radius)] px-2.5 py-1.5 text-left font-normal',
        active && 'is-active',
        itemClassName,
      )}
    >
      {Icon ? <Icon data-icon="inline-start" className="text-muted-foreground" /> : null}
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium text-foreground">{item.title}</span>
        {item.description ? <span className="block truncate text-[11px] text-muted-foreground">{item.description}</span> : null}
      </span>
    </Button>
  )
}
