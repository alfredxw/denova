export interface AgentSessionRecoveryTarget {
  sessionId: string
  taskId?: string
}

export interface InteractiveStoryRecoveryTarget {
  storyId: string
  branchId: string
  taskId?: string
}

export const AGENT_SESSION_RECOVERY_EVENT = 'nova:open-agent-session'
export const INTERACTIVE_STORY_RECOVERY_EVENT = 'nova:open-interactive-story'

let pendingAgentTarget: AgentSessionRecoveryTarget | null = null
let pendingStoryTarget: InteractiveStoryRecoveryTarget | null = null

export function requestAgentSessionRecovery(target: AgentSessionRecoveryTarget) {
  pendingAgentTarget = { ...target }
  window.dispatchEvent(new CustomEvent<AgentSessionRecoveryTarget>(AGENT_SESSION_RECOVERY_EVENT, { detail: pendingAgentTarget }))
}

export function consumeAgentSessionRecovery(): AgentSessionRecoveryTarget | null {
  const target = pendingAgentTarget
  pendingAgentTarget = null
  return target
}

export function requestInteractiveStoryRecovery(target: InteractiveStoryRecoveryTarget) {
  pendingStoryTarget = { ...target }
  window.dispatchEvent(new CustomEvent<InteractiveStoryRecoveryTarget>(INTERACTIVE_STORY_RECOVERY_EVENT, { detail: pendingStoryTarget }))
}

export function consumeInteractiveStoryRecovery(): InteractiveStoryRecoveryTarget | null {
  const target = pendingStoryTarget
  pendingStoryTarget = null
  return target
}
