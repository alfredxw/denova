import { useCallback, useMemo, useState } from 'react'
import type { ComponentProps, ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'
import {
  readWritingSessionRailVisibility,
  writeWritingSessionRailVisibility,
  WritingAgentWorkspace,
} from '@/features/agent-chat/WritingAgentWorkspace'

type WritingAgentPanelOptions = ComponentProps<typeof WritingAgentWorkspace>

interface WritingAgentPanelProjection {
  content: ReactNode
  portal: ReactNode
  railVisible: boolean
}

/** Owns the one stable foreground Writing AgentPanel across every route. */
export function useWritingAgentPanel(options: WritingAgentPanelOptions): WritingAgentPanelProjection {
  const [host] = useState(() => createStablePortalHost('h-full min-h-0 w-full min-w-0 overflow-hidden'))
  const [railVisible, setRailVisible] = useState(readWritingSessionRailVisibility)
  const slot = useMemo(() => (
    <StablePortalSlot
      host={host}
      fallback={null}
      className="h-full min-h-0 w-full min-w-0 overflow-hidden"
    />
  ), [host])
  const updateRailVisibility = useCallback((visible: boolean) => {
    setRailVisible(visible)
    writeWritingSessionRailVisibility(visible)
  }, [])
  const panel = (
    <WritingAgentWorkspace
      key={options.projectId}
      {...options}
      sessionRailVisible={railVisible}
      onSessionRailVisibleChange={updateRailVisibility}
    />
  )

  return {
    content: options.active ? (host ? slot : panel) : null,
    portal: host ? createPortal(panel, host, 'workbench-agent-panel') : null,
    railVisible,
  }
}
