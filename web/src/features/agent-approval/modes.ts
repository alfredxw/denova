export const AGENT_APPROVAL_MODES = ['ask', 'write', 'full_access'] as const

export type AgentApprovalMode = typeof AGENT_APPROVAL_MODES[number]

export const DEFAULT_AGENT_APPROVAL_MODE: AgentApprovalMode = 'write'

/** Validates untyped settings/API data before exposing it to the UI. */
export function normalizeAgentApprovalMode(value: unknown): AgentApprovalMode | null {
  return typeof value === 'string' && AGENT_APPROVAL_MODES.includes(value as AgentApprovalMode)
    ? value as AgentApprovalMode
    : null
}
