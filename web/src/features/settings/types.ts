import type { AgentApprovalMode } from '@/features/agent-approval/modes'

export type { AgentApprovalMode } from '@/features/agent-approval/modes'

export interface Settings {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  openai_context_window_tokens?: number | null
  model_profiles?: ModelProfileSettings[]
  image_api_key?: string
  image_api_base_url?: string
  image_api_model?: string
  default_image_api_profile_id?: string
  image_api_profiles?: ImageAPIProfileSettings[]
  agent_models?: AgentModelSettings
  agent_tools?: AgentToolSettings
  agent_prompts?: AgentPromptSettings
  agent_skills?: AgentSkillSettings
  agent_context?: AgentContextSettings
  general_sub_agents?: AgentGeneralSubAgentSettings
  sub_agents?: SubAgentConfig[]
  web_access?: WebAccessSettings
  skills_dir?: string
  backend_port?: number | null
  frontend_port?: number | null
  allow_lan_access?: boolean | null
  remote_access_username?: string
  remote_access_password?: string
  remote_access_password_set?: boolean
  auto_save_enabled?: boolean | null
  auto_save_interval_ms?: number | null
  hide_novel_chapter_body_in_live_output?: boolean | null
  chapter_filename_format?: string
  volume_dir_format?: string
  max_open_tabs?: number | null
  chapter_group_min?: number | null
  chapter_group_max?: number | null
  version_timed_enabled?: boolean | null
  version_timed_interval_minutes?: number | null
  ui_font_family?: string
  ui_font_size?: number | null
  reading_font_family?: string
  reading_font_size?: number | null
  language?: string
  theme?: string
  motion_intensity?: string
  update_check_enabled?: boolean | null
  max_iteration?: number | null
  model_max_retries?: number | null
  agent_idle_timeout_seconds?: number | null
  agent_tool_result_limit_kb?: number | null
  agent_tool_parallelism?: number | null
  agent_approval_mode?: AgentApprovalMode
  agent_approval_rules?: AgentApprovalRule[]
  shell_environment_mode?: ShellEnvironmentMode
  shell_environment_shell?: string
  agent_bash_path?: string
  terminal_enabled?: boolean | null
  terminal_shell?: string
  terminal_commands?: TerminalCommandSettings[]
  terminal_max_sessions?: number | null
  terminal_scrollback_kb?: number | null
  llm_input_log_enabled?: boolean | null
  trace_capture_level?: string
  trace_exporter?: string
  trace_retention_runs?: number | null
  plan_mode_default?: boolean | null
  ide_story_teller_id?: string
  interactive_story_teller_id?: string
  ide_image_preset_id?: string
  writing_skill_default?: string
  interactive_stage_font_size?: number | null
  interactive_stage_line_height?: number | null
}

export type ShellEnvironmentMode = 'auto' | 'process'

export interface AgentApprovalRule {
  id: string
  scope: 'workspace'
  project_id?: string
  workspace?: string
  tool_name: string
  matcher_version: number
  command_key: string
  command_pattern: string
  approved_args_hash: string
  approved_command: string
  approved_cwd?: string
  source_rule_id?: string
  created_at: string
}

/** User-owned terminal shortcut resolved by stable ID on the backend. */
export interface TerminalCommandSettings {
  id: string
  name: string
  command: string
  enabled: boolean
}

export interface WebAccessSettings {
  searxng_base_url?: string
  search_max_results?: number | null
  search_provider_timeout_seconds?: number | null
  fetch_max_response_kb?: number | null
  fetch_max_content_chars?: number | null
}

export interface ModelProfileSettings {
  id?: string
  name?: string
  provider?: string
  protocol?: string
  api_key?: string
  base_url?: string
  model?: string
  headers?: Record<string, string>
  protocol_options?: Record<string, unknown>
  temperature?: number | null
  context_window_tokens?: number | null
  max_output_tokens?: number | null
}

export interface ModelEndpointPreset {
  base_url?: string
}

export interface ModelProviderPreset {
  id: string
  name: string
  default_protocol: string
  endpoints: Record<string, ModelEndpointPreset>
}

export interface ModelCatalog {
  providers: ModelProviderPreset[]
  protocols: string[]
}

export interface ModelPingResult {
  ok: boolean
  latency_ms: number
  provider: string
  protocol: string
  base_url: string
  model: string
}

export interface ImageAPIProfileSettings {
  id?: string
  name?: string
  provider?: string
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  default_size?: string
  default_quality?: string
  default_output_format?: string
}

export interface AgentModelSettings {
  default?: AgentModelOverride
  general?: AgentModelOverride
  ide?: AgentModelOverride
  interactive_story?: AgentModelOverride
  image?: AgentModelOverride
  config_manager?: AgentModelOverride
  interactive_director?: AgentModelOverride
  version_summary?: AgentModelOverride
  tool_agent?: AgentModelOverride
  automation?: AgentModelOverride
  context_compaction?: AgentModelOverride
}

export interface AgentModelOverride {
  profile_id?: string
  temperature?: number | null
  thinking_level?: string
}

export interface AgentToolSettings {
  default?: AgentToolOverride
  general?: AgentToolOverride
  ide?: AgentToolOverride
  interactive_story?: AgentToolOverride
  image?: AgentToolOverride
  config_manager?: AgentToolOverride
  interactive_director?: AgentToolOverride
  version_summary?: AgentToolOverride
  tool_agent?: AgentToolOverride
  automation?: AgentToolOverride
  context_compaction?: AgentToolOverride
}

export interface AgentSkillSettings {
  default?: AgentSkillOverride
  general?: AgentSkillOverride
  ide?: AgentSkillOverride
  interactive_story?: AgentSkillOverride
  image?: AgentSkillOverride
  config_manager?: AgentSkillOverride
  interactive_director?: AgentSkillOverride
  version_summary?: AgentSkillOverride
  tool_agent?: AgentSkillOverride
  automation?: AgentSkillOverride
  context_compaction?: AgentSkillOverride
}

export type AgentSkillOverride = Record<string, boolean>

interface AgentContextSettings {
  default?: AgentContextOverride
  general?: AgentContextOverride
  ide?: AgentContextOverride
  interactive_story?: AgentContextOverride
  image?: AgentContextOverride
  config_manager?: AgentContextOverride
  interactive_director?: AgentContextOverride
  version_summary?: AgentContextOverride
  tool_agent?: AgentContextOverride
  automation?: AgentContextOverride
  context_compaction?: AgentContextOverride
}

export interface AgentContextOverride {
  compaction_enabled?: boolean | null
  compaction_threshold?: number | null
  tool_result_context_enabled?: boolean | null
  max_fragment_bytes?: number | null
  max_total_injected_bytes?: number | null
  max_fragments?: number | null
  max_metadata_field_bytes?: number | null
  max_provider_input_bytes?: number | null
}

export interface ResolvedAgentContextSettings {
  compaction_enabled: boolean
  compaction_threshold: number
  tool_result_context_enabled: boolean
  max_fragment_bytes: number
  max_total_injected_bytes: number
  max_fragments: number
  max_metadata_field_bytes: number
  max_provider_input_bytes: number
}

interface AgentGeneralSubAgentSettings {
  default?: boolean | null
  general?: boolean | null
  ide?: boolean | null
  interactive_story?: boolean | null
  config_manager?: boolean | null
  automation?: boolean | null
}

export type AgentToolCapability =
  | 'workspace_read'
  | 'workspace_write'
  | 'shell'
  | 'web_search'
  | 'web_fetch'
  | 'browser'
  | 'ask'
  | 'todo'
  | 'skills'
  | 'delegation'
  | 'config_read'
  | 'config_apply'
  | 'event_read'
  | 'lore_read'
  | 'lore_write'
  | 'image_generation'
  | 'context_rewind'

export type AgentToolOverride = Partial<Record<AgentToolCapability, boolean>>

export interface AgentToolDescriptorSummary {
  execution: string
  mutation_scope: string
  post_check: string
  recovery: string
  result_projection: string
  result_retention: string
  steering: string
}

export interface AgentToolCapabilityCatalogEntry {
  capability: AgentToolCapability
  title_key: string
  description_key: string
  tool_names: string[]
  descriptor: AgentToolDescriptorSummary
  available_to_subagents: boolean
}

export type AgentToolAvailability = 'available' | 'runtime_check' | 'unavailable'

export interface ResolvedAgentToolCapability extends AgentToolCapabilityCatalogEntry {
  allowed: boolean
  availability: AgentToolAvailability
  unavailable_reason_key?: string
}

export interface SubAgentConfig {
  id?: string
  name?: string
  description?: string
  system_prompt?: string
  enabled?: boolean | null
  parents?: string[]
  model?: AgentModelOverride
  tools?: AgentToolOverride
}

interface AgentPromptSettings {
  default?: AgentPromptOverride
  general?: AgentPromptOverride
  ide?: AgentPromptOverride
  interactive_story?: AgentPromptOverride
  image?: AgentPromptOverride
  config_manager?: AgentPromptOverride
  interactive_director?: AgentPromptOverride
  version_summary?: AgentPromptOverride
  tool_agent?: AgentPromptOverride
  automation?: AgentPromptOverride
  context_compaction?: AgentPromptOverride
}

export interface AgentPromptOverride {
  flow_prompt?: string
  system_prompt?: string
}

export interface AgentPromptSource {
  id: string
  title: string
  source: string
  content?: string
  editable?: boolean
  field?: 'flow_prompt' | 'system_prompt'
}

interface AgentPromptSourceList {
  sources?: AgentPromptSource[]
}

interface AgentPromptSourceSettings {
  default?: AgentPromptSourceList
  general?: AgentPromptSourceList
  ide?: AgentPromptSourceList
  interactive_story?: AgentPromptSourceList
  image?: AgentPromptSourceList
  config_manager?: AgentPromptSourceList
  interactive_director?: AgentPromptSourceList
  version_summary?: AgentPromptSourceList
  tool_agent?: AgentPromptSourceList
  automation?: AgentPromptSourceList
  context_compaction?: AgentPromptSourceList
}

export interface AgentPromptBlocks {
  runtime_contract?: string
  output_protocol?: string
  editable_system_prompt?: string
}

interface AgentPromptBlockSettings {
  default?: AgentPromptBlocks
  general?: AgentPromptBlocks
  ide?: AgentPromptBlocks
  interactive_story?: AgentPromptBlocks
  image?: AgentPromptBlocks
  config_manager?: AgentPromptBlocks
  interactive_director?: AgentPromptBlocks
  version_summary?: AgentPromptBlocks
  tool_agent?: AgentPromptBlocks
  automation?: AgentPromptBlocks
  context_compaction?: AgentPromptBlocks
}

interface SettingsPaths {
  denova_dir: string
  nova_dir: string
  user_config: string
  workspace_config: string
}

interface SettingsAccess {
  local_url: string
  lan_url: string
}

interface SettingsRuntime {
  goos: string
  dev_mode?: boolean
}

interface SettingsRevisions {
  user?: string
  workspace?: string
}

export interface LayeredSettings {
  default: Settings
  global: Settings
  user: Settings
  workspace: Settings
  effective: Settings
  paths: SettingsPaths
  access?: SettingsAccess
  runtime?: SettingsRuntime
  revisions?: SettingsRevisions
  builtin_agent_prompts?: AgentPromptSettings
  builtin_agent_prompt_blocks?: AgentPromptBlockSettings
  builtin_agent_prompt_sources?: AgentPromptSourceSettings
  agent_tool_capabilities?: AgentToolCapabilityCatalogEntry[]
  resolved_agent_tool_manifests: Partial<Record<Exclude<keyof AgentModelSettings, 'default'>, ResolvedAgentToolCapability[]>>
  resolved_agent_contexts: Partial<Record<Exclude<keyof AgentContextSettings, 'default'>, ResolvedAgentContextSettings>>
}

export type SettingsLayer = 'user' | 'workspace'

interface UpdateAsset {
  name: string
  size: number
  download_url: string
  browser_download_url: string
}

export interface UpdateCheckResult {
  current_version: string
  latest_version: string
  update_available: boolean
  can_install: boolean
  platform: string
  release_url: string
  published_at: string
  release_notes?: string
  asset?: UpdateAsset
  message?: string
}

export interface UpdateInstallResult {
  previous_version: string
  installed_version: string
  status?: 'staged' | 'installed' | string
  installed: boolean
  staged?: boolean
  apply_ready?: boolean
  restart_required: boolean
  backup_path?: string
  staged_path?: string
  apply_log_path?: string
  message?: string
}

export interface UpdateApplyResult {
  status: 'restarting' | string
  version: string
  log_path?: string
}

export interface UpdateInstallProgress {
  phase: 'checking' | 'downloading' | 'verifying' | 'extracting' | 'replacing' | 'staging' | 'staged' | 'installed' | string
  asset_name?: string
  archive_path?: string
  downloaded_bytes?: number
  total_bytes?: number
  percent?: number
  message?: string
}
