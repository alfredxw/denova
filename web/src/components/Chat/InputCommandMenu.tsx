import { useEffect, useRef } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Command as CommandIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

export interface InputCommandOption {
  cmd: string
  description: string
  hint: string
  icon: LucideIcon
  source: 'builtin' | 'skill'
}

export interface IndexedInputCommandOption {
  command: InputCommandOption
  index: number
}

interface InputCommandMenuProps {
  open: boolean
  skillsOnly: boolean
  builtinCommands: IndexedInputCommandOption[]
  skillCommands: IndexedInputCommandOption[]
  activeIndex: number
  onActiveIndexChange: (index: number) => void
  onSelect: (command: InputCommandOption) => void
}

const GROUP_CLASS_NAME = 'p-0 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-0.5 [&_[cmdk-group-heading]]:pt-1 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-[var(--nova-text-faint)]'

/** Compact command palette shared by every Agent composer that supports slash commands. */
export function InputCommandMenu({
  open,
  skillsOnly,
  builtinCommands,
  skillCommands,
  activeIndex,
  onActiveIndexChange,
  onSelect,
}: InputCommandMenuProps) {
  const { t } = useTranslation()
  const itemRefs = useRef<Array<HTMLDivElement | null>>([])
  const commands = [...builtinCommands, ...skillCommands]
  const activeCommand = commands.find(({ index }) => index === activeIndex)?.command ?? commands[0]?.command

  useEffect(() => {
    if (!open) return
    itemRefs.current[activeIndex]?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex, commands.length, open])

  return (
    <Popover open={open && commands.length > 0}>
      <PopoverTrigger asChild>
        <span className="absolute bottom-full left-3 h-0 w-0" />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        side="top"
        className="nova-command-menu mb-1.5 w-[min(30rem,calc(100vw-1.5rem))] overflow-hidden rounded-xl border border-[var(--nova-border)] p-0 text-[var(--nova-text)]"
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <Command
          shouldFilter={false}
          value={activeCommand?.cmd}
          onValueChange={(value) => {
            const next = commands.find(({ command }) => command.cmd === value)
            if (next) onActiveIndexChange(next.index)
          }}
          className="bg-transparent"
        >
          <div className="flex h-9 min-w-0 items-center gap-2 border-b border-[var(--nova-border-soft)] px-2.5">
            <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
              <CommandIcon className="size-3.5" />
            </span>
            <span className="shrink-0 text-xs font-medium text-[var(--nova-text)]">
              {skillsOnly ? t('chat.commands.skillsGroup') : t('chat.commands.title')}
            </span>
            <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-faint)]">
              {skillsOnly ? t('chat.commands.skillsDescription') : t('chat.commands.description')}
            </span>
            <kbd className="shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--nova-text-faint)]">/</kbd>
          </div>

          <CommandList className="max-h-[min(20rem,45vh)] p-1">
            <CommandEmpty className="py-5 text-center text-xs text-[var(--nova-text-faint)]">
              {t('chat.commands.empty')}
            </CommandEmpty>
            {builtinCommands.length > 0 ? (
              <CommandGroup heading={t('chat.commands.group')} className={GROUP_CLASS_NAME}>
                {builtinCommands.map(({ command, index }) => renderCommandItem(command, index))}
              </CommandGroup>
            ) : null}
            {skillCommands.length > 0 ? (
              <CommandGroup
                heading={skillsOnly ? undefined : t('chat.commands.skillsGroup')}
                className={`${GROUP_CLASS_NAME} ${builtinCommands.length > 0 ? '[&_[cmdk-group-heading]]:pt-1.5' : ''}`}
              >
                {skillCommands.map(({ command, index }) => renderCommandItem(command, index))}
              </CommandGroup>
            ) : null}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )

  function renderCommandItem(command: InputCommandOption, index: number) {
    const { cmd, description, hint, icon: Icon, source } = command
    const active = index === activeIndex

    return (
      <CommandItem
        key={cmd}
        ref={(element) => { itemRefs.current[index] = element }}
        value={cmd}
        data-command-source={source}
        onMouseEnter={() => onActiveIndexChange(index)}
        onSelect={() => onSelect(command)}
        aria-label={[cmd, description, hint].join(' · ')}
        className={`group min-h-10 cursor-pointer gap-1.5 whitespace-nowrap rounded-lg border px-2 py-1 text-[var(--nova-text-muted)] sm:min-h-9 [&>svg:last-child]:hidden ${
          active
            ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]'
            : 'border-transparent hover:border-[var(--nova-border-soft)] hover:bg-[var(--nova-hover)]'
        }`}
      >
        <span className={`flex size-6 shrink-0 items-center justify-center rounded-md ${
          active ? 'bg-[var(--nova-surface-2)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)]'
        }`}>
          <Icon className="size-3.5" />
        </span>
        <span className="max-w-[42%] shrink-0 truncate font-mono text-xs font-medium text-[var(--nova-text)]" title={cmd}>
          {cmd}
        </span>
        <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text-muted)]" title={description}>
          {description}
        </span>
        <span className="ml-1 shrink-0 text-[11px] text-[var(--nova-text-faint)]">
          {hint}
        </span>
      </CommandItem>
    )
  }
}
