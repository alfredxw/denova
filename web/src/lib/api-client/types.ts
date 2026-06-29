export interface ChatMessage {
  type?: 'message' | 'clear'
  role?: 'user' | 'assistant' | 'thinking' | 'tool_call' | 'tool_result' | 'context_compaction' | 'token_usage' | 'system' | 'error'
  content?: string
  id?: string
  turn_id?: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error'
  result?: string
  illustration?: ChapterIllustration
  interactive_image?: InteractiveImage
  interactive_images?: InteractiveImage[]
  interactive_image_error?: InteractiveImageError
  interactive_image_status?: 'running' | 'success' | 'error'
  phase?: string
  attempt?: number
  tokens_before?: number
  tokens_after?: number
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
  created_at?: string
  turn_versions?: { turn_id: string; ts: string; current?: boolean }[]
  turn_version_index?: number
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
  content: string
  note?: string
  bytes: number
  chars: number
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
}

export interface CharacterCardPreview {
  name: string
  entry_count: number
  tags: string[]
  opening_preset_count: number
  user_placeholder_found: boolean
  will_import_cover: boolean
  compatibility: CharacterCardCompatibilityReport
}

export interface CharacterCardCompatibilityReport {
  imported_fields: string[]
  downgraded_fields: string[]
  unsupported_fields: string[]
}

export interface NovelImportChapter {
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

export type VersionSource = 'manual' | 'timer' | 'agent' | 'rollback_backup'

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

export interface VersionAutoInfo {
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
  name: string
  importance: 'major' | 'important' | 'minor'
  load_mode: 'resident' | 'auto' | 'manual'
  tags: string[]
  brief_description: string
  keywords: string[]
  content: string
  created_at: string
  updated_at: string
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

export interface SkillSnapshot {
  scopes: SkillScopeInfo[]
  skills: SkillSummary[]
}

export interface SkillDocument extends SkillSummary {
  content: string
}

export type LoreItemInput = Omit<LoreItem, 'created_at' | 'updated_at'>

export type AutomationScope = 'user' | 'workspace'
export type AutomationTemplate = 'memory_consolidation' | 'review' | 'continue_writing' | 'custom_prompt'
export type AutomationWritePolicy = 'read_only' | 'allow_lore_write' | 'allow_file_write' | 'allow_lore_and_file_write'
export type AutomationWriteMode = 'read_only' | 'confirm_write' | 'auto_write'
export type AutomationWriteScope = 'none' | 'lore' | 'file' | 'lore_and_file'
export type AutomationOutputPolicy = 'run_record_only' | 'optional_file'
export type AutomationScheduleKind = 'manual' | 'daily' | 'weekly' | 'monthly' | 'every_hours'
export type AutomationTriggerType = 'manual' | 'schedule' | 'semantic' | 'chapter_batch'
export type AutomationActionPolicy = 'confirm' | 'auto_run' | 'notify_only'
export type AutomationNotifyPolicy = 'inbox' | 'silent'
export type AutomationInboxStatus = 'pending' | 'dismissed' | 'confirmed' | 'auto_run'
export type AutomationInboxPurpose = 'trigger' | 'write_confirmation'

export interface AutomationSchedule {
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

export interface AutomationTriggerState {
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
  scope: AutomationScope
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

export interface AutomationRunResult {
  task: AutomationTask
  run: AutomationRunRecord
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
