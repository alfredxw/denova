import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageSquareText } from 'lucide-react'
import type { AdaptiveSurfaceControls } from '@/components/layout/adaptive-surface'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { EmptyState } from '@/components/common/EmptyState'
import type { AgentChatProject } from './api'
import { AgentChatSecondaryPaneControl } from './AgentChatSecondaryPaneControl'
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
  secondaryBusy: boolean
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
    command?: string,
  ) => void
  onOpenFiles: (group: AgentChatGroupId) => void
  onOpenPage: (group: AgentChatGroupId, pageID: AgentChatPageId) => void
  onShowSecondary: () => void
  onHideSecondary: () => void
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
  secondaryBusy,
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
  onShowSecondary,
  onHideSecondary,
}: AgentChatProjectGroupProps) {
  const { t } = useTranslation()
  const groupTabs = tabsInGroup(state.tabs, group)
  const secondaryTabs = tabsInGroup(state.tabs, 'secondary')
  const activeID = state.activeTabIds[group]
  const openInGroup = (action: () => void, target: AgentChatGroupId) => {
    action()
    if (target === 'secondary' && mobileControls.isMobile) mobileControls.openRight()
  }
  const secondaryControl = (
    <AgentChatSecondaryPaneControl
      visible={secondaryTabs.length > 0 && (
        mobileControls.isMobile
          ? mobileControls.openPaneId === 'agent-chat-secondary'
          : state.secondaryVisible
      )}
      hasTabs={secondaryTabs.length > 0}
      busy={secondaryBusy}
      newChatDisabled={project.status !== 'available'}
      terminalCommands={terminalCommands}
      pagesEnabled={project.type === 'book'}
      onShow={() => (mobileControls.isMobile ? mobileControls.openRight() : onShowSecondary())}
      onHide={() => (mobileControls.isMobile ? mobileControls.closePane() : onHideSecondary())}
      onNewAgentTab={(target) => openInGroup(() => onNewAgentTab(target), target)}
      onNewTerminalTab={(target, profileID, profileName) => (
        openInGroup(() => onNewTerminalTab(target, profileID, profileName), target)
      )}
      onOpenFiles={(target) => openInGroup(() => onOpenFiles(target), target)}
      onOpenPage={(target, pageID) => openInGroup(() => onOpenPage(target, pageID), target)}
    />
  )
  const primaryOwnsSecondaryControl = group === 'primary'
    && (mobileControls.isMobile || !state.secondaryVisible || secondaryTabs.length === 0)
  const secondaryOwnsSecondaryControl = group === 'secondary' && paneVisible
  const tabBarEndActions = primaryOwnsSecondaryControl
    ? secondaryControl
    : secondaryOwnsSecondaryControl
      ? <div className="hidden lg:flex">{secondaryControl}</div>
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
            pagesEnabled={project.type === 'book'}
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
            onNewTerminalTab={(target, profileID, profileName, command) => (
              openInGroup(() => onNewTerminalTab(target, profileID, profileName, command), target)
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
