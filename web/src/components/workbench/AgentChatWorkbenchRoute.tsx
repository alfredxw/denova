import { lazy, memo } from 'react'
import type { ComponentProps } from 'react'
import { RetainedWorkbenchRouteLayer } from './WorkbenchRouteHost'

const AgentChatRoute = memo(lazy(() => import('@/features/agent-chat/AgentChatRoute').then((module) => ({ default: module.AgentChatRoute }))))

interface AgentChatWorkbenchRouteProps extends ComponentProps<typeof AgentChatRoute> {
  mounted: boolean
  visible: boolean
  loadingLabel: string
  retentionKey: string
}

/** Keeps the shared Agent Chat project mounted without coupling it to Writing layout state. */
export function AgentChatWorkbenchRoute({
  mounted,
  visible,
  loadingLabel,
  retentionKey,
  ...routeProps
}: AgentChatWorkbenchRouteProps) {
  if (!mounted) return null
  return (
    <RetainedWorkbenchRouteLayer visible={visible} loadingLabel={loadingLabel} retentionKey={retentionKey}>
      <AgentChatRoute {...routeProps} />
    </RetainedWorkbenchRouteLayer>
  )
}
