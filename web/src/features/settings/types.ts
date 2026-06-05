export interface Settings {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  model_profiles?: ModelProfileSettings[]
  agent_models?: AgentModelSettings
  skills_dir?: string
  auto_save_enabled?: boolean | null
  auto_save_interval_ms?: number | null
  chapter_filename_format?: string
  max_open_tabs?: number | null
  draft_flow_enabled?: boolean | null
  chapter_group_min?: number | null
  chapter_group_max?: number | null
  version_timed_enabled?: boolean | null
  version_timed_interval_minutes?: number | null
  version_agent_enabled?: boolean | null
  version_agent_char_threshold?: number | null
  version_auto_retention?: number | null
  ui_font_family?: string
  reading_font_family?: string
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
}

export interface AgentModelSettings {
  default?: AgentModelOverride
  ide?: AgentModelOverride
  interactive_story?: AgentModelOverride
  lore_editor?: AgentModelOverride
  teller_editor?: AgentModelOverride
  interactive_state?: AgentModelOverride
  interactive_hot_choices?: AgentModelOverride
  version_summary?: AgentModelOverride
}

export interface AgentModelOverride {
  profile_id?: string
  temperature?: number | null
  enable_thinking?: boolean | null
  reasoning_effort?: string
}

export interface SettingsPaths {
  nova_dir: string
  user_config: string
  workspace_config: string
}

export interface LayeredSettings {
  default: Settings
  global: Settings
  user: Settings
  workspace: Settings
  effective: Settings
  paths: SettingsPaths
}

export type SettingsLayer = 'user' | 'workspace'
