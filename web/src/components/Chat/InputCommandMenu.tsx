import { useEffect, useRef } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Command as CommandIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CommandGroup, CommandItem } from '@/components/ui/command'
import { INPUT_SUGGESTION_GROUP_CLASS_NAME, InputSuggestionMenu, inputSuggestionItemClassName } from './InputSuggestionMenu'

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
    <InputSuggestionMenu
      open={open && commands.length > 0}
      value={activeCommand?.cmd}
      onValueChange={(value) => {
        const next = commands.find(({ command }) => command.cmd === value)
        if (next) onActiveIndexChange(next.index)
      }}
      icon={CommandIcon}
      title={skillsOnly ? t('chat.commands.skillsGroup') : t('chat.commands.title')}
      description={skillsOnly ? t('chat.commands.skillsDescription') : t('chat.commands.description')}
      shortcut="/"
      emptyText={t('chat.commands.empty')}
    >
      {builtinCommands.length > 0 ? (
        <CommandGroup heading={t('chat.commands.group')} className={INPUT_SUGGESTION_GROUP_CLASS_NAME}>
          {builtinCommands.map(({ command, index }) => renderCommandItem(command, index))}
        </CommandGroup>
      ) : null}
      {skillCommands.length > 0 ? (
        <CommandGroup
          heading={skillsOnly ? undefined : t('chat.commands.skillsGroup')}
          className={`${INPUT_SUGGESTION_GROUP_CLASS_NAME} ${builtinCommands.length > 0 ? '[&_[cmdk-group-heading]]:pt-1.5' : ''}`}
        >
          {skillCommands.map(({ command, index }) => renderCommandItem(command, index))}
        </CommandGroup>
      ) : null}
    </InputSuggestionMenu>
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
        className={inputSuggestionItemClassName(active)}
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
