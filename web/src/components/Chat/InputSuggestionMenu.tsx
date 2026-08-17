import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Command, CommandEmpty, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

interface InputSuggestionMenuProps {
  open: boolean
  value?: string
  onValueChange?: (value: string) => void
  icon: LucideIcon
  title: string
  description: string
  shortcut: string
  emptyText: string
  children: ReactNode
}

export const INPUT_SUGGESTION_GROUP_CLASS_NAME = 'p-0 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-0.5 [&_[cmdk-group-heading]]:pt-1 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-[var(--nova-text-faint)]'

/** Shared compact shell for inline composer suggestions such as commands and references. */
export function InputSuggestionMenu({
  open,
  value,
  onValueChange,
  icon: Icon,
  title,
  description,
  shortcut,
  emptyText,
  children,
}: InputSuggestionMenuProps) {
  return (
    <Popover open={open}>
      <PopoverTrigger asChild>
        <span className="absolute bottom-full left-3 h-0 w-0" />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        side="top"
        className="nova-command-menu mb-1.5 w-[min(30rem,calc(100vw-1.5rem))] overflow-hidden rounded-xl border border-[var(--nova-border)] p-0 text-[var(--nova-text)]"
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <Command shouldFilter={false} value={value} onValueChange={onValueChange} className="bg-transparent">
          <div className="flex h-9 min-w-0 items-center gap-2 border-b border-[var(--nova-border-soft)] px-2.5">
            <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
              <Icon className="size-3.5" />
            </span>
            <span className="shrink-0 text-xs font-medium text-[var(--nova-text)]">{title}</span>
            <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-faint)]">{description}</span>
            <kbd className="shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--nova-text-faint)]">{shortcut}</kbd>
          </div>

          <CommandList className="max-h-[min(20rem,45vh)] p-1">
            <CommandEmpty className="py-5 text-center text-xs text-[var(--nova-text-faint)]">{emptyText}</CommandEmpty>
            {children}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

export function inputSuggestionItemClassName(active: boolean) {
  return `group min-h-10 cursor-pointer gap-1.5 whitespace-nowrap rounded-lg border px-2 py-1 text-[var(--nova-text-muted)] sm:min-h-9 [&>svg:last-child]:hidden ${
    active
      ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]'
      : 'border-transparent hover:border-[var(--nova-border-soft)] hover:bg-[var(--nova-hover)]'
  }`
}
