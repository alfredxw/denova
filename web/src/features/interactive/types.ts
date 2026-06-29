import type { SSEEvent } from '@/lib/api'

export type InteractiveSubmode = 'story' | 'timeline' | 'memory' | 'lore' | 'creator' | 'teller'

export interface StorySummary {
  id: string
  title: string
  origin: string
  story_teller_id: string
  reply_target_chars: number
  image_settings?: StoryImageSettings
  opening: StoryOpeningConfig
  created_at: string
  updated_at: string
  branches: number
  events: number
}

export type StoryImageMode = 'manual' | 'interval'

export interface StoryImageSettings {
  mode: StoryImageMode
  interval_turns: number
  preset_id?: string
}

export type StoryOpeningMode = 'ai' | 'preset' | 'custom'

export interface StoryOpeningConfig {
  mode: StoryOpeningMode
  preset_id?: string
  preset_text?: string
  custom_text?: string
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

export interface ImagePreset {
  version: number
  id: string
  name: string
  description: string
  prompt?: string
  slots?: ImagePresetSlot[]
  tags: string[]
  path?: string
  custom: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface ImagePresetSlot {
  id: string
  name: string
  target: 'agent_system' | 'tool_request'
  enabled: boolean
  content: string
}

export interface StyleRule {
  scene: string
  style_contents: string[]
}

export interface TellerContextPolicy {
  creator: string
  lore: string
  runtime_state: string
}

export interface TellerPromptSlot {
  id: string
  name: string
  target: 'system' | 'turn_context' | 'state_memory'
  enabled: boolean
  content: string
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
  memory_entry_id?: string
  memory_status?: 'pending' | 'ready' | 'failed'
  memory_error?: string
  versions?: TurnVersion[]
  version_idx?: number
}

export interface TurnDisplayEvent {
  id?: string
  role: 'assistant' | 'thinking' | 'tool_call' | 'tool_result'
  content?: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error'
  result?: string
  created_at?: string
  run_id?: string
  agent_name?: string
  root_agent_name?: string
  run_path?: string[]
  subagent?: boolean
  subagent_session_id?: string
  subagent_type?: string
}

export interface TokenUsageEvent {
  id?: string
  type?: 'token_usage'
  story_id?: string
  branch_id?: string
  created_at?: string
  run_id?: string
  agent_kind?: string
  prompt_tokens?: number
  cached_prompt_tokens?: number
  uncached_prompt_tokens?: number
  cache_hit_rate?: number
  completion_tokens?: number
  reasoning_tokens?: number
  total_tokens?: number
  model_calls?: number
  generated_bytes?: number
  usage_calls?: TokenUsageCall[]
}

export interface TokenUsageCall {
  index?: number
  created_at?: string
  finish_reason?: string
  requested_tools?: string[]
  after_tools?: string[]
  prompt_tokens?: number
  cached_prompt_tokens?: number
  uncached_prompt_tokens?: number
  cache_hit_rate?: number
  completion_tokens?: number
  reasoning_tokens?: number
  total_tokens?: number
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
  token_usage_events?: TokenUsageEvent[]
  state: Record<string, unknown>
  graph?: StoryGraph
}

export interface InteractiveMemoryEntry {
  id: string
  branch_id: string
  turn_id?: string
  title: string
  summary: string
  content: string
  people?: string[]
  places?: string[]
  tags?: string[]
  importance: number
  archived: boolean
  manual: boolean
  created_at: string
  updated_at: string
}

export interface InteractiveMemoryRecall {
  branch_id: string
  turn_id?: string
  query?: string
  memory_ids: string[]
  created_at: string
}

export interface InteractiveMemoryState {
  story_id: string
  branch_id: string
  entries: InteractiveMemoryEntry[]
  recent_recall?: InteractiveMemoryRecall
  sync_status?: 'pending' | 'ready' | 'failed' | ''
  sync_error?: string
}

export interface StoryMemorySettings {
  enabled: boolean
  auto_interval_turns: number
}

export interface StoryMemoryField {
  id: string
  name: string
  description?: string
  generation_instruction?: string
  enabled?: boolean
  required?: boolean
  order: number
}

export interface StoryMemoryStructure {
  id: string
  name: string
  description?: string
  generation_instruction?: string
  mode: 'singleton' | 'keyed' | 'append'
  key_field_id?: string
  fields: StoryMemoryField[]
  enabled?: boolean
  order: number
  built_in?: boolean
  created_at?: string
  updated_at?: string
}

export interface StoryMemoryRecord {
  id: string
  structure_id: string
  branch_id: string
  turn_id?: string
  anchor_turn_id?: string
  key?: string
  values: Record<string, string>
  archived?: boolean
  manual?: boolean
  source?: string
  inherited_from?: string
  created_at: string
  updated_at: string
}

export interface StoryMemoryState {
  story_id: string
  branch_id: string
  settings: StoryMemorySettings
  structures: StoryMemoryStructure[]
  records: StoryMemoryRecord[]
  recent_recall?: InteractiveMemoryRecall
  sync_status?: 'pending' | 'ready' | 'failed' | ''
  sync_error?: string
  next_auto_in_turns?: number
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
