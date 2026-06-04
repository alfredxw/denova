import type { ReactNode } from 'react'
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
  onClosePanels: () => void
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
  onClosePanels,
}: CommandPaletteProps) {
  const actions: CommandAction[] = [
    {
      id: 'save',
      label: '保存当前章节',
      shortcut: '⌘S',
      icon: <Save className="h-4 w-4" />,
      onSelect: onSave,
    },
    {
      id: 'agent',
      label: '打开创作Agent',
      icon: <Bot className="h-4 w-4" />,
      onSelect: onOpenAgent,
    },
    {
      id: 'versions',
      label: '打开版本管理',
      icon: <History className="h-4 w-4" />,
      onSelect: onOpenVersions,
    },
    {
      id: 'search',
      label: '打开全局搜索',
      shortcut: '⌘⇧F',
      icon: <Search className="h-4 w-4" />,
      onSelect: onOpenSearch,
    },
    {
      id: 'continue',
      label: '继续写作',
      shortcut: '⌘↵',
      disabled: isStreaming,
      icon: <Sparkles className="h-4 w-4" />,
      onSelect: onContinueWriting,
    },
    {
      id: 'close',
      label: '关闭面板',
      shortcut: 'Esc',
      icon: <X className="h-4 w-4" />,
      onSelect: onClosePanels,
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
      title="命令面板"
      description="搜索并执行工作台命令"
      className="border-[#303238] bg-[#202124] text-[#d7dbe2]"
    >
      <CommandInput
        placeholder="搜索命令..."
        className="text-[#d7dbe2] placeholder:text-[#666d78]"
      />
      <CommandList className="max-h-[360px]">
        <CommandEmpty>未找到匹配命令</CommandEmpty>
        <CommandGroup heading="工作台">
          {actions.map((action) => (
            <CommandItem
              key={action.id}
              value={action.label}
              disabled={action.disabled}
              onSelect={() => runAction(action)}
              className="cursor-pointer text-[#d7dbe2] data-[selected=true]:bg-[#303238]"
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
