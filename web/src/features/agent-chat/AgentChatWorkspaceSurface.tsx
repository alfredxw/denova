import { ResourceWorkspace } from '@/components/layout/resource-workspace'
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { type AdaptiveSurfaceControls } from '@/components/layout/adaptive-surface'
import {
  AgentChatActivitySidebar,
  type AgentChatActivitySidebarProps,
} from './AgentChatActivitySidebar'
import { AgentChatSidebarToggle } from './AgentChatSidebarToggle'
import './workspace-surface.css'
import { persistSidebarVisible, readSidebarVisible } from './tab-state'

type SidebarProps = AgentChatActivitySidebarProps

interface AgentChatWorkspaceSurfaceProps {
  sidebarProps: SidebarProps
  secondaryPane: {
    available: boolean
    focused: boolean
    onFocus: (focused: boolean) => void
    content: ReactNode
    visible: boolean
    expanded: boolean
    layoutKey: string
    onOpen: () => void
    onClose: () => void
  }
  children: ReactNode | ((controls: AdaptiveSurfaceControls) => ReactNode)
}

/** Keeps sidebar interaction state outside the live conversation tree. */
export function AgentChatWorkspaceSurface({
  sidebarProps,
  secondaryPane,
  children,
}: AgentChatWorkspaceSurfaceProps) {
  const { t } = useTranslation()
  const [sidebarVisible, setSidebarVisible] = useState(readSidebarVisible)
  const toggleSidebar = useCallback(() => setSidebarVisible((visible) => !visible), [])

  useEffect(() => {
    persistSidebarVisible(sidebarVisible)
  }, [sidebarVisible])

  const sidebar = useMemo(
    () => <AgentChatActivitySidebar {...sidebarProps} />,
    [sidebarProps],
  )

  return (
    <div className="h-full min-h-0" data-agent-chat-surface data-sidebar-visible={sidebarVisible} data-secondary-expanded={secondaryPane.expanded && secondaryPane.visible}>
      <ResourceWorkspace
        title={t('workbench.activity.agentchat')}
        secondaryView={{ available: secondaryPane.available, open: secondaryPane.focused, onOpenChange: secondaryPane.onFocus, returnToContentOnSelection: false, label: t('workbench.mobile.secondary') }}
        contentViews={{ value: 'content', items: [{ value: 'content', label: t('workbench.mobile.primary') }], onValueChange: () => {} }}
        className="h-full min-h-0"
        collapseAt={720}
        desktopOverlay={<AgentChatSidebarToggle visible={sidebarVisible} onToggle={toggleSidebar} tree={sidebarProps} />}
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
        }}
        rightExpanded={secondaryPane.expanded}
        rightResize={{
          layoutKey: secondaryPane.layoutKey,
          label: t('agentChat.tabs.resizeSplit'),
          defaultSize: '66%',
          minSize: '280px',
          maxSize: '75%',
          mainMinSize: '360px',
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
      </ResourceWorkspace>
    </div>
  )
}
