import type { ButtonHTMLAttributes } from 'react'
import { ChevronDown, type LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

export const WORKBENCH_CONTEXT_MENU_CLASS = 'max-h-[min(32rem,calc(100vh-3.5rem))] w-[22rem] max-w-[calc(100vw-1rem)] gap-0 overflow-hidden border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)] ring-0'
export const WORKBENCH_CONTEXT_MENU_GROUP_CLASS = 'max-h-[min(21rem,calc(100vh-10rem))] overflow-y-auto px-1.5 pb-1.5'
export const WORKBENCH_CONTEXT_MENU_ITEM_CLASS = 'min-h-14 gap-2.5 border-b border-[var(--nova-border)] px-2 py-1.5 last:border-b-0 focus:bg-[var(--nova-hover)] focus:text-[var(--nova-text)]'

interface WorkbenchContextSwitcherTriggerProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  icon: LucideIcon
  label: string
  compact?: boolean
  iconOnly?: boolean
}

/** Shared workbench trigger for switching the active content context without changing modes. */
export function WorkbenchContextSwitcherTrigger({
  icon: Icon,
  label,
  compact = false,
  iconOnly = false,
  className,
  type = 'button',
  ...props
}: WorkbenchContextSwitcherTriggerProps) {
  return (
    <button
      type={type}
      data-slot="workbench-context-switcher-trigger"
      className={cn(
        'flex min-w-0 select-none items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] outline-none transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:ring-2 focus-visible:ring-[var(--nova-field-focus-border)] data-[state=open]:border-[var(--nova-field-focus-border)] data-[state=open]:bg-[var(--nova-active)] data-[state=open]:text-[var(--nova-text)]',
        iconOnly
          ? 'size-8 p-0'
          : compact
            ? 'h-8 max-w-[34vw] px-2 text-[11px]'
            : 'h-9 w-full max-w-none justify-start px-2.5 text-[11px]',
        className,
      )}
      {...props}
    >
      <Icon aria-hidden="true" className={cn('h-3.5 w-3.5 shrink-0', compact && !iconOnly && 'hidden')} />
      <span className={cn('min-w-0 truncate font-medium', iconOnly && 'sr-only')}>{label}</span>
      {!iconOnly && <ChevronDown aria-hidden="true" className="ml-auto h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
    </button>
  )
}
