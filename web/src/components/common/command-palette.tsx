import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Bot,
  History,
  Save,
  Search,
  Sparkles,
  X,
} from 'lucide-react'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from '@/components/ui/command'

type CommandAction = {
  id: string
  label: string
  shortcut?: string
  disabled?: boolean
  icon: ReactNode
  onSelect: () => void
}

interface CommandPaletteProps {
  open: boolean
  isStreaming?: boolean
  onOpenChange: (open: boolean) => void
  onSave: () => void
  onOpenAgent: () => void
  onOpenVersions: () => void
  onOpenSearch: () => void
  onContinueWriting: () => void
  onToggleRightPanel: () => void
}

/** 工作台命令面板，集中承载全局快捷操作，不直接调用业务 API。 */
export function CommandPalette({
  open,
  isStreaming = false,
  onOpenChange,
  onSave,
  onOpenAgent,
  onOpenVersions,
  onOpenSearch,
  onContinueWriting,
  onToggleRightPanel,
}: CommandPaletteProps) {
  const { t } = useTranslation()
  const actions: CommandAction[] = [
    {
      id: 'save',
      label: t('command.saveChapter'),
      shortcut: '⌘S',
      icon: <Save className="h-4 w-4" />,
      onSelect: onSave,
    },
    {
      id: 'agent',
      label: t('command.openAgent'),
      icon: <Bot className="h-4 w-4" />,
      onSelect: onOpenAgent,
    },
    {
      id: 'versions',
      label: t('command.openVersions'),
      icon: <History className="h-4 w-4" />,
      onSelect: onOpenVersions,
    },
    {
      id: 'search',
      label: t('command.openSearch'),
      shortcut: '⌘⇧F',
      icon: <Search className="h-4 w-4" />,
      onSelect: onOpenSearch,
    },
    {
      id: 'continue',
      label: t('command.continueWriting'),
      shortcut: '⌘↵',
      disabled: isStreaming,
      icon: <Sparkles className="h-4 w-4" />,
      onSelect: onContinueWriting,
    },
    {
      id: 'toggle-right-panel',
      label: t('command.toggleRightPanel'),
      shortcut: '⌘⌥B / CtrlAltB',
      icon: <X className="h-4 w-4" />,
      onSelect: onToggleRightPanel,
    },
  ]

  const runAction = (action: CommandAction) => {
    if (action.disabled) return
    action.onSelect()
    onOpenChange(false)
  }

  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('command.title')}
      description={t('command.description')}
      className="border-[var(--nova-border)] bg-[var(--nova-menu-bg)] text-[var(--nova-text)]"
    >
      <CommandInput
        placeholder={t('command.placeholder')}
        className="text-[var(--nova-text)] placeholder:text-[var(--nova-text-faint)]"
      />
      <CommandList className="max-h-[360px]">
        <CommandEmpty>{t('command.empty')}</CommandEmpty>
        <CommandGroup heading={t('command.group.workbench')}>
          {actions.map((action) => (
            <CommandItem
              key={action.id}
              value={action.label}
              disabled={action.disabled}
              onSelect={() => runAction(action)}
              className="cursor-pointer text-[var(--nova-text)] data-[selected=true]:bg-[var(--nova-menu-item-hover-bg)]"
            >
              {action.icon}
              <span>{action.label}</span>
              {action.shortcut && <CommandShortcut>{action.shortcut}</CommandShortcut>}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}
