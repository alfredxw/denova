// MessageItem render model. Agent API/history/stream payloads use AgentUIMessage;
// this shape remains private to the presentation adapter. Role-specific payloads
// are kept on a discriminated union so invalid message combinations fail during
// development instead of silently reaching a renderer.
export type ChatMessageStatus = 'running' | 'success' | 'error'
export type ChatPlanAction = 'approved' | 'continue' | 'exited'
export type InteractiveImageStatus = ChatMessageStatus

export type ToolPresentationKind =
  | 'generic'
  | 'file'
  | 'search'
  | 'terminal'
  | 'web'
  | 'browser'
  | 'image'
  | 'interactive_media'
  | 'todo'
  | 'interaction'
  | 'delegation'
  | 'script'

export interface ToolPresentation {
  call: ToolPresentationKind
  result: ToolPresentationKind
}

interface ChatMessageBase {
  type?: 'message' | 'clear'
  content?: string
  id?: string
  render_key?: string
  streaming_target_content?: string
  turn_id?: string
  navigation_turn_id?: string
  run_id?: string
  display_segment_id?: string
  display_phase?: 'candidate' | 'progress' | 'final' | 'partial'
  agent_kind?: string
  agent_name?: string
  root_agent_name?: string
  run_path?: string[]
  subagent?: boolean
  subagent_session_id?: string
  subagent_type?: string
	parent_call_id?: string
  streaming?: boolean
  created_at?: string
  tool_presentation?: ToolPresentation
  turn_versions?: { turn_id: string; ts: string; current?: boolean }[]
  turn_version_index?: number
}

export interface ChatAttachment {
  id?: string
  name: string
  media_type?: string
  size: number
}

export interface UserChatMessage extends ChatMessageBase {
  role: 'user'
  user_references?: UserMessageReference[]
  attachments?: ChatAttachment[]
}

export interface AssistantChatMessage extends ChatMessageBase {
  role: 'assistant'
  interactive_image?: InteractiveImage
  interactive_images?: InteractiveImage[]
  interactive_image_error?: InteractiveImageError
  interactive_image_status?: InteractiveImageStatus
}

export interface ThinkingChatMessage extends ChatMessageBase {
  role: 'thinking'
}

interface ToolChatMessageBase extends ChatMessageBase {
  name?: string
  result?: string
  status?: ChatMessageStatus
  illustration?: ChapterIllustration
  interactive_image?: InteractiveImage
  interactive_images?: InteractiveImage[]
  interactive_image_error?: InteractiveImageError
  interactive_image_status?: InteractiveImageStatus
}

export interface ToolCallChatMessage extends ToolChatMessageBase {
  role: 'tool_call'
  args?: string
  ask?: AgentAskInteraction
}

export interface ToolResultChatMessage extends ToolChatMessageBase {
  role: 'tool_result'
}

export interface AskChatMessage extends ChatMessageBase {
  role: 'ask'
  ask?: AgentAskInteraction
  status?: ChatMessageStatus
}

export interface RuleRollChatMessage extends ChatMessageBase {
  role: 'rule_roll'
  rule_roll?: PublicRuleRoll
}

export interface ContextCompactionChatMessage extends ChatMessageBase {
  role: 'context_compaction'
  status?: ChatMessageStatus
  phase?: string
  attempt?: number
  tokens_before?: number
  tokens_after?: number
  projected_tokens_before?: number
  projected_tokens_after?: number
  reserved_completion_tokens?: number
  reserved_tool_result_tokens?: number
  context_window_tokens?: number
  threshold?: number
  target_ratio?: number
  epoch?: number
  source_message_count?: number
  message_count_before?: number
  message_count_after?: number
  skipped_reason?: string
}

export interface TokenUsageChatMessage extends ChatMessageBase {
  role: 'token_usage'
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

export interface ProposedPlanChatMessage extends ChatMessageBase {
  role: 'proposed_plan'
  status?: ChatMessageStatus
  thinking_preview?: string
  plan_action?: ChatPlanAction
}

export interface SystemChatMessage extends ChatMessageBase {
  role: 'system'
}

export interface ErrorChatMessage extends ChatMessageBase {
  role: 'error'
}

export type ChatMessage =
  | UserChatMessage
  | AssistantChatMessage
  | ThinkingChatMessage
  | ToolCallChatMessage
  | ToolResultChatMessage
  | AskChatMessage
  | RuleRollChatMessage
  | ContextCompactionChatMessage
  | TokenUsageChatMessage
  | ProposedPlanChatMessage
  | SystemChatMessage
  | ErrorChatMessage

export interface AgentAskOption {
  id: string
  label: string
  description?: string
}

export interface AgentAskQuestion {
  id: string
  question: string
  options?: AgentAskOption[]
  multi_select?: boolean
  recommended_option_id?: string
}

export interface AgentAskAnswer {
  question_id: string
  selected_option_ids?: string[]
  custom_input?: string
}

export interface AgentAskSelectedOption {
  id: string
  label: string
}

export interface AgentAskAnswerResult {
  question_id: string
  question: string
  selected_options?: AgentAskSelectedOption[]
  custom_input?: string
}

export interface AgentToolApprovalPresentation {
  mode: 'ask' | 'write' | 'full_access' | string
  tool_name: string
  command?: string
  details?: string
  cwd?: string
  risk: 'low' | 'medium' | 'high' | 'critical' | string
  rule_id: string
  args_hash: string
  can_remember?: boolean
  rule_matcher_version?: number
  rule_match_key?: string
  rule_display_pattern?: string
}

export interface AgentAskInteraction {
  schema: 'ask.pending.v1' | string
  id: string
  kind?: 'question' | 'tool_approval' | string
  tool_call_id: string
  provider_call_id?: string
  task_id?: string
  agent_kind: string
  status: 'pending' | 'answered' | 'cancelled'
  questions: AgentAskQuestion[]
  allow_other?: boolean
  approval?: AgentToolApprovalPresentation
  answers?: AgentAskAnswerResult[]
  cancel_reason?: string
  created_at?: string
  resolved_at?: string
}

export interface AgentAskResolution {
  schema: 'ask.result.v1'
  id: string
  status: 'answered' | 'cancelled'
  answers?: AgentAskAnswerResult[]
  cancel_reason?: string
}

export interface UserMessageReference {
  kind: 'file' | 'lore' | 'style' | 'selection' | 'review_comment'
  id?: string
  label: string
  detail?: string
  start_line?: number
  end_line?: number
}

export interface PublicRuleRoll {
  resolution_id?: string
  label?: string
  difficulty?: string
  dice?: string
  roll_mode?: string
  rolls?: number[]
  kept_roll?: number
  base_target?: number
  target?: number
  bonus_total?: number
  total?: number
  outcome?: string
  result?: string
  cost?: string
  stakes?: string
  state_changes?: PublicRuleStateChange[]
}

export interface PublicRuleStateChange {
  actor_id: string
  field_id: string
  change: number
  reason?: string
}

export interface ChapterIllustration {
  schema: 'chapter_illustration.v1' | string
  chapter_path: string
  image_path: string
  meta_path: string
  markdown: string
  alt_text: string
  profile_id: string
  provider: string
  model: string
  size?: string
  quality?: string
  output_format?: string
  created_at?: string
  revised_prompt?: string
  mime_type?: string
  size_bytes?: number
}

export interface InteractiveImage {
  schema: 'interactive_image.v1' | string
  story_id: string
  branch_id: string
  turn_id: string
  image_path: string
  meta_path: string
  alt_text?: string
  profile_id?: string
  provider?: string
  model?: string
  size?: string
  quality?: string
  output_format?: string
  created_at?: string
  revised_prompt?: string
  mime_type?: string
  size_bytes?: number
}

export interface InteractiveImageError {
  schema: 'interactive_image_error.v1' | string
  story_id?: string
  branch_id?: string
  turn_id?: string
  message?: string
  created_at?: string
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

export interface SessionSummary {
  id: string
  title: string
  created_at: string
  updated_at: string
  active: boolean
  message_count: number
  /** Project-scoped runtimes expose execution state per conversation. */
  running?: boolean
}

export interface IDEContext {
  currentFile?: string
  openFiles?: string[]
}

export interface AgentRunTraceSummary {
  id: string
  created_at: string
  path: string
  status: string
  reason?: string
  events: number
  context_parts: number
  tool_calls?: number
  tool_successes?: number
  tool_blocked?: number
  tool_errors?: number
  tool_truncated?: number
  invalid_tool_args?: number
  tool_domain_accepted?: number
  tool_domain_rejected?: number
  tool_domain_pending?: number
  tool_domain_diagnostics?: number
  llm_calls?: number
  prompt_tokens?: number
  cached_prompt_tokens?: number
  uncached_prompt_tokens?: number
  cache_hit_rate?: number
  duration_ms?: number
  task_id?: string
  agent_kind?: string
  session_id?: string
  phase?: string
  mutations?: number
  verification_status?: string
  recoverable?: boolean
  content_captured?: boolean
}

export interface GlobalAgentRunTraceSummary extends AgentRunTraceSummary {
  project_id: string
  project_name: string
  session_title?: string
  trajectory_uri: string
}

export interface GlobalAgentRunTraceIssue {
  project_id: string
  project_name: string
  message: string
}

export interface GlobalAgentRunTraceCatalog {
  runs: GlobalAgentRunTraceSummary[]
  issues: GlobalAgentRunTraceIssue[]
}

export interface AgentRunTraceRecord {
  type: string
  run_id: string
  created_at: string
  data?: Record<string, unknown>
}

export interface AgentRunTrace {
  summary: AgentRunTraceSummary
  records: AgentRunTraceRecord[]
  truncated?: boolean
}

export interface ContextAnalysisPart {
  id?: string
  source: string
  title: string
  role?: string
  kind?: string
  tool_name?: string
  tool_call_id?: string
  content: string
  note?: string
  bytes: number
  chars: number
  /** Diagnostic source breakdown; content remains the exact model-visible message. */
  parts?: ContextAnalysisPart[]
}

export interface ContextAnalysisCompaction {
  id?: string
  epoch: number
  summary: string
  tokens_before?: number
  tokens_after?: number
  target_ratio?: number
  source_message_count?: number
  source_turn_count?: number
  removable?: boolean
}

export interface ContextAnalysis {
  agent_kind: string
  mode: string
  system_prompt: string
  system_prompt_parts: ContextAnalysisPart[]
  context_parts: ContextAnalysisPart[]
  context_messages: ContextAnalysisPart[]
  message_count: number
  token_estimate?: number
	projected_token_estimate?: number
	reserved_completion_tokens?: number
	reserved_tool_result_tokens?: number
  context_window_tokens?: number
  context_usage_ratio?: number
  compaction_epoch?: number
  compaction_active?: boolean
  would_compact?: boolean
  compaction?: ContextAnalysisCompaction
}

export interface SSEEvent {
  /** Durable streams use the SSE id as their replay cursor. */
  id?: string
  event: string
  data: string
}

export interface BookRecord {
  project_id: string
  name: string
  path: string
  author: string
  cover_updated_at?: string
  last_opened_at: string
}

export type BookSortMode = 'recent' | 'manual'

export interface BookshelfResult {
  books: BookRecord[]
  sort_mode: BookSortMode
}

export interface BookCoverResult {
  schema: 'book_cover.v1' | string
  cover_path: string
  source_path: string
  meta_path: string
  backup_path?: string
  cover_updated_at: string
  image_preset_id?: string
  profile_id: string
  provider: string
  model: string
  size?: string
  quality?: string
  output_format?: string
  created_at?: string
  revised_prompt?: string
  mime_type?: string
  size_bytes?: number
}

export interface ChapterSummary {
  path: string
  file_name: string
  display_title: string
  index: number
  words: number
  status: string
  confirmed: boolean
  updated_at: string
  volume: string
  volume_path: string
}

export interface DocumentPreview {
  path: string
  title: string
  excerpt: string
  words: number
  updated_at: string
}

export interface WorkspaceSummary {
  title: string
  author: string
  chapter_count: number
  total_words: number
  chapters: ChapterSummary[]
  ideas?: DocumentPreview
  outline?: DocumentPreview
  chapter_plans: DocumentPreview[]
}

export interface WorkspaceSearchResult {
  path: string
  line: number
  column: number
  preview: string
  match_text: string
}

export interface WorkspaceReplaceFileResult {
  path: string
  replacements: number
}

export interface WorkspaceReplaceResult {
  workspace: string
  files: WorkspaceReplaceFileResult[]
  total_replacements: number
  skipped: string[]
}

export interface CharacterCardImportResult {
  project_id?: string
  name: string
  target_path: string
  entry_count: number
  item_count: number
  item_ids: string[]
  cover_path?: string
  opening_preset_path?: string
  opening_preset_count: number
  user_placeholder_found: boolean
  user_character_name?: string
  compatibility: CharacterCardCompatibilityReport
  workspace?: string
  book_meta?: BookMeta
  message: string
  resident_lore_bytes: number
  classification_mode: LoreClassificationMode
  classification_counts: Partial<Record<LoreItem['type'], number>>
  uncertain_type_count: number
}

export interface CharacterCardPreview {
  name: string
  entry_count: number
  tags: string[]
  opening_preset_count: number
  user_placeholder_found: boolean
  will_import_cover: boolean
  compatibility: CharacterCardCompatibilityReport
  enabled_entry_count: number
  disabled_entry_count: number
  resident_entry_count: number
  resident_entry_bytes: number
  resident_lore_bytes: number
  auto_entry_count: number
  removed_runtime_entry_count: number
  sanitized_mixed_entry_count: number
  opening_truncated_count: number
  resident_lore_warning: boolean
  resident_lore_warning_threshold_kb: number
  classification_mode: LoreClassificationMode
  classification_counts: Partial<Record<LoreItem['type'], number>>
  uncertain_type_count: number
}

interface CharacterCardCompatibilityReport {
  capabilities: string[]
  sanitized_runtime: string[]
  discarded_extensions: string[]
  warnings: string[]
  ignored_loading_rules: boolean
}

interface NovelImportChapter {
  index: number
  title: string
  chars: number
  path?: string
  volume?: string
  volume_path?: string
}

export interface NovelImportPreview {
  title: string
  language?: string
  chapter_filename_format?: string
  volume_dir_format?: string
  split_strategy: string
  split_regex: string
  sample_chars: number
  chapter_count: number
  total_chars: number
  chapters: NovelImportChapter[]
  warnings?: string[]
}

export interface NovelImportProgress {
  step: string
}

export interface NovelImportResult {
  workspace: string
  book_meta?: BookMeta
  title: string
  chapter_count: number
  total_chars: number
  chapter_paths: string[]
  message: string
}

export interface BookMeta {
  title: string
  author: string
  description: string
  created_at: string
  updated_at: string
}

type VersionSource = 'manual' | 'timer' | 'agent' | 'rollback_backup'

export interface VersionChange {
  path: string
  status: 'added' | 'modified' | 'deleted' | string
}

export interface VersionEntry {
  id: string
  message: string
  created_at: string
  source: VersionSource
  file_count: number
  total_bytes: number
  changed_paths: string[]
}

interface VersionAutoInfo {
  timed_enabled: boolean
  timed_interval_minutes: number
  retention: number
  last_auto_at?: string
}

export interface VersionStatus {
  has_versions: boolean
  clean: boolean
  changes: VersionChange[]
  latest?: VersionEntry
  auto: VersionAutoInfo
}

export interface VersionCommandResult {
  message: string
  version?: VersionEntry
  status?: VersionStatus
}

type VersionRestoreScope = 'workspace' | 'paths'

interface VersionRestoreChange {
  path: string
  status: 'added' | 'modified' | 'deleted'
  text: boolean
  binary: boolean
  missing_in_version?: boolean
  missing_in_workspace?: boolean
}

export interface VersionRestorePlan {
  target: VersionEntry
  scope: VersionRestoreScope
  paths: string[]
  changes: VersionRestoreChange[]
  will_create_backup: boolean
  current_dirty: boolean
  backup_message?: string
  warnings?: string[]
}

export interface VersionRestoreResult {
  message: string
  target: VersionEntry
  version?: VersionEntry
  backup_version?: VersionEntry
  restored_paths: string[]
  scope: VersionRestoreScope
  status?: VersionStatus
}

export interface VersionDiff {
  version: VersionEntry
  base_version?: VersionEntry
  comparison: VersionDiffComparison
  changes: VersionChange[]
  files?: VersionFileDiff[]
  path?: string
  original?: string
  modified?: string
  text: boolean
  binary: boolean
  missing_in_original?: boolean
  missing_in_modified?: boolean
}

export interface VersionFileDiff {
  path: string
  status: VersionChange['status']
  original?: string
  modified?: string
  text: boolean
  binary: boolean
  missing_in_original?: boolean
  missing_in_modified?: boolean
}

export type VersionDiffComparison = 'workspace' | 'parent'

export interface LoreItem {
  id: string
  enabled: boolean
  type: 'character' | 'world' | 'location' | 'faction' | 'rule' | 'item' | 'other'
  type_source: 'heuristic' | 'semantic' | 'manual' | 'legacy'
  name: string
  importance: 'major' | 'important' | 'minor'
  load_mode: 'resident' | 'auto' | 'manual'
  tags: string[]
  brief_description: string
  keywords: string[]
  content: string
  created_at: string
  updated_at: string
  image?: LoreItemImage
  provenance?: {
    kind: string
    source_name: string
    source_record_id: string
    source_hash: string
  }
}

export type LoreClassificationMode = 'heuristic' | 'semantic'

export interface LoreClassificationPreviewRequest {
  item_ids?: string[]
  mode?: LoreClassificationMode
}

export interface LoreClassificationPreviewItem {
  id: string
  name: string
  current_type: LoreItem['type']
  current_type_source: LoreItem['type_source']
  suggested_type: LoreItem['type']
  confidence: 'high' | 'medium' | 'low'
  reason?: string
  suggestion_source: 'heuristic' | 'semantic'
}

export interface LoreClassificationPreview {
  revision: string
  mode: LoreClassificationMode
  items: LoreClassificationPreviewItem[]
  counts: Partial<Record<LoreItem['type'], number>>
  warning?: string
}

export interface LoreClassificationApplyRequest {
  revision: string
  changes: Array<{ id: string; type: LoreItem['type'] }>
}

export interface LoreTypeApplyResult {
  revision: string
  items: LoreItem[]
  updated: LoreItem[]
}

interface LoreItemImage {
  schema: 'lore_item_image.v1' | string
  image_path: string
  meta_path: string
  alt_text?: string
  image_preset_id?: string
  profile_id?: string
  provider?: string
  model?: string
  size?: string
  quality?: string
  output_format?: string
  created_at?: string
  revised_prompt?: string
  mime_type?: string
  size_bytes?: number
}

export interface LoreItemImageGenerateRequest {
  mode?: 'agent' | 'custom'
  command_id?: string
  prompt?: string
  instruction?: string
  image_preset_id?: string
  profile_id?: string
}

export type SkillScope = 'builtin' | 'user' | 'workspace'

export interface SkillScopeInfo {
  scope: SkillScope
  path: string
  writable: boolean
}

export interface SkillSummary {
  name: string
  description: string
  category?: string
  capabilities?: string[]
  context?: string
  agent?: string
  model?: string
  scope: SkillScope
  path: string
  editable: boolean
  active: boolean
  updated_at?: string
}

export interface SkillCreateMetadata {
  category?: string
  capabilities?: string[]
}

export interface SkillFile {
  path: string
  size: number
  entry: boolean
  editable: boolean
  updated_at?: string
}

export interface SkillSnapshot {
  scopes: SkillScopeInfo[]
  skills: SkillSummary[]
}

export interface SkillDocument extends SkillSummary {
  content: string
  revision: string
  files?: SkillFile[]
}

export interface SkillFileDocument {
  skill: SkillSummary
  file: SkillFile
  content: string
  revision: string
}

export interface SkillInstallCandidate {
  id: string
  name?: string
  description?: string
  source_path: string
  conflict: boolean
  invalid_reason?: string
}

export interface SkillInstallPreview {
  candidates: SkillInstallCandidate[]
}

export interface SkillInstallResult {
  installed: SkillSummary[]
}

export type LoreItemInput = Omit<LoreItem, 'created_at' | 'updated_at' | 'provenance'>

type AutomationScope = 'user' | 'workspace'
type AutomationTemplate = 'memory_consolidation' | 'review' | 'continue_writing' | 'custom_prompt'
export type AutomationSessionStrategy = 'per_run' | 'per_task'
type AutomationScheduleKind = 'manual' | 'daily' | 'weekly' | 'monthly' | 'every_hours'
export type AutomationTriggerType = 'manual' | 'schedule' | 'semantic' | 'chapter_batch'
type AutomationActionPolicy = 'confirm' | 'auto_run' | 'notify_only'
export type AutomationNotifyPolicy = 'inbox' | 'silent'
type AutomationInboxStatus = 'pending' | 'dismissed' | 'confirmed' | 'auto_run'
type AutomationInboxPurpose = 'trigger' | 'write_confirmation'

export interface AutomationExecutionTarget {
  kind: 'user' | 'workspace'
  project_id?: string
  workspace?: string
}

interface AutomationSchedule {
  kind: AutomationScheduleKind
  every_hours?: number
  weekday?: number
  day_of_month?: number
  hour: number
  minute: number
  cron?: string
}

export interface AutomationTriggerDefinition {
  id: string
  type: AutomationTriggerType
  enabled: boolean
  name?: string
  action_policy?: AutomationActionPolicy
  notify_policy?: AutomationNotifyPolicy
  schedule?: AutomationSchedule
  semantic_condition?: string
  chapter_batch_size?: number
}

interface AutomationTriggerState {
  last_checked_at?: string
  last_matched_at?: string
  last_evidence_fingerprint?: string
  last_observation_fingerprint?: string
}

export interface AutomationRunRecord {
  id: string
  task_id: string
  task_revision?: string
  session_id?: string
  session_strategy?: AutomationSessionStrategy
  turn_id?: string
  project_id?: string
  scope: AutomationScope
  workspace?: string
  trigger: 'manual' | 'schedule' | 'condition' | 'inbox_confirmation' | 'write_confirmation'
  source_run_id?: string
  trigger_evidence?: AutomationTriggerEvidence[]
  runtime_command_id?: string
  runtime_operation_id?: string
  runtime_receipt_cursor?: number
  status: 'running' | 'success' | 'failed' | 'aborted'
  started_at: string
  finished_at?: string
  summary: string
  error?: string
  output_path?: string
  tool_manifest: Array<{ source: string; allowed: boolean }>
}

export interface AutomationTask {
  id?: string
  catalog_id?: string
  revision?: string
  scope: AutomationScope
  target?: AutomationExecutionTarget
  enabled: boolean
  name: string
  template: AutomationTemplate
  prompt: string
  model_profile_id?: string
  schedule: AutomationSchedule
  triggers: AutomationTriggerDefinition[]
  default_action_policy: AutomationActionPolicy
  trigger_state?: Record<string, AutomationTriggerState>
  session_strategy: AutomationSessionStrategy
  last_run?: AutomationRunRecord
  recent_runs: AutomationRunRecord[]
  created_at?: string
  updated_at?: string
}

/** User-editable task definition. Runtime trigger/run state is intentionally excluded. */
export type AutomationTaskUpdate = Pick<AutomationTask,
  | 'enabled'
  | 'name'
  | 'template'
  | 'prompt'
  | 'model_profile_id'
  | 'schedule'
  | 'triggers'
  | 'default_action_policy'
  | 'session_strategy'
>

/** Complete caller-owned creation input; scheduler/runtime fields cannot cross this seam. */
export type AutomationTaskDefinition = AutomationTaskUpdate & Pick<AutomationTask, 'scope' | 'target'>

export type AutomationTaskTemplateDefaults = Pick<AutomationTask,
  | 'enabled'
  | 'name'
  | 'template'
  | 'prompt'
  | 'model_profile_id'
  | 'schedule'
  | 'triggers'
  | 'default_action_policy'
  | 'session_strategy'
>

export interface AutomationTaskTemplate {
  id: string
  version: number
  description: string
  target_kinds: AutomationExecutionTarget['kind'][]
  defaults: AutomationTaskTemplateDefaults
}

export interface AutomationActiveRun {
  run: AutomationRunRecord
  task_id: string
}

export interface AutomationTriggerEvidence {
  source: string
  title: string
  ref?: string
  snippet?: string
}

export interface AutomationInboxItem {
  id: string
  task_id: string
  trigger_id: string
  purpose?: AutomationInboxPurpose
  scope: AutomationScope
  project_id?: string
  workspace?: string
  status: AutomationInboxStatus
  action_policy: AutomationActionPolicy
  notify_policy: AutomationNotifyPolicy
  title: string
  summary: string
  action_error?: string
  evidence: AutomationTriggerEvidence[]
  fingerprint: string
  run_id?: string
  source_run_id?: string
  created_at: string
  updated_at: string
  read_at?: string
  handled_at?: string
}

export interface AutomationInboxActionResult {
  item: AutomationInboxItem
  run?: AutomationRunRecord
}

export interface TextSelection {
  fileName: string
  startLine: number
  endLine: number
  content: string
}
