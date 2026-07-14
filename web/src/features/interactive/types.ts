import type { SSEEvent } from '@/lib/api'

export type InteractiveSubmode = 'story' | 'timeline' | 'memory' | 'lore' | 'creator' | 'teller'

export interface StorySummary {
  id: string
  title: string
  origin: string
  story_teller_id: string
  story_director_id: string
  module_refs?: StoryDirectorModuleRefs
  reply_target_chars: number
  image_settings?: StoryImageSettings
  opening: StoryOpeningConfig
  created_at: string
  updated_at: string
  branches: number
  events: number
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
  orchestration?: TellerOrchestrationConfig | null
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
  opening_selector?: StoryDirectorOpeningSelector
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
  event_system_id?: string
  event_system_disabled?: boolean
  rule_system_id?: string
  rule_system_disabled?: boolean
  actor_state_id?: string
  actor_state_disabled?: boolean
  memory_structure_id?: string
  memory_structure_disabled?: boolean
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
  event_system?: StoryDirectorEventSystem
  trpg_system?: StoryDirectorTRPGSystem
  actor_state?: StoryDirectorActorStateSystem
  story_memory_structures?: StoryMemoryStructure[]
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
  migration_warnings?: string[]
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

interface StoryDirectorStrategy {
  enabled: boolean
  mainline_strength?: string
  failure_policy?: string
  pacing_curve?: string
	event_frequency?: 'off' | 'sparse' | 'balanced' | 'frequent' | string
  director_agent_mode?: 'triggered' | 'every_turn' | 'off' | string
  rule_state_consumption_mode?: 'hybrid_auto' | 'director_only' | string
  rule_visibility_mode?: 'audit_only' | 'public_roll' | string
	state_schema_adaptation_mode?: 'after_opening' | 'off' | string
  branch_planning_turns?: number
  planning_templates?: DirectorPlanningTemplates
  prompt_markdown?: string
}

interface StoryDirectorEventSystem {
  event_packages?: TellerEventPackage[]
  custom_events?: DirectorEvent[]
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
  /** Legacy v5 fields accepted only while loading old workspace data. */
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

interface TellerOrchestrationConfig {
  enabled: boolean
  mainline_strength?: string
  failure_policy?: string
	pacing_curve?: string
	event_frequency?: 'off' | 'sparse' | 'balanced' | 'frequent' | string
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
  intensity?: string
}

interface TellerOpeningConfig {
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

interface TellerContextPolicy {
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
  memory_entry_id?: string
  memory_status?: 'pending' | 'running' | 'ready' | 'failed'
  memory_error?: string
  versions?: TurnVersion[]
  version_idx?: number
}

export interface TurnContract {
  player_intent: string
  scene_goal: string
  beats?: string[]
  npc_intents?: string[]
  reveals?: string[]
  costs?: string[]
  continuity_constraints?: string[]
  choice_axes?: string[]
  expected_state_changes?: string[]
  scene_transition_candidate?: TurnSceneTransition
  plan_alignment_candidate?: {
    level?: 'aligned' | 'minor_deviation' | 'major_deviation' | string
    invalidated_plan_refs?: string[]
    reason?: string
  }
}

export interface TurnSceneTransition {
  kind?: 'none' | 'exit' | 'enter' | 'replace' | string
  from?: string
  to?: string
  reason?: string
}

export interface TurnResult {
  contract: TurnContract
  actor_state_patches?: Array<{
    actor_id?: string
    actor_name?: string
    template_id?: string
    state?: Record<string, unknown>
    reason?: string
  }>
  fact_candidates?: Array<{
    kind?: string
    subject?: string
    fact: string
    visibility?: string
    importance?: string
    people?: string[]
    places?: string[]
    tags?: string[]
  }>
  scene_result?: {
    status?: 'continued' | 'completed' | 'transitioned' | 'terminal' | string
    scene_id?: string
    next_scene_id?: string
    summary?: string
    next_scene_goal?: string
  }
  plan_signals?: {
    scene_transition?: TurnSceneTransition
    deviation_level?: 'none' | 'minor' | 'major' | string
    invalidated_refs?: string[]
    reason?: string
  }
  choices?: string[]
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

interface DirectorEvent {
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
  intensity?: string
  required_foreshadowing?: string[]
  payoff_target?: string
  reward?: string
  cost?: string
  failure_level?: string
  compatible_genres?: string[]
  incompatible_state_flags?: string[]
  user_configured?: boolean
  director_instruction_note?: string
}

export interface DirectorPlanDocs {
  plan: string
  lore_context: string
}

export interface DirectorPlanningTemplates {
  plan: string
}

interface DirectorPlanVisibleDocs {
  plan?: string
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
	field_path?: string
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
	field_path?: string
  usage?: 'check_decision' | 'difficulty' | 'outcome_design' | 'prose' | 'memory' | string
  guidance?: string
}

export interface RuleOutcomeStateChangeBinding {
  outcome?: 'critical_success' | 'success' | 'failure' | 'critical_failure' | string
  state_changes?: RuleComputedStateChange[]
}

export interface RuleComputedStateChange {
  source?: 'actor' | 'target' | string
	field_id?: string
	field_path?: string
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
	field_path?: string
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
  dice?: string
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
	source_path?: string
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
	actor_id?: string
	field_id?: string
	path?: string
  change: number
  reason?: string
}

interface TurnCheckAdjudication {
  reason?: string
  stakes?: string
  difficulty_reason?: string
  roll_mode_reason?: string
	state_refs?: Array<{ actor_id: string; field_id: string }>
	state_paths?: string[]
}

interface RuleResult {
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
	path?: string
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

interface OpeningRolledTrait {
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
	evidence_kind?: 'confirmed' | 'inferred' | 'default' | string
	expected_type?: string
	min?: number
	max?: number
	decision: 'covered' | 'add' | 'replace' | 'ignored' | string
	template_id?: string
	field_id?: string
	reason?: string
}

export interface ActorStateSchemaAdaptationChange {
	kind: 'template' | 'field' | 'actor' | string
	op: 'add' | 'replace' | 'remove' | string
	template_id?: string
	field_id?: string
	target_id?: string
	actor_id?: string
	reason?: string
}

export interface StateSchemaInitializationStatus {
	mode: 'after_opening' | 'off' | string
	status: 'waiting_opening' | 'running' | 'ready' | 'failed' | 'skipped' | string
	outcome?: 'changed' | 'unchanged' | string
	source_turn_id?: string
	base_revision?: number
	target_revision?: number
	summary?: string
	error?: string
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

interface InteractiveMemoryRecall {
  branch_id: string
  turn_id?: string
  query?: string
  memory_ids: string[]
  created_at: string
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
