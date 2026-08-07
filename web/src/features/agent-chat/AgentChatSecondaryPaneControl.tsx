import { Circle, CircleDot, PanelRightClose, PanelRightOpen } from 'lucide-react'
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
  busy: boolean
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
 * Persistent split affordance for the primary tab strip.
 *
 * An empty pane opens the same creation menu as New Tab. Once the pane owns tabs, the control
 * only changes visibility; closing tabs and stopping their runtimes remain explicit actions.
 */
export function AgentChatSecondaryPaneControl({
  visible,
  hasTabs,
  busy,
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
  const hiddenPopulated = hasTabs && !visible
  const hiddenBusy = hasTabs && !visible && busy
  const label = hiddenBusy
    ? t('agentChat.tabs.showSecondaryRunning')
    : t(visible ? 'agentChat.tabs.hideSecondary' : 'agentChat.tabs.showSecondary')
  const Icon = visible ? PanelRightClose : PanelRightOpen
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon-xs"
      className="relative h-7 w-8 shrink-0 rounded-lg"
      aria-label={label}
      aria-pressed={hasTabs ? visible : undefined}
      onClick={hasTabs ? (visible ? onHide : onShow) : undefined}
    >
      <Icon className="size-4" />
      {hiddenPopulated && !hiddenBusy ? (
        <Circle
          data-slot="secondary-pane-presence-indicator"
          aria-hidden="true"
          className="absolute right-1 top-1 size-1.5 fill-current text-[var(--nova-accent)]"
        />
      ) : null}
      {hiddenBusy ? (
        <CircleDot
          data-slot="secondary-pane-running-indicator"
          aria-hidden="true"
          className="absolute right-0.5 top-0.5 size-2.5 fill-[var(--nova-warning-bg)] text-[var(--nova-warning)]"
        />
      ) : null}
    </Button>
  )

  if (hasTabs) return button

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
