export interface Settings {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  openai_context_window_tokens?: number | null
  model_profiles?: ModelProfileSettings[]
  agent_models?: AgentModelSettings
  agent_tools?: AgentToolSettings
  agent_prompts?: AgentPromptSettings
  agent_skills?: AgentSkillSettings
  agent_context?: AgentContextSettings
  skills_dir?: string
  backend_port?: number | null
  frontend_port?: number | null
  allow_lan_access?: boolean | null
  remote_access_username?: string
  remote_access_password?: string
  remote_access_password_set?: boolean
  auto_save_enabled?: boolean | null
  auto_save_interval_ms?: number | null
  chapter_filename_format?: string
  volume_dir_format?: string
  max_open_tabs?: number | null
  draft_flow_enabled?: boolean | null
  chapter_group_min?: number | null
  chapter_group_max?: number | null
  version_timed_enabled?: boolean | null
  version_timed_interval_minutes?: number | null
  version_agent_enabled?: boolean | null
  version_agent_char_threshold?: number | null
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
  plan_mode_default?: boolean | null
  ide_story_teller_id?: string
  interactive_max_tokens?: number | null
  interactive_hot_choices_enabled?: boolean | null
  interactive_stage_font_size?: number | null
  interactive_stage_line_height?: number | null
}

export interface ModelProfileSettings {
  id?: string
  name?: string
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  temperature?: number | null
  context_window_tokens?: number | null
}

export interface AgentModelSettings {
  default?: AgentModelOverride
  ide?: AgentModelOverride
  interactive_story?: AgentModelOverride
  config_manager?: AgentModelOverride
  interactive_state?: AgentModelOverride
  interactive_hot_choices?: AgentModelOverride
  version_summary?: AgentModelOverride
  tool_agent?: AgentModelOverride
  automation?: AgentModelOverride
  context_compaction?: AgentModelOverride
}

export interface AgentModelOverride {
  profile_id?: string
  temperature?: number | null
  enable_thinking?: boolean | null
  reasoning_effort?: string
}

export interface AgentToolSettings {
  default?: AgentToolOverride
  ide?: AgentToolOverride
  interactive_story?: AgentToolOverride
  config_manager?: AgentToolOverride
  interactive_state?: AgentToolOverride
  interactive_hot_choices?: AgentToolOverride
  version_summary?: AgentToolOverride
  tool_agent?: AgentToolOverride
  automation?: AgentToolOverride
  context_compaction?: AgentToolOverride
}

export interface AgentSkillSettings {
  default?: AgentSkillOverride
  ide?: AgentSkillOverride
  interactive_story?: AgentSkillOverride
  config_manager?: AgentSkillOverride
  interactive_state?: AgentSkillOverride
  interactive_hot_choices?: AgentSkillOverride
  version_summary?: AgentSkillOverride
  tool_agent?: AgentSkillOverride
  automation?: AgentSkillOverride
  context_compaction?: AgentSkillOverride
}

export type AgentSkillOverride = Record<string, boolean>

export interface AgentContextSettings {
  default?: AgentContextOverride
  ide?: AgentContextOverride
  interactive_story?: AgentContextOverride
  config_manager?: AgentContextOverride
  interactive_state?: AgentContextOverride
  interactive_hot_choices?: AgentContextOverride
  version_summary?: AgentContextOverride
  tool_agent?: AgentContextOverride
  automation?: AgentContextOverride
  context_compaction?: AgentContextOverride
}

export interface AgentContextOverride {
  compaction_enabled?: boolean | null
  compaction_threshold?: number | null
  compaction_recent_turns?: number | null
  compaction_target_min_ratio?: number | null
  compaction_target_max_ratio?: number | null
}

export interface AgentToolOverride {
  file_read?: boolean | null
  file_write?: boolean | null
  shell_execute?: boolean | null
  skills?: boolean | null
  lore_read?: boolean | null
  lore_write?: boolean | null
  todo?: boolean | null
  web_search?: boolean | null
}

export interface AgentPromptSettings {
  default?: AgentPromptOverride
  ide?: AgentPromptOverride
  interactive_story?: AgentPromptOverride
  config_manager?: AgentPromptOverride
  interactive_state?: AgentPromptOverride
  interactive_hot_choices?: AgentPromptOverride
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

export interface AgentPromptSourceList {
  sources?: AgentPromptSource[]
}

export interface AgentPromptSourceSettings {
  default?: AgentPromptSourceList
  ide?: AgentPromptSourceList
  interactive_story?: AgentPromptSourceList
  config_manager?: AgentPromptSourceList
  interactive_state?: AgentPromptSourceList
  interactive_hot_choices?: AgentPromptSourceList
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

export interface AgentPromptBlockSettings {
  default?: AgentPromptBlocks
  ide?: AgentPromptBlocks
  interactive_story?: AgentPromptBlocks
  config_manager?: AgentPromptBlocks
  interactive_state?: AgentPromptBlocks
  interactive_hot_choices?: AgentPromptBlocks
  version_summary?: AgentPromptBlocks
  tool_agent?: AgentPromptBlocks
  automation?: AgentPromptBlocks
  context_compaction?: AgentPromptBlocks
}

export interface SettingsPaths {
  nova_dir: string
  user_config: string
  workspace_config: string
}

export interface SettingsAccess {
  local_url: string
  lan_url: string
}

export interface LayeredSettings {
  default: Settings
  global: Settings
  user: Settings
  workspace: Settings
  effective: Settings
  paths: SettingsPaths
  access?: SettingsAccess
  builtin_agent_prompts?: AgentPromptSettings
  builtin_agent_prompt_blocks?: AgentPromptBlockSettings
  builtin_agent_prompt_sources?: AgentPromptSourceSettings
}

export type SettingsLayer = 'user' | 'workspace'

export interface UpdateAsset {
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
  installed: boolean
  restart_required: boolean
  backup_path?: string
  staged_path?: string
  message?: string
}
