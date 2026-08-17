import { useMemo, useState } from 'react'
import type { ComponentProps, ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { AgentPanel } from '@/components/Chat/AgentPanel'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'

type WritingAgentPanelOptions = ComponentProps<typeof AgentPanel>

interface WritingAgentPanelProjection {
  content: ReactNode
  portal: ReactNode
}

/** Owns the one stable foreground Writing AgentPanel across every route. */
export function useWritingAgentPanel(options: WritingAgentPanelOptions): WritingAgentPanelProjection {
  const [host] = useState(() => createStablePortalHost('h-full min-h-0 w-full min-w-0 overflow-hidden'))
  const slot = useMemo(() => (
    <StablePortalSlot
      host={host}
      fallback={null}
      className="h-full min-h-0 w-full min-w-0 overflow-hidden"
    />
  ), [host])
  const panel = <AgentPanel {...options} />

  return {
    content: options.active ? (host ? slot : panel) : null,
    portal: host ? createPortal(panel, host, 'workbench-agent-panel') : null,
  }
}
