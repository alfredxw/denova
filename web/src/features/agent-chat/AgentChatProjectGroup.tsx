import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageSquareText } from 'lucide-react'
import type { AdaptiveSurfaceControls } from '@/components/layout/adaptive-surface'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { EmptyState } from '@/components/common/EmptyState'
import type { AgentChatProject } from './api'
import { AgentChatTabBar } from './AgentChatTabBar'
import { mountedAgentChatTabKey } from './use-agent-chat-tab-workbench'
import { otherTabIds, tabIdsAfter, tabsInGroup, type AgentChatProjectTabState } from './tab-state'
import type {
  AgentChatGroupId,
  AgentChatPageId,
  AgentChatTab,
  TerminalCommandProfile,
  TerminalProfileId,
} from './types'
import { agentChatPageIdsForProjectType } from './types'

export type AgentChatPaneControls = Pick<
  AdaptiveSurfaceControls,
  'isMobile' | 'openPaneId' | 'openLeft' | 'openRight' | 'closePane'
>

/** A desktop secondary pane is already hosted by the outer adaptive surface. */
export const DESKTOP_SECONDARY_PANE_CONTROLS: AgentChatPaneControls = {
  isMobile: false,
  openPaneId: null,
  openLeft: () => {},
  openRight: () => {},
  closePane: () => {},
}

interface AgentChatProjectGroupProps {
  project: AgentChatProject
  state: AgentChatProjectTabState
  group: AgentChatGroupId
  paneVisible: boolean
  mobileControls: AgentChatPaneControls
  mountedTabKeys: ReadonlySet<string>
  terminalCommands: TerminalCommandProfile[]
  secondaryControl: ReactNode
  tabTitle: (tab: AgentChatTab) => string
  renderTab: (tab: AgentChatTab, active: boolean) => ReactNode
  onFocus: (group: AgentChatGroupId) => void
  onActivate: (group: AgentChatGroupId, tabID: string) => void
  onClose: (tabIDs: string[]) => void
  onRename: (tabID: string, title: string) => void
  onTogglePin: (tabID: string) => void
  onMoveTab: (sourceID: string, group: AgentChatGroupId, beforeID: string | null) => void
  onNewAgentTab: (group: AgentChatGroupId) => void
  onNewTerminalTab: (
    group: AgentChatGroupId,
    profileID: TerminalProfileId,
    profileName?: string,
  ) => void
  onOpenFiles: (group: AgentChatGroupId) => void
  onOpenPage: (group: AgentChatGroupId, pageID: AgentChatPageId) => void
}

/** Renders one workbench pane without knowing the runtime owned by each tab kind. */
export function AgentChatProjectGroup({
  project,
  state,
  group,
  paneVisible,
  mobileControls,
  mountedTabKeys,
  terminalCommands,
  secondaryControl,
  tabTitle,
  renderTab,
  onFocus,
  onActivate,
  onClose,
  onRename,
  onTogglePin,
  onMoveTab,
  onNewAgentTab,
  onNewTerminalTab,
  onOpenFiles,
  onOpenPage,
}: AgentChatProjectGroupProps) {
  const { t } = useTranslation()
  const pageIds = agentChatPageIdsForProjectType(project.type)
  const groupTabs = tabsInGroup(state.tabs, group)
  const secondaryTabs = tabsInGroup(state.tabs, 'secondary')
  const activeID = state.activeTabIds[group]
  const openInGroup = (action: () => void, target: AgentChatGroupId) => {
    action()
    if (target === 'secondary' && mobileControls.isMobile) mobileControls.openRight()
  }
  const rightmostDesktopGroup = state.secondaryVisible && secondaryTabs.length > 0
    ? 'secondary'
    : 'primary'
  const tabBarEndActions = mobileControls.isMobile
    ? group === 'primary' ? secondaryControl : undefined
    : group === rightmostDesktopGroup
      ? <span data-slot="secondary-pane-control-spacer" aria-hidden="true" className="block h-7 w-8" />
      : undefined

  return (
    <div
      data-agent-chat-group={group}
      className="flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-bg)]"
      onPointerDownCapture={() => state.focusedGroup !== group && onFocus(group)}
      onFocusCapture={() => state.focusedGroup !== group && onFocus(group)}
    >
      <div className="flex items-center gap-1 bg-[var(--nova-surface)] pl-1.5 md:pl-0">
        {group === 'primary' && mobileControls.isMobile && (
          <MobilePaneTrigger
            side="left"
            className="size-7 shrink-0"
            label={t('agentChat.sidebar.projects')}
            onClick={mobileControls.openLeft}
          />
        )}
        <div className="min-w-0 flex-1">
          <AgentChatTabBar
            projectId={project.id}
            group={group}
            tabs={groupTabs}
            activeTabId={activeID}
            tabTitle={tabTitle}
            terminalCommands={terminalCommands}
            pageIds={pageIds}
            newChatDisabled={project.status !== 'available'}
            endActions={tabBarEndActions}
            onActivate={(tabID) => onActivate(group, tabID)}
            onClose={(tabID) => onClose([tabID])}
            onCloseOthers={(tabID) => onClose(otherTabIds(state.tabs, tabID))}
            onCloseToRight={(tabID) => onClose(tabIdsAfter(state.tabs, tabID))}
            onRename={onRename}
            onTogglePin={onTogglePin}
            onMoveTab={onMoveTab}
            onNewAgentTab={(target) => openInGroup(() => onNewAgentTab(target), target)}
            onNewTerminalTab={(target, profileID, profileName) => (
              openInGroup(() => onNewTerminalTab(target, profileID, profileName), target)
            )}
            onOpenFiles={(target) => openInGroup(() => onOpenFiles(target), target)}
            onOpenPage={(target, pageID) => openInGroup(() => onOpenPage(target, pageID), target)}
          />
        </div>
      </div>

      <div className="relative min-h-0 flex-1">
        {groupTabs.length === 0 ? (
          <EmptyState
            variant="page"
            icon={MessageSquareText}
            title={t('agentChat.empty.title')}
            description={t('agentChat.empty.description')}
            action={{ label: t('agentChat.tabs.newChat'), onClick: () => onNewAgentTab(group) }}
          />
        ) : groupTabs.map((tab) => {
          const active = paneVisible && tab.id === activeID
          const mounted = mountedTabKeys.has(mountedAgentChatTabKey(project.id, tab.id))
          if (!active && !mounted) return null
          return (
            <section key={tab.id} hidden={!active} aria-hidden={!active} className="absolute inset-0 flex min-h-0 flex-col">
              {renderTab(tab, active)}
            </section>
          )
        })}
      </div>
    </div>
  )
}
