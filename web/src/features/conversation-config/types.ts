import type { AgentApprovalMode } from '@/features/agent-approval/modes'
import type { ThinkingLevel } from '@/features/settings/thinking-levels'

export type ConversationConfigMode = 'writing' | 'agent_chat' | 'interactive' | 'automation'

/** Stable identity understood by the backend adapters for every conversation surface. */
export interface ConversationConfigBinding {
  mode: ConversationConfigMode
  project_id?: string
  session_id?: string
  story_id?: string
  branch_id?: string
  origin?: string
  resource_id?: string
  run_id?: string
}

/** Fully resolved runtime selection persisted with one conversation. */
export interface ConversationConfigSnapshot {
  runtime?: import('@/features/agent-runtime/types').RuntimeSelection
  runtime_capabilities?: import('@/features/agent-runtime/types').EngineCapabilities
  runtime_status?: import('@/features/agent-runtime/types').EngineDescriptor['status']
  agent_kind: string
  custom_agent_id?: string
  profile_id: string
  thinking_level: ThinkingLevel
  approval_mode: AgentApprovalMode
  revision: number
}

/** Missing snapshots preserve the original Native UI while it initializes. */
export function supportsRuntimeOperation(snapshot: ConversationConfigSnapshot | null, capability: keyof import('@/features/agent-runtime/types').EngineCapabilities): boolean {
  return snapshot?.runtime_capabilities?.[capability] ?? (!snapshot?.runtime || snapshot.runtime.kind === 'native')
}

export interface ConversationConfigChanges {
  runtime?: import('@/features/agent-runtime/types').RuntimeSelection
  /** Complete model selection for the current Codex engine; never switches engines. */
  codex?: import('@/features/agent-runtime/types').CodexRuntimeSettings
  claude?: import('@/features/agent-runtime/types').ClaudeRuntimeSettings
  custom_agent_id?: string
  profile_id?: string
  thinking_level?: ThinkingLevel
  approval_mode?: AgentApprovalMode
}

export interface ConversationConfigController {
  binding?: ConversationConfigBinding
  snapshot: ConversationConfigSnapshot | null
  initialized: boolean
  loading: boolean
  saving: boolean
  error: string | null
  patch: (changes: ConversationConfigChanges) => Promise<boolean>
  reload: () => Promise<ConversationConfigSnapshot | null>
}
