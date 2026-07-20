import { Check, ChevronDown } from 'lucide-react'
import { Fragment, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

interface CompactResourcePickerProps<T> {
  items: T[]
  selectedId?: string
  getId: (item: T) => string
  getLabel: (item: T) => string
  label: string
  ariaLabel: string
  placeholder: string
  emptyLabel?: string
  layout?: 'inline' | 'sidebar'
  disabled?: boolean
  triggerClassName?: string
  contentClassName?: string
  trailingAction?: ReactNode
  /** Override a complete item row while preserving the shared picker chrome. */
  renderItem?: (item: T, context: CompactResourcePickerItemContext) => ReactNode
  renderFooter?: (close: () => void) => ReactNode
  onSelect: (id: string) => void
}

interface CompactResourcePickerItemContext {
  id: string
  label: string
  selected: boolean
  close: () => void
  select: () => void
}

/** Compact labeled listbox presentation shared by story-scoped resource selectors. */
export function CompactResourcePicker<T>({
  items,
  selectedId,
  getId,
  getLabel,
  label,
  ariaLabel,
  placeholder,
  emptyLabel = placeholder,
  layout = 'inline',
  disabled = false,
  triggerClassName,
  contentClassName,
  trailingAction,
  renderItem,
  renderFooter,
  onSelect,
}: CompactResourcePickerProps<T>) {
  const [open, setOpen] = useState(false)
  const sidebar = layout === 'sidebar'
  const selectedItem = items.find((item) => getId(item) === selectedId)
  const close = () => setOpen(false)

  const selector = (
    <Popover open={open} onOpenChange={(nextOpen) => setOpen(!disabled && nextOpen)}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          className={cn(
            'nova-field w-full min-w-0 justify-between px-3 py-0.5 text-xs font-normal text-[var(--nova-text)] focus:ring-0',
            triggerClassName,
          )}
          aria-label={ariaLabel}
          aria-expanded={open}
        >
          <span className="min-w-0 flex-1 truncate text-left">{selectedItem ? getLabel(selectedItem) : placeholder}</span>
          <ChevronDown data-icon="inline-end" className={cn('shrink-0 text-[var(--nova-text-faint)] transition-transform', open && 'rotate-180')} />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={6}
        collisionPadding={8}
        className={cn(
          'max-h-[min(70dvh,28rem)] overflow-y-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1 text-[var(--nova-text)] shadow-[var(--nova-shadow)]',
          sidebar
            ? 'w-[min(calc(100vw-2rem),24rem)]'
            : 'w-[var(--radix-popover-trigger-width)] max-w-[calc(100vw-2rem)]',
          contentClassName,
        )}
      >
        <div aria-label={ariaLabel} className="flex flex-col gap-1">
          {items.length === 0 ? (
            <div className="px-2 py-2 text-xs text-[var(--nova-text-faint)]">{emptyLabel}</div>
          ) : items.map((item) => {
            const id = getId(item)
            const itemLabel = getLabel(item)
            const selected = id === selectedId
            if (renderItem) {
              return (
                <Fragment key={id}>
                  {renderItem(item, {
                    id,
                    label: itemLabel,
                    selected,
                    close,
                    select: () => {
                      close()
                      onSelect(id)
                    },
                  })}
                </Fragment>
              )
            }
            return (
              <button
                key={id}
                type="button"
                aria-current={selected ? 'true' : undefined}
                className={cn(
                  'flex w-full min-w-0 items-center gap-2 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-xs leading-5',
                  selected
                    ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
                    : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
                )}
                onClick={() => {
                  close()
                  onSelect(id)
                }}
              >
                <span className="min-w-0 flex-1 truncate">{itemLabel}</span>
                {selected ? <Check className="size-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : null}
              </button>
            )
          })}
        </div>
        {renderFooter?.(close)}
      </PopoverContent>
    </Popover>
  )

  if (sidebar) {
    return (
      <div className="flex min-w-0 flex-col gap-1.5">
        <div className="flex items-center justify-between gap-2">
          <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{label}</span>
          {trailingAction}
        </div>
        <div className="min-w-0 w-full">{selector}</div>
      </div>
    )
  }

  return (
    <div data-layout="inline" className="flex w-full min-w-0 flex-wrap items-center gap-1.5 sm:w-auto sm:flex-nowrap">
      <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{label}</span>
      <div className="min-w-0 flex-1 basis-40 sm:w-[190px] sm:flex-none">{selector}</div>
      {trailingAction}
    </div>
  )
}
