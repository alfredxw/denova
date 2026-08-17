import type { AgentApprovalMode } from '@/features/agent-approval/modes'
import type { ThinkingLevel } from '@/features/settings/thinking-levels'

export type ConversationConfigMode = 'writing' | 'agent_chat' | 'interactive' | 'config_manager' | 'automation'

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
  agent_kind: string
  profile_id: string
  thinking_level: ThinkingLevel
  approval_mode: AgentApprovalMode
  revision: number
}

export interface ConversationConfigChanges {
  profile_id?: string
  thinking_level?: ThinkingLevel
  approval_mode?: AgentApprovalMode
}

export interface ConversationConfigController {
  snapshot: ConversationConfigSnapshot | null
  initialized: boolean
  loading: boolean
  saving: boolean
  error: string | null
  patch: (changes: ConversationConfigChanges) => Promise<boolean>
  reload: () => Promise<ConversationConfigSnapshot | null>
}
