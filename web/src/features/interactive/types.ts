import type { SSEEvent } from '@/lib/api'

export type InteractiveSubmode = 'story' | 'timeline' | 'director' | 'lore' | 'creator' | 'teller'

export interface StorySummary {
  id: string
  title: string
  origin: string
  story_teller_id: string
  story_director_id: string
  module_refs?: StoryDirectorModuleRefs
  reply_target_chars: number
  choice_count: number
  image_settings?: StoryImageSettings
  opening: StoryOpeningConfig
  state_schema_policy?: StoryStateSchemaPolicy
  created_at: string
  updated_at: string
  branches: number
  events: number
}

export type StoryStateSchemaMode = 'adapt_template' | 'fixed_template' | 'generate'

export interface StoryStateSchemaPolicy {
  mode: StoryStateSchemaMode
}

type StoryImageMode = 'manual' | 'interval'

export interface StoryImageSettings {
  mode: StoryImageMode
  interval_turns: number
  preset_id?: string
}

type StoryOpeningMode = 'ai' | 'preset' | 'custom'

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
  style_refs?: string[] | null
  style_rules?: StyleRule[] | null
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
  trpg_system: StoryDirectorTRPGSystem
  actor_state?: StoryDirectorActorStateSystem
  resolved_snapshot?: StoryDirectorResolvedSnapshot
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
  rule_system_id?: string
  rule_system_disabled?: boolean
  actor_state_id?: string
  actor_state_disabled?: boolean
  image_preset_id?: string
  image_preset_disabled?: boolean
}

interface StoryDirectorModuleWarning {
  module: string
  id?: string
  message: string
}

interface StoryDirectorResolvedSnapshot {
  version: number
  resolved_at?: string
  status?: string
  warnings?: StoryDirectorModuleWarning[]
  module_refs?: StoryDirectorModuleRefs
  narrative_style_id?: string
  image_preset_id?: string
  event_packages?: TellerEventPackage[]
  trpg_system?: StoryDirectorTRPGSystem
  actor_state?: StoryDirectorActorStateSystem
}

export interface EventPackageModule {
  version: number
  id: string
  name: string
  description: string
  events?: TellerEventCard[]
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
  actor_state_id?: string
  trpg_system: StoryDirectorTRPGSystem
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
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
}

interface StoryDirectorStrategy {
  enabled: boolean
  mainline_strength?: string
  failure_policy?: string
  pacing_curve?: string
  event_frequency?: 'off' | 'sparse' | 'balanced' | 'frequent' | string
  director_agent_mode?: 'triggered' | 'every_turn' | 'off' | string
  rule_state_consumption_mode?: 'hybrid_auto' | 'director_only' | string
  rule_visibility_mode?: 'audit_only' | 'public_roll' | string
  branch_planning_turns?: number
  planning_templates?: DirectorPlanningTemplates
  prompt_markdown?: string
}

export interface StoryDirectorTRPGSystem {
  rule_templates?: RuleCheck[]
}

export interface StoryDirectorActorStateSystem {
  templates?: ActorStateTemplate[]
  initial_actors?: ActorStateInitialActor[]
  trait_pools?: ActorTraitPool[]
}

export interface ActorStateTemplate {
  id: string
  name: string
  description?: string
  fields?: ActorStateField[]
  trait_rules?: ActorTraitRule[]
  /** Legacy Beta input retained for API compatibility; normalized schemas ignore it. */
  display_groups?: string[]
}

export interface ActorTraitRule {
  pool_id: string
  draw_count: number
}

export interface ActorTraitPool {
  id: string
  name: string
  description?: string
  traits?: ActorTraitDefinition[]
}

export interface ActorTraitDefinition {
  id: string
  name: string
  summary?: string
  weight?: number
  visibility?: 'visible' | 'hidden' | 'spoiler'
}

export interface ActorTraitInstance {
  pool_id: string
  pool_name?: string
  trait_id: string
  name: string
  summary?: string
  visibility?: 'visible' | 'hidden' | 'spoiler'
  source_kind?: string
  source_id?: string
  source_turn_id?: string
}

export interface ActorTraitSelection {
  pool_id: string
  trait_ids?: string[]
}

export interface InitialActorTraitRoll {
  actor_id: string
  selections?: ActorTraitSelection[]
  seed?: number
}

export interface ActorStateField {
  id?: string
  path?: string
  name: string
  type: 'number' | 'string' | 'bool' | 'enum' | 'object' | 'list' | string
  default?: unknown
  min?: number
  max?: number
  options?: string[]
  visibility?: 'visible' | 'hidden' | 'spoiler'
  description?: string
  update_instruction?: string
  /** Legacy Beta input retained for API compatibility; field array order is the fallback. */
  order?: number
  /** Optional presentation hint: cluster fields under one named ledger section. */
  group?: string
  /** Optional presentation hint: pin the field renderer; falls back to heuristics when empty. */
  display?: 'stat' | 'inline' | 'block' | 'list'
}

export interface ActorStateInitialActor {
  id: string
  name: string
  template_id: string
  role?: string
  description?: string
  state?: Record<string, unknown>
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
  intensity?: string
}

interface TellerContextPolicy {
  creator: string
  lore: string
  runtime_state: string
}

export interface TellerPromptSlot {
  id: string
  name: string
  target: 'system' | 'turn_context'
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
  run_id?: string
  agent_kind?: string
  display_events?: TurnDisplayEvent[]
  state_delta?: StateDelta
  hot_state?: HotState
  rule_resolution?: RuleResolution
  turn_result?: TurnResult
  terminal_outcome?: TerminalOutcome
  state_status?: 'pending' | 'ready' | 'failed'
  state_error?: string
  versions?: TurnVersion[]
  version_idx?: number
}

export interface UpdateTurnNarrativeResult {
  turn: TurnEvent
  context_compaction_invalidated: boolean
}

export interface TurnResult {
  state_updates: Array<{ op: 'replace' | 'delta' | 'create' | string; path: string; value: unknown }>
  choices: string[]
  director_update?: { needed: boolean; reason?: string }
}

export interface TurnDisplayEvent {
  id?: string
  role: 'assistant' | 'thinking' | 'tool_call' | 'tool_result' | 'narrative'
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

interface TokenUsageCall {
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

interface TurnVersion {
  turn_id: string
  ts: string
  current?: boolean
}

interface StateDelta {
	schema_version?: number
  ops?: StateOp[]
  actor_ops?: ActorStateOp[]
}

export interface ActorStateOp {
  op: string
  actor_id: string
  field_id: string
  value?: unknown
  reason?: string
  source_turn_id?: string
  source_kind?: string
  source_id?: string
}

export interface StateOp {
  op: string
  path: string
  value?: unknown
  reason?: string
  source_turn_id?: string
  source_kind?: string
  source_id?: string
}

interface HotState {
  choices: string[]
}

export interface DirectorPlanDocs {
  plan: string
  agent_brief: string
  lore_context: string
}

export interface DirectorPlanningTemplates {
  plan: string
  agent_brief: string
}

interface DirectorPlanVisibleDocs {
  agent_brief?: string
  lore_context?: string
}

interface DirectorPlanDocInfo {
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
	decision?: PlanDecision
	event_opportunity?: EventOpportunity
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
	decision?: PlanDecision
	event_runtime?: DirectorEventRuntime
	event_opportunity?: EventOpportunity
}

export interface PlanDecision {
  mode: 'keep' | 'patch' | 'replan' | string
  triggers?: string[]
  scene_transition?: {
    kind?: 'none' | 'exit' | 'enter' | 'replace' | string
    from?: string
    to?: string
    evidence?: string[]
  }
  deviation?: {
    level?: 'none' | 'minor' | 'major' | string
    invalidated_plan_refs?: string[]
    reason?: string
  }
  reason?: string
	base_revision?: string
	event_decision?: EventDecision
}

export interface EventDecision {
	mode: 'none' | 'seed' | 'advance' | 'payoff' | 'resolve' | 'abandon' | string
	event_ref?: string
	summary?: string
	reason?: string
	evidence?: string[]
	evidence_turn_ids?: string[]
}

export interface EventOpportunity {
	due: boolean
	kind: 'none' | 'new' | 'active' | string
	reason?: string
	turns_since_review?: number
	review_interval?: number
	active_event_ref?: string
	forced?: boolean
}

export interface DirectorEventThread {
	event_ref: string
	summary?: string
	stage?: string
	seeded_turn_id?: string
	updated_turn_id?: string
}

export interface DirectorEventRuntime {
	active?: DirectorEventThread
	last_opportunity_turn_id?: string
	recent_decisions?: Array<{ id: string; source_turn_id: string; decision: EventDecision }>
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
	event_runtime?: DirectorEventRuntime
	lore_revision?: string
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
  dice?: '1d20' | string
  modifier?: number
  failure_policy?: 'fail_forward' | 'success_at_cost' | 'blocked' | 'hard_failure' | string
  difficulty_guidance?: string
  state_effect_guidance?: string
  trigger?: string
  must_check_examples?: string[]
  skip_check_examples?: string[]
  success_hint?: string
  failure_hint?: string
  state_bindings?: RuleStateBinding[]
}

export interface RuleStateBinding {
  id?: string
  label?: string
  trigger?: string
  actor_template_id?: string
  target_template_id?: string
  modifiers?: RuleStateBindingModifier[]
  narrative_state_refs?: RuleNarrativeStateRef[]
  outcome_state_changes?: RuleOutcomeStateChangeBinding[]
}

export interface RuleStateBindingModifier {
  source?: 'actor' | 'target' | string
  field_id?: string
  value_path?: string[]
  effect?: 'advantage' | 'resistance' | string
  scale?: number
  offset?: number
  min?: number
  max?: number
  rounding?: 'none' | 'floor' | 'ceil' | 'nearest' | string
  required?: boolean
}

export interface RuleNarrativeStateRef {
  source?: 'actor' | 'target' | 'scene' | string
  field_id?: string
  usage?: 'check_decision' | 'difficulty' | 'outcome_design' | 'prose' | string
  guidance?: string
}

export interface RuleOutcomeStateChangeBinding {
  outcome?: 'critical_success' | 'success' | 'failure' | 'critical_failure' | string
  state_changes?: RuleComputedStateChange[]
}

export interface RuleComputedStateChange {
  source?: 'actor' | 'target' | string
  field_id?: string
  change_formula?: RuleStateChangeFormula
  reason?: string
}

export interface RuleStateChangeFormula {
  base?: number
  terms?: RuleStateFormulaTerm[]
  min?: number
  max?: number
  rounding?: 'none' | 'floor' | 'ceil' | 'nearest' | string
}

export interface RuleStateFormulaTerm {
  source?: 'actor' | 'target' | string
  field_id?: string
  value_path?: string[]
  scale?: number
  offset?: number
}

export interface RuleResolution {
  id?: string
  request: TurnCheckRequest
  result: RuleResult
  state_consumption?: RuleStateConsumption
  terminal_candidate?: TerminalCandidate
  rule_constraints?: string[]
  created_at?: string
  seed?: number
}

interface TurnCheckRequest {
  action: string
  intent: string
  challenge: string
  cost: string
  state: string
  adjudication?: TurnCheckAdjudication
  rule?: TurnCheckRule
  bonuses?: TurnCheckBonus[]
  difficulty: 'very_easy' | 'easy' | 'normal' | 'hard' | 'very_hard' | string
  outcomes: TurnCheckOutcomes
}

interface TurnCheckRule {
  template?: string
  template_id?: string
  label?: string
  failure_policy?: string
  roll_mode?: 'normal' | 'advantage' | 'disadvantage' | string
  modifier?: number
  binding_id?: string
  actor_id?: string
  target_actor_id?: string
}

interface TurnCheckBonus {
  kind?: string
  actor_id?: string
  field_id?: string
  reason: string
  value: number
}

interface TurnCheckOutcomes {
  critical_success: TurnCheckOutcome
  success: TurnCheckOutcome
  failure: TurnCheckOutcome
  critical_failure: TurnCheckOutcome
}

interface TurnCheckOutcome {
  result: string
  state_changes?: TurnStateChange[]
}

interface TurnStateChange {
  actor_id: string
  field_id: string
  change: number
  reason?: string
}

interface TurnCheckAdjudication {
  reason?: string
  stakes?: string
  difficulty_reason?: string
  roll_mode_reason?: string
  state_refs?: Array<{ actor_id: string; field_id: string }>
}

interface RuleResult {
  id?: string
  label?: string
  kind?: string
  mode?: string
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
  bonus_details?: TurnCheckBonus[]
  base_target?: number
  target?: number
  result?: string
  state_changes?: TurnStateChange[]
}

interface RuleStateConsumption {
  status: 'none' | 'disabled' | 'applied' | 'partial' | 'skipped' | string
  mode?: 'hybrid_auto' | 'director_only' | string
  applied_ops?: StateOp[]
  applied_actor_ops?: ActorStateOp[]
  warnings?: RuleStateConsumptionWarning[]
}

interface RuleStateConsumptionWarning {
  actor_id?: string
  field_id?: string
  reason: string
}

interface TerminalCandidate {
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

export interface ActorTraitRollRequest {
  story_director_id?: string
  actor_id: string
  template_id: string
  selections?: ActorTraitSelection[]
  seed?: number
}

export interface ActorTraitRollResult {
  story_director_id?: string
  actor_id: string
  template_id: string
  seed: number
  traits: ActorTraitInstance[]
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
  actor_state_schema?: ActorStateSchemaSnapshot
	state_schema_initialization?: StateSchemaInitializationStatus
  graph?: StoryGraph
}

export interface ActorStateSchemaSnapshot {
  version: number
	revision: number
  system: StoryDirectorActorStateSystem
	trpg_system?: StoryDirectorTRPGSystem
	adaptation?: ActorStateSchemaAdaptationRecord
  legacy_field_paths?: Record<string, Record<string, string>>
  legacy_actor_templates?: Record<string, string>
}

export interface ActorStateSchemaAdaptationRecord {
	source: string
	summary?: string
	source_turn_id?: string
	lore_revision?: string
	template_ops?: number
	field_ops?: number
	initial_actor_ops?: number
	actor_ops?: number
	reviewed_lore_ids?: string[]
	requirements?: ActorStateSchemaRequirementReview[]
	changes?: ActorStateSchemaAdaptationChange[]
	warnings?: string[]
}

export interface ActorStateSchemaRequirementSource {
	kind: 'lore' | 'opening' | 'turn_result' | 'trpg' | string
	id: string
}

export interface ActorStateSchemaRequirementReview {
	source: ActorStateSchemaRequirementSource
	requirement: string
	evidence_kind: 'confirmed' | 'inferred' | 'default' | string
	value_policy?: 'schema_only' | 'preserve' | 'initialize' | 'defer' | string
	actor_id?: string
	expected_type?: string
	min?: number
	max?: number
	decision: 'covered' | 'add' | 'replace' | 'ignored' | string
	template_id?: string
	field_id?: string
	reason?: string
}

export interface ActorStateSchemaAdaptationChange {
	kind: 'template' | 'field' | 'actor' | 'actor_field' | string
	op: 'add' | 'replace' | 'remove' | 'set' | string
	template_id?: string
	field_id?: string
	target_id?: string
	actor_id?: string
	reason?: string
	value_source?: {
		source_id: string
		item_id: string
		source: ActorStateSchemaRequirementSource
		evidence_kind: string
	}
}

export interface StateSchemaInitializationStatus {
  mode: StoryStateSchemaMode
  status: 'waiting_opening' | 'ready'
  outcome?: 'changed' | 'unchanged' | 'fixed'
  source_turn_id?: string
  base_revision?: number
  target_revision?: number
  summary?: string
  lore_revision?: string
  reviewed_lore_ids?: string[]
  requirements?: ActorStateSchemaRequirementReview[]
  changes?: ActorStateSchemaAdaptationChange[]
  warnings?: string[]
  started_at?: string
  completed_at?: string
  updated_at?: string
}

interface ContextCompactionEvent {
  id?: string
  agent_kind?: string
  epoch: number
  summary: string
  source_turn_count?: number
  retained_turns?: number
  tokens_before?: number
  tokens_after?: number
	projected_tokens_before?: number
	projected_tokens_after?: number
	reserved_completion_tokens?: number
	reserved_tool_result_tokens?: number
  target_ratio?: number
  context_window_tokens?: number
  strategy?: string
  threshold?: number
  reason?: string
  phase?: string
}

interface ContextCompactionRemovalEvent {
  id?: string
  agent_kind?: string
  compaction_id?: string
  source_turn_count?: number
  reason?: string
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

interface StoryGraph {
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
