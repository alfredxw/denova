export interface Settings {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  model_profiles?: ModelProfileSettings[]
  agent_models?: AgentModelSettings
  agent_tools?: AgentToolSettings
  agent_prompts?: AgentPromptSettings
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
  ui_font_family?: string
  ui_font_size?: number | null
  reading_font_family?: string
  reading_font_size?: number | null
  language?: string
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
  tool_agent?: AgentModelOverride
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
  lore_editor?: AgentToolOverride
  teller_editor?: AgentToolOverride
  interactive_state?: AgentToolOverride
  interactive_hot_choices?: AgentToolOverride
  version_summary?: AgentToolOverride
  tool_agent?: AgentToolOverride
}

export interface AgentToolOverride {
  file_read?: boolean | null
  file_write?: boolean | null
  shell_execute?: boolean | null
  skills?: boolean | null
  lore_read?: boolean | null
  lore_write?: boolean | null
  todo?: boolean | null
}

export interface AgentPromptSettings {
  default?: AgentPromptOverride
  ide?: AgentPromptOverride
  interactive_story?: AgentPromptOverride
  lore_editor?: AgentPromptOverride
  teller_editor?: AgentPromptOverride
  interactive_state?: AgentPromptOverride
  interactive_hot_choices?: AgentPromptOverride
  version_summary?: AgentPromptOverride
  tool_agent?: AgentPromptOverride
}

export interface AgentPromptOverride {
  system_prompt?: string
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
