import { Maximize2, Minimize2, PanelRightClose, PanelRightOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { AgentChatNewTabMenuItems } from './AgentChatNewTabMenuItems'
import type {
  AgentChatGroupId,
  AgentChatPageId,
  TerminalCommandProfile,
  TerminalProfileId,
} from './types'

interface AgentChatSecondaryPaneControlProps {
  visible: boolean
  hasTabs: boolean
  expanded?: boolean
  onToggleExpanded?: () => void
  newChatDisabled?: boolean
  terminalCommands: TerminalCommandProfile[]
  pageIds: readonly AgentChatPageId[]
  onShow: () => void
  onHide: () => void
  onNewAgentTab: (group: AgentChatGroupId) => void
  onNewTerminalTab: (group: AgentChatGroupId, profileId: TerminalProfileId, profileName?: string) => void
  onOpenFiles: (group: AgentChatGroupId) => void
  onOpenPage: (group: AgentChatGroupId, pageId: AgentChatPageId) => void
}

/**
 * Split controls in the rightmost visible tab strip.
 *
 * An empty pane opens the same creation menu as New Tab. Once the pane owns tabs, the control
 * only changes visibility; closing tabs and stopping their runtimes remain explicit actions.
 */
export function AgentChatSecondaryPaneControl({
  visible,
  hasTabs,
  expanded = false,
  onToggleExpanded,
  newChatDisabled = false,
  terminalCommands,
  pageIds,
  onShow,
  onHide,
  onNewAgentTab,
  onNewTerminalTab,
  onOpenFiles,
  onOpenPage,
}: AgentChatSecondaryPaneControlProps) {
  const { t } = useTranslation()
  const label = t(visible ? 'agentChat.tabs.hideSecondary' : 'agentChat.tabs.showSecondary')
  const Icon = visible ? PanelRightClose : PanelRightOpen
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon-xs"
      className="h-7 w-8 shrink-0 rounded-lg"
      aria-label={label}
      aria-pressed={hasTabs ? visible : undefined}
      onClick={hasTabs ? (visible ? onHide : onShow) : undefined}
    >
      <Icon className="size-4" />
    </Button>
  )

  if (hasTabs) return (
    <>
      {visible && onToggleExpanded && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="h-7 w-8 shrink-0 rounded-lg"
          aria-label={t(expanded ? 'agentChat.tabs.restoreSecondary' : 'agentChat.tabs.expandSecondary')}
          title={t(expanded ? 'agentChat.tabs.restoreSecondary' : 'agentChat.tabs.expandSecondary')}
          aria-pressed={expanded}
          onClick={onToggleExpanded}
        >
          {expanded ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}
        </Button>
      )}
      {button}
    </>
  )

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{button}</DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-48">
        <AgentChatNewTabMenuItems
          group="secondary"
          newChatDisabled={newChatDisabled}
          terminalCommands={terminalCommands}
          pageIds={pageIds}
          onNewAgentTab={onNewAgentTab}
          onNewTerminalTab={onNewTerminalTab}
          onOpenFiles={onOpenFiles}
          onOpenPage={onOpenPage}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
