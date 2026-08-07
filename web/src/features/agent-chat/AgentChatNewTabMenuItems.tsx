import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Bot,
  Clock3,
  Database,
  FolderTree,
  History,
  MessageSquareText,
  PenLine,
  SlidersHorizontal,
  Sparkles,
  TerminalSquare,
} from 'lucide-react'
import { DropdownMenuItem, DropdownMenuSeparator } from '@/components/ui/dropdown-menu'
import {
  type AgentChatGroupId,
  type AgentChatPageId,
  type TerminalCommandProfile,
  type TerminalProfileId,
} from './types'

/** Every project page uses the same icon in tabs and in creation menus. */
export const AGENT_CHAT_PAGE_ICONS: Record<AgentChatPageId, ReactNode> = {
  reader: <PenLine className="size-3.5" />,
  lore: <Database className="size-3.5" />,
  presets: <SlidersHorizontal className="size-3.5" />,
  skills: <Sparkles className="size-3.5" />,
  agents: <Bot className="size-3.5" />,
  automations: <Clock3 className="size-3.5" />,
  versions: <History className="size-3.5" />,
}

interface AgentChatNewTabMenuItemsProps {
  group: AgentChatGroupId
  newChatDisabled?: boolean
  terminalCommands: TerminalCommandProfile[]
  pageIds: readonly AgentChatPageId[]
  onNewAgentTab: (group: AgentChatGroupId) => void
  onNewTerminalTab: (group: AgentChatGroupId, profileId: TerminalProfileId, profileName?: string) => void
  onOpenFiles: (group: AgentChatGroupId) => void
  onOpenPage: (group: AgentChatGroupId, pageId: AgentChatPageId) => void
}

/** Shared creation choices keep the ordinary New Tab and secondary-pane launchers consistent. */
export function AgentChatNewTabMenuItems({
  group,
  newChatDisabled = false,
  terminalCommands,
  pageIds,
  onNewAgentTab,
  onNewTerminalTab,
  onOpenFiles,
  onOpenPage,
}: AgentChatNewTabMenuItemsProps) {
  const { t } = useTranslation()
  return (
    <>
      <DropdownMenuItem disabled={newChatDisabled} onSelect={() => onNewAgentTab(group)}>
        <MessageSquareText />
        {t('agentChat.tabs.newChat')}
      </DropdownMenuItem>
      <DropdownMenuItem onSelect={() => onNewTerminalTab(group, 'shell')}>
        <TerminalSquare />
        {t('agentChat.tabs.newTerminal')}
      </DropdownMenuItem>
      {terminalCommands.map((command) => (
        <DropdownMenuItem key={command.id} onSelect={() => onNewTerminalTab(group, command.id, command.name)}>
          <TerminalSquare />
          {command.name}
        </DropdownMenuItem>
      ))}
      <DropdownMenuSeparator />
      {pageIds.length > 0
        ? pageIds.map((pageId) => (
            <DropdownMenuItem key={pageId} onSelect={() => onOpenPage(group, pageId)}>
              {AGENT_CHAT_PAGE_ICONS[pageId]}
              {t(`agentChat.page.${pageId}`)}
            </DropdownMenuItem>
          ))
        : null}
      <DropdownMenuItem onSelect={() => onOpenFiles(group)}>
        <FolderTree />
        {t('files.title')}
      </DropdownMenuItem>
    </>
  )
}
