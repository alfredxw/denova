export interface AgentChatSessionNavigationTarget {
  projectId: string
  sessionId: string
}

export const AGENT_CHAT_SESSION_NAVIGATION_EVENT = 'nova:open-agent-chat-session'

let pendingTarget: AgentChatSessionNavigationTarget | null = null

/** Opens one durable project conversation without changing the Writing mode. */
export function requestAgentChatSessionNavigation(target: AgentChatSessionNavigationTarget) {
  pendingTarget = { ...target }
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent<AgentChatSessionNavigationTarget>(AGENT_CHAT_SESSION_NAVIGATION_EVENT, {
      detail: pendingTarget,
    }))
  }
}

export function consumeAgentChatSessionNavigation(): AgentChatSessionNavigationTarget | null {
  const target = pendingTarget
  pendingTarget = null
  return target
}
