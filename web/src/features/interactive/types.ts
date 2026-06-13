import type { SSEEvent } from '@/lib/api'

export type InteractiveSubmode = 'story' | 'timeline' | 'lore' | 'creator' | 'teller'

export interface StorySummary {
  id: string
  title: string
  origin: string
  story_teller_id: string
  reply_target_chars: number
  created_at: string
  updated_at: string
  branches: number
  events: number
}

export interface StoryIndex {
  current_story_id: string
  stories: StorySummary[]
}

export interface Teller {
  version: number
  id: string
  name: string
  description: string
  random_event_rate: number
  style_rules?: StyleRule[] | null
  tags: string[]
  context_policy: TellerContextPolicy
  slots: TellerPromptSlot[]
  custom: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface StyleRule {
  scene: string
  styles: string[]
}

export interface TellerContextPolicy {
  creator: string
  lore: string
  runtime_state: string
  recent_turns: number
}

export interface TellerPromptSlot {
  id: string
  name: string
  target: 'system' | 'turn_context' | 'state_memory'
  enabled: boolean
  content: string
}

export interface TellerAgentResult {
  message: string
  action: 'create' | 'update'
  teller: Teller
  tellers: Teller[]
}

export interface TurnEvent {
  id: string
  parent_id: string | null
  branch_id: string
  ts: string
  user: string
  narrative: string
  thinking?: string
  display_events?: TurnDisplayEvent[]
  state_delta?: StateDelta
  hot_state?: HotState
  state_status?: 'pending' | 'ready' | 'failed'
  state_error?: string
  versions?: TurnVersion[]
  version_idx?: number
}

export interface TurnDisplayEvent {
  id?: string
  role: 'thinking' | 'tool_call' | 'tool_result'
  content?: string
  name?: string
  status?: 'running' | 'success' | 'error'
  created_at?: string
}

export interface TurnVersion {
  turn_id: string
  ts: string
  current?: boolean
}

export interface StateDelta {
  ops: StateOp[]
}

export interface StateOp {
  op: string
  path: string
  value?: unknown
}

export interface HotState {
  choices: string[]
}

export interface HotChoicesResponse {
  enabled: boolean
  choices: string[]
}

export interface Snapshot {
  story_id: string
  branch_id: string
  turns: TurnEvent[]
  current_turn?: TurnEvent
  state: Record<string, unknown>
  graph?: StoryGraph
}

export interface BranchSummary {
  id: string
  head: string
  from?: string
  from_event?: string
  title?: string
  created_at: string
  current: boolean
}

export interface PlotNode {
  id: string
  parent_id?: string
  branch_id: string
  title: string
  summary: string
  ts: string
  current: boolean
  head: boolean
}

export interface StoryGraph {
  nodes: PlotNode[]
  branches: BranchSummary[]
}

export type InteractiveSSEEvent = SSEEvent
