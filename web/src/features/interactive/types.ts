import type { SSEEvent } from '@/lib/api'

export type InteractiveSubmode = 'story' | 'timeline' | 'memory' | 'lore' | 'creator' | 'teller'

export interface StorySummary {
  id: string
  title: string
  origin: string
  story_teller_id: string
  story_director_id: string
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
  style_refs?: string[] | null
  style_rules?: StyleRule[] | null
  orchestration?: TellerOrchestrationConfig | null
  tags: string[]
  context_policy: TellerContextPolicy
  slots: TellerPromptSlot[]
  custom: boolean
  builtin_overridden?: boolean
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
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface StoryDirector {
  version: number
  id: string
  name: string
  description: string
  module_refs?: StoryDirectorModuleRefs
  strategy: StoryDirectorStrategy
  event_packages?: TellerEventPackage[]
  stat_system: StoryDirectorStatSystem
  trpg_system: StoryDirectorTRPGSystem
  actor_state?: StoryDirectorActorStateSystem
  opening_selector: StoryDirectorOpeningSelector
  resolved_snapshot?: StoryDirectorResolvedSnapshot
  tags: string[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface StoryDirectorModuleRefs {
  narrative_style_id?: string
  narrative_style_disabled?: boolean
  event_package_ids?: string[]
  event_packages_disabled?: boolean
  event_system_id?: string
  event_system_disabled?: boolean
  rule_system_id?: string
  rule_system_disabled?: boolean
  actor_state_id?: string
  actor_state_disabled?: boolean
  memory_structure_id?: string
  memory_structure_disabled?: boolean
  opening_selector_id?: string
  opening_selector_disabled?: boolean
  image_preset_id?: string
  image_preset_disabled?: boolean
}

export interface StoryDirectorModuleWarning {
  module: string
  id?: string
  message: string
}

export interface StoryDirectorResolvedSnapshot {
  version: number
  resolved_at?: string
  status?: string
  warnings?: StoryDirectorModuleWarning[]
  module_refs?: StoryDirectorModuleRefs
  narrative_style_id?: string
  image_preset_id?: string
  event_packages?: TellerEventPackage[]
  event_system?: StoryDirectorEventSystem
  stat_system?: StoryDirectorStatSystem
  trpg_system?: StoryDirectorTRPGSystem
  actor_state?: StoryDirectorActorStateSystem
  story_memory_structures?: StoryMemoryStructure[]
  opening_selector?: StoryDirectorOpeningSelector
}

export interface EventPackageModule {
  version: number
  id: string
  name: string
  description: string
  events?: TellerEventCard[]
  tags: string[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface RuleSystemModule {
  version: number
  id: string
  name: string
  description: string
  stat_system: StoryDirectorStatSystem
  trpg_system: StoryDirectorTRPGSystem
  tags: string[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface ActorStateModule {
  version: number
  id: string
  name: string
  description: string
  actor_state: StoryDirectorActorStateSystem
  tags: string[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface StoryMemoryStructureModule {
  version: number
  id: string
  name: string
  description: string
  structures: StoryMemoryStructure[]
  tags: string[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface OpeningSelectorModule {
  version: number
  id: string
  name: string
  description: string
  opening_selector: StoryDirectorOpeningSelector
  tags: string[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

export interface StoryDirectorStrategy {
  enabled: boolean
  mainline_strength?: string
  failure_policy?: string
  pacing_curve?: string
  random_event_rate?: number
  director_agent_mode?: 'triggered' | 'every_turn' | 'off' | string
  branch_planning_turns?: number
  planning_templates?: DirectorPlanDocs
  prompt_markdown?: string
}

export interface StoryDirectorEventSystem {
  event_packages?: TellerEventPackage[]
  custom_events?: DirectorEvent[]
}

export interface StoryDirectorStatSystem {
  attributes?: StoryDirectorAttribute[]
}

export interface StoryDirectorAttribute {
  id?: string
  path: string
  name: string
  type?: string
  default?: number
  min?: number
  max?: number
  visibility?: 'visible' | 'hidden' | 'spoiler'
  description?: string
}

export interface StoryDirectorTRPGSystem {
  rule_templates?: RuleCheck[]
}

export interface StoryDirectorActorStateSystem {
  templates?: ActorStateTemplate[]
  initial_actors?: ActorStateInitialActor[]
}

export interface ActorStateTemplate {
  id: string
  name: string
  description?: string
  fields?: ActorStateField[]
}

export interface ActorStateField {
  id?: string
  path: string
  name: string
  type: 'number' | 'string' | 'bool' | 'enum' | 'object' | 'list' | string
  default?: unknown
  min?: number
  max?: number
  options?: string[]
  visibility?: 'visible' | 'hidden' | 'spoiler'
  description?: string
  update_instruction?: string
  order?: number
}

export interface ActorStateInitialActor {
  id: string
  name: string
  template_id: string
  role?: string
  description?: string
  state?: Record<string, unknown>
}

export interface StoryDirectorOpeningSelector {
  enabled: boolean
  trait_pools?: OpeningTraitPool[]
  initial_state_ops?: StateOp[]
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
  style_refs?: string[]
  style_contents?: string[]
}

export interface StyleReference {
  name: string
  description: string
  path: string
  display_path: string
  size?: number
  updated_at?: string
  missing?: boolean
  error?: string
}

export interface StyleReferenceFileDocument {
  reference: StyleReference
  content: string
  revision: string
}

export interface TellerOrchestrationConfig {
  enabled: boolean
  mainline_strength?: string
  failure_policy?: string
  pacing_curve?: string
  event_packages?: TellerEventPackage[]
  custom_events?: DirectorEvent[]
  rule_templates?: RuleCheck[]
  opening?: TellerOpeningConfig
}

export interface TellerEventPackage {
  id?: string
  name?: string
  enabled: boolean
  events?: TellerEventCard[]
}

export interface TellerEventCard {
  id?: string
  type_name?: string
  description_markdown?: string
  enabled: boolean
  category?: string
  tags?: string[]
  weight?: number
  cooldown_turns?: number
  intensity?: string
}

export interface TellerOpeningConfig {
  enabled: boolean
  trait_pools?: OpeningTraitPool[]
  initial_state_ops?: StateOp[]
}

export interface OpeningTraitPool {
  id?: string
  name?: string
  draw_count?: number
  traits?: OpeningTrait[]
}

export interface OpeningTrait {
  id?: string
  name?: string
  summary?: string
  weight?: number
  ops?: StateOp[]
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
  rule_resolution?: RuleResolution
  terminal_outcome?: TerminalOutcome
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
  agent_kind?: string
  agent_name?: string
  root_agent_name?: string
  run_path?: string[]
  subagent?: boolean
  subagent_session_id?: string
  subagent_type?: string
  sse_hidden_fields?: string[]
  sse_hidden_reason?: string
  sse_display_notice?: string
  sse_generated_chars?: number
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
  reason?: string
  source_turn_id?: string
}

export interface HotState {
  choices: string[]
}

export interface DirectorEvent {
  id?: string
  name?: string
  category?: string
  status?: string
  enabled?: boolean
  summary?: string
  public_summary?: string
  hidden_truth?: string
  template?: string
  normalized_trigger?: string
  weight?: number
  cooldown_turns?: number
  intensity?: string
  required_foreshadowing?: string[]
  payoff_target?: string
  reward?: string
  cost?: string
  failure_level?: string
  compatible_genres?: string[]
  incompatible_state_flags?: string[]
  user_configured?: boolean
  last_triggered_turn_id?: string
  next_eligible_after_turns?: number
  director_instruction_note?: string
}

export interface DirectorPlanDocs {
  plan: string
}

export interface DirectorPlanVisibleDocs {
  plan?: string
}

export interface DirectorPlanDocInfo {
  path: string
  bytes: number
  hash: string
  visible_bytes?: number
}

export interface DirectorPlanRunStatus {
  status?: string
  summary?: string
  error?: string
  source_turn_id?: string
  updated_at?: string
  planned_docs?: number
  completed_docs?: number
  start_ready?: boolean
  blocking?: boolean
}

export interface DirectorPlanStatus {
  story_id: string
  branch_id: string
  status: string
  summary?: string
  error?: string
  source_turn_id?: string
  updated_at?: string
  planned_docs: number
  completed_docs: number
  doc_bytes: number
  visible_bytes: number
  start_ready: boolean
  blocking: boolean
  revision?: string
}

export interface DirectorPlanMetadata {
  version: number
  story_id: string
  branch_id: string
  revision: string
  branch_planning_turns: number
  updated_at: string
  source?: string
  source_turn_id?: string
  docs?: Record<string, DirectorPlanDocInfo>
  last_run?: DirectorPlanRunStatus
}

export interface DirectorPlan {
  story_id: string
  branch_id: string
  docs: DirectorPlanDocs
  visible_docs?: DirectorPlanVisibleDocs
  metadata: DirectorPlanMetadata
}

export interface UpdateDirectorPlanInput {
  branch_id?: string
  docs: DirectorPlanDocs
  base_revision?: string
  source?: string
  summary?: string
}

export interface RuleCheck {
  id?: string
  label?: string
  kind?: string
  mode?: 'default' | 'd20_dc' | 'd100_under'
  attribute_path?: string
  expression?: string
  dice?: string
  modifier?: number
  difficulty?: number
  resource_cost_path?: string
  resource_cost?: number
  success_state_ops?: StateOp[]
  failure_state_ops?: StateOp[]
  terminal_on_failure?: boolean
  terminal_type?: string
  terminal_reason?: string
  seed?: number
}

export interface RuleResolution {
  id?: string
  request: TurnCheckRequest
  result: RuleResult
  terminal_candidate?: TerminalCandidate
  rule_constraints?: string[]
  created_at?: string
  seed?: number
}

export interface TurnCheckRequest {
  action: string
  intent: string
  challenge: string
  cost: string
  state: string
  rule?: TurnCheckRule
  bonuses?: TurnCheckBonus[]
  difficulty: 'very_easy' | 'easy' | 'normal' | 'hard' | 'very_hard' | string
  outcomes: TurnCheckOutcomes
}

export interface TurnCheckRule {
  template?: string
  dice?: string
  roll_mode?: 'normal' | 'advantage' | 'disadvantage' | string
}

export interface TurnCheckBonus {
  reason: string
  value: number
}

export interface TurnCheckOutcomes {
  critical_success: TurnCheckOutcome
  success: TurnCheckOutcome
  failure: TurnCheckOutcome
  critical_failure: TurnCheckOutcome
}

export interface TurnCheckOutcome {
  result: string
  state_changes?: TurnStateChange[]
}

export interface TurnStateChange {
  path: string
  change: number
}

export interface RuleResult {
  id?: string
  label?: string
  kind?: string
  mode?: string
  attribute_path?: string
  attribute_value?: number
  expression?: string
  expression_value?: number
  dice?: string
  rolls?: number[]
  roll_total?: number
  modifier?: number
  difficulty?: number
  total?: number
  outcome: string
  seed?: number
  constraints?: string[]
  error?: string
  roll_mode?: string
  kept_roll?: number
  bonus_total?: number
  target?: number
  result?: string
  state_changes?: TurnStateChange[]
}

export interface TerminalCandidate {
  type?: string
  reason?: string
  check_id?: string
}

export interface TerminalOutcome {
  terminal: boolean
  type?: string
  reason?: string
  final_narrative_summary?: string
  caused_by_turn_id?: string
  rule_resolution_id?: string
  restart_suggestions?: string[]
}

export interface HotChoicesResponse {
  enabled: boolean
  choices: string[]
}

export interface OpeningRollRequest {
  teller_id?: string
  story_director_id?: string
  selected_trait_ids?: string[]
  locked_trait_ids?: string[]
  seed?: number
}

export interface OpeningRollResult {
  teller_id?: string
  story_director_id?: string
  seed: number
  traits: OpeningRolledTrait[]
  state_ops: StateOp[]
}

export interface OpeningRolledTrait {
  pool_id: string
  id: string
  name: string
  summary?: string
}

export interface RuleResolutionRerollInput {
  branch_id?: string
  turn_id?: string
}

export interface Snapshot {
  story_id: string
  branch_id: string
  turns: TurnEvent[]
  current_turn?: TurnEvent
  token_usage_events?: TokenUsageEvent[]
  context_compaction?: ContextCompactionEvent | null
  context_compaction_removal?: ContextCompactionRemovalEvent | null
  director_plan?: DirectorPlan
  director_plan_status?: DirectorPlanStatus
  state: Record<string, unknown>
  graph?: StoryGraph
}

export interface ContextCompactionEvent {
  id?: string
  agent_kind?: string
  epoch: number
  summary: string
  source_turn_count?: number
  retained_turns?: number
  tokens_before?: number
  tokens_after?: number
  target_ratio?: number
  context_window_tokens?: number
  strategy?: string
  threshold?: number
  reason?: string
  phase?: string
}

export interface ContextCompactionRemovalEvent {
  id?: string
  agent_kind?: string
  compaction_id?: string
  source_turn_count?: number
  reason?: string
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
  read_only?: boolean
  derived?: boolean
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
  memory_structure_id?: string
  memory_structure_name?: string
  memory_structure_disabled?: boolean
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
  terminal?: boolean
  terminal_type?: string
}

export interface StoryGraph {
  nodes: PlotNode[]
  branches: BranchSummary[]
}

export interface InteractiveTurnPersistedEvent {
  story_id: string
  branch_id: string
  turn: TurnEvent
  director_plan?: DirectorPlan
  director_plan_status?: DirectorPlanStatus
  state?: Record<string, unknown>
  graph?: StoryGraph
  branches?: BranchSummary[]
  context_compaction?: ContextCompactionEvent | null
  context_compaction_removal?: ContextCompactionRemovalEvent | null
}

export type InteractiveSSEEvent = SSEEvent
