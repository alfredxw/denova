export interface AgentChatSessionNavigationTarget {
  projectId: string
  sessionId: string
  /** Optional Project-relative source to show alongside the conversation. */
  sourcePath?: string
  /** Submitted once after the destination conversation is ready, never a second session. */
  initialInstruction?: string
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

/** Keep an intent pending across lazy mounts and Strict Mode effect restarts. */
export function pendingAgentChatSessionNavigation() {
  return pendingTarget
}

export function completeAgentChatSessionNavigation(target: AgentChatSessionNavigationTarget) {
  if (pendingTarget === target) pendingTarget = null
}
