export interface StyleRule {
  scene: string
  styles: string[]
}

export interface Settings {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  skills_dir?: string
  auto_save_enabled?: boolean | null
  auto_save_interval_ms?: number | null
  chapter_filename_format?: string
  max_open_tabs?: number | null
  ui_font_family?: string
  reading_font_family?: string
  max_iteration?: number | null
  model_max_retries?: number | null
  plan_mode_default?: boolean | null
  interactive_reply_target_chars?: number | null
  interactive_max_tokens?: number | null
  interactive_stage_font_size?: number | null
  interactive_stage_line_height?: number | null
  style_rules?: StyleRule[] | null
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
