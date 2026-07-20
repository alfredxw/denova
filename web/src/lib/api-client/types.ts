// MessageItem render model. Agent API/history/stream payloads use AgentUIMessage;
// this shape remains for the render adapter and local legacy interactive state.
export interface ChatMessage {
  type?: 'message' | 'clear'
  role?: 'user' | 'assistant' | 'thinking' | 'tool_call' | 'tool_result' | 'rule_roll' | 'context_compaction' | 'token_usage' | 'plan_question' | 'proposed_plan' | 'system' | 'error'
  content?: string
  id?: string
  render_key?: string
  streaming_target_content?: string
  turn_id?: string
  navigation_turn_id?: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error'
  result?: string
  illustration?: ChapterIllustration
  interactive_image?: InteractiveImage
  interactive_images?: InteractiveImage[]
  interactive_image_error?: InteractiveImageError
  interactive_image_status?: 'running' | 'success' | 'error'
  rule_roll?: PublicRuleRoll
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
  streaming?: boolean
  thinking_preview?: string
  plan_action?: 'answered' | 'approved' | 'continue' | 'exited'
  created_at?: string
  turn_versions?: { turn_id: string; ts: string; current?: boolean }[]
  turn_version_index?: number
  user_references?: UserMessageReference[]
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
  event: string
  data: string
}

export interface FileOperationResult {
  path: string
  message: string
}

export interface CreateFileRequest {
  path: string
  type: 'file' | 'dir'
  content?: string
}

export interface CopyMoveRequest {
  from: string
  to: string
}

export interface RenameRequest {
  path: string
  new_name: string
}

export interface BookRecord {
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
  agent_enabled: boolean
  agent_char_threshold: number
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
  changes: VersionChange[]
  path?: string
  original?: string
  modified?: string
  text: boolean
  binary: boolean
  missing_in_version?: boolean
  missing_in_workspace?: boolean
}

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
  instruction?: string
  image_preset_id?: string
  profile_id?: string
}

export interface LoreImagesGenerateRequest extends LoreItemImageGenerateRequest {
  item_ids: string[]
  overwrite_existing?: boolean
}

export interface LoreImageProgressEvent {
  item_id: string
  index: number
  total: number
  status: 'running' | 'skipped' | 'success' | 'error'
  message?: string
  item?: LoreItem
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
  context?: string
  agent?: string
  model?: string
  scope: SkillScope
  path: string
  editable: boolean
  active: boolean
  updated_at?: string
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
type AutomationWritePolicy = 'read_only' | 'allow_lore_write' | 'allow_file_write' | 'allow_lore_and_file_write'
type AutomationWriteMode = 'read_only' | 'confirm_write' | 'auto_write'
type AutomationWriteScope = 'none' | 'lore' | 'file' | 'lore_and_file'
type AutomationOutputPolicy = 'run_record_only' | 'optional_file'
type AutomationScheduleKind = 'manual' | 'daily' | 'weekly' | 'monthly' | 'every_hours'
export type AutomationTriggerType = 'manual' | 'schedule' | 'semantic' | 'chapter_batch'
type AutomationActionPolicy = 'confirm' | 'auto_run' | 'notify_only'
export type AutomationNotifyPolicy = 'inbox' | 'silent'
type AutomationInboxStatus = 'pending' | 'dismissed' | 'confirmed' | 'auto_run'
type AutomationInboxPurpose = 'trigger' | 'write_confirmation'

export interface AutomationExecutionTarget {
  kind: 'user' | 'workspace'
  workspace_id?: string
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
  session_id?: string
  scope: AutomationScope
  workspace?: string
  trigger: 'manual' | 'schedule' | 'condition' | 'inbox_confirmation' | 'write_confirmation'
  source_run_id?: string
  trigger_evidence?: AutomationTriggerEvidence[]
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
  write_policy?: AutomationWritePolicy
  write_mode: AutomationWriteMode
  write_scope: AutomationWriteScope
  output_policy: AutomationOutputPolicy
  output_path: string
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
  | 'write_mode'
  | 'write_scope'
  | 'output_policy'
  | 'output_path'
>

export type AutomationTaskTemplateDefaults = Pick<AutomationTask,
  | 'enabled'
  | 'name'
  | 'template'
  | 'prompt'
  | 'model_profile_id'
  | 'schedule'
  | 'triggers'
  | 'default_action_policy'
  | 'write_mode'
  | 'write_scope'
  | 'output_policy'
  | 'output_path'
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
  workspace?: string
  status: AutomationInboxStatus
  action_policy: AutomationActionPolicy
  notify_policy: AutomationNotifyPolicy
  title: string
  summary: string
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
