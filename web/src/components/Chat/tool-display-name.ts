import type { TFunction } from 'i18next'

const TOOL_NAME_KEYS = {
  unknown_tool: 'chat.tool.name.unknown',
  read: 'chat.tool.name.read',
  glob: 'chat.tool.name.glob',
  grep: 'chat.tool.name.grep',
  write: 'chat.tool.name.write',
  edit: 'chat.tool.name.edit',
  bash: 'chat.tool.name.bash',
  pwsh: 'chat.tool.name.pwsh',
  web_search: 'chat.tool.name.webSearch',
  web_fetch: 'chat.tool.name.webFetch',
  browser: 'chat.tool.name.browser',
  skill: 'chat.tool.name.skill',
  goal_finish: 'chat.tool.name.goalFinish',
  config_read: 'chat.tool.name.configRead',
  config_apply: 'chat.tool.name.configApply',
  list_lore_items: 'chat.tool.name.listLoreItems',
  read_lore_items: 'chat.tool.name.readLoreItems',
  write_lore_items: 'chat.tool.name.writeLoreItems',
  generate_image: 'chat.tool.name.generateImage',
  generate_chapter_illustration: 'chat.tool.name.generateChapterIllustration',
  search_story_history: 'chat.tool.name.searchStoryHistory',
  prepare_interactive_turn: 'chat.tool.name.prepareInteractiveTurn',
  submit_interactive_turn: 'chat.tool.name.submitInteractiveTurn',
  initialize_story_state_schema: 'chat.tool.name.initializeStoryStateSchema',
  submit_director_plan_update: 'chat.tool.name.submitDirectorPlanUpdate',
} as const

/** Localizes built-in tool names while preserving custom tool identities verbatim. */
export function toolDisplayName(name: string, t: TFunction): string {
  const normalizedName = name.trim() || 'unknown_tool'
  const key = TOOL_NAME_KEYS[normalizedName as keyof typeof TOOL_NAME_KEYS]
  return key ? t(key) : normalizedName
}
