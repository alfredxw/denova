import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AdaptiveSurface, type AdaptiveSurfaceControls } from '@/components/layout/adaptive-surface'
import {
  AgentChatActivitySidebar,
  AgentChatSidebarRail,
  type AgentChatActivitySidebarProps,
} from './AgentChatActivitySidebar'
import { persistSidebarVisible, readSidebarVisible } from './tab-state'

type SidebarProps = Omit<AgentChatActivitySidebarProps, 'onCollapse'>

interface AgentChatWorkspaceSurfaceProps {
  sidebarProps: SidebarProps
  secondaryPane: {
    content: ReactNode
    visible: boolean
    layoutKey: string
    onOpen: () => void
    onClose: () => void
  }
  createDisabled: boolean
  onCreateDefaultSession: () => void
  children: ReactNode | ((controls: AdaptiveSurfaceControls) => ReactNode)
}

/**
 * Owns the lightweight sidebar toggle state outside AgentChatView's live conversation tree.
 * This keeps a toggle from reconciling mounted chats and terminals, while the compact rail and
 * full project tree cross-fade inside one continuously sized desktop column.
 */
export function AgentChatWorkspaceSurface({
  sidebarProps,
  secondaryPane,
  createDisabled,
  onCreateDefaultSession,
  children,
}: AgentChatWorkspaceSurfaceProps) {
  const { t } = useTranslation()
  const [sidebarVisible, setSidebarVisible] = useState(readSidebarVisible)
  const collapseSidebar = useCallback(() => setSidebarVisible(false), [])
  const expandSidebar = useCallback(() => setSidebarVisible(true), [])

  useEffect(() => {
    persistSidebarVisible(sidebarVisible)
  }, [sidebarVisible])

  // These trees are intentionally stable across a local toggle. The full activity list may hold
  // many sortable rows, and rebuilding it on the same frame as the width transition causes jank.
  const sidebar = useMemo(
    () => <AgentChatActivitySidebar {...sidebarProps} onCollapse={collapseSidebar} />,
    [collapseSidebar, sidebarProps],
  )
  const rail = useMemo(
    () => (
      <AgentChatSidebarRail
        {...sidebarProps}
        onExpand={expandSidebar}
        onCreateDefaultSession={onCreateDefaultSession}
        createDisabled={createDisabled}
      />
    ),
    [createDisabled, expandSidebar, onCreateDefaultSession, sidebarProps],
  )

  return (
    <AdaptiveSurface
      className="h-full min-h-0"
      collapseAt={720}
      leftResize={{
        layoutKey: 'nova-agent-chat-activity-layout',
        label: t('layout.resize.sidebar'),
        defaultSize: '260px',
        minSize: '200px',
        maxSize: '36%',
        mainMinSize: '320px',
      }}
      left={{
        id: 'agent-chat-activity',
        side: 'left',
        title: t('agentChat.sidebar.projects'),
        content: sidebar,
        desktopClassName: 'h-full min-h-0 min-w-0',
        desktopVisible: sidebarVisible,
        desktopCollapsedSize: '40px',
        desktopCollapsedContent: rail,
      }}
      rightResize={{
        layoutKey: secondaryPane.layoutKey,
        label: t('agentChat.tabs.resizeSplit'),
        defaultSize: '50%',
        minSize: '280px',
        maxSize: '65%',
        mainMinSize: '280px',
      }}
      right={{
        id: 'agent-chat-secondary',
        side: 'right',
        title: t('agentChat.tabs.secondaryWorkspace'),
        content: secondaryPane.content,
        desktopClassName: 'h-full min-h-0 min-w-0',
        desktopVisible: secondaryPane.visible,
        onOpen: secondaryPane.onOpen,
        onClose: secondaryPane.onClose,
      }}
    >
      {children}
    </AdaptiveSurface>
  )
}
