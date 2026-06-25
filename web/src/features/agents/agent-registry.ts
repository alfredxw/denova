import type { ElementType } from 'react'
import { Archive, Clock, Database, FileText, FolderOpen, Globe2, ListChecks, MessageSquareText, PenLine, Search, Settings2, Shield, Sparkles, Terminal, Wrench } from 'lucide-react'
import type { AgentModelSettings, AgentSkillSettings, AgentToolOverride } from '@/features/settings/types'
import type { SkillSummary } from '@/lib/api'

export type AgentKey = keyof AgentModelSettings
export type VisibleAgentKey = Exclude<AgentKey, 'default'>
export type ToolKey = keyof AgentToolOverride
export type AgentCapabilityMode = 'tools' | 'built_in' | 'model_only'
export type DeepAgentParentKey = Extract<VisibleAgentKey, 'ide' | 'interactive_story' | 'config_manager' | 'automation'>

export interface AgentViewDefinition {
  key: VisibleAgentKey
  titleKey: string
  subtitleKey: string
  groupKey: string
  capabilityMode: AgentCapabilityMode
  icon: ElementType
}

export interface AgentToolDefinition {
  key: ToolKey
  titleKey: string
  subtitleKey: string
  icon: ElementType
}

export const AGENTS: AgentViewDefinition[] = [
  { key: 'ide', titleKey: 'agents.ide.title', subtitleKey: 'agents.ide.subtitle', groupKey: 'agents.group.writing', capabilityMode: 'tools', icon: PenLine },
  { key: 'config_manager', titleKey: 'agents.configManager.title', subtitleKey: 'agents.configManager.subtitle', groupKey: 'agents.group.writing', capabilityMode: 'tools', icon: Settings2 },
  { key: 'interactive_story', titleKey: 'agents.interactiveStory.title', subtitleKey: 'agents.interactiveStory.subtitle', groupKey: 'agents.group.interactive', capabilityMode: 'tools', icon: MessageSquareText },
  { key: 'interactive_state', titleKey: 'agents.interactiveState.title', subtitleKey: 'agents.interactiveState.subtitle', groupKey: 'agents.group.interactive', capabilityMode: 'model_only', icon: Shield },
  { key: 'interactive_hot_choices', titleKey: 'agents.interactiveHotChoices.title', subtitleKey: 'agents.interactiveHotChoices.subtitle', groupKey: 'agents.group.interactive', capabilityMode: 'model_only', icon: Sparkles },
  { key: 'version_summary', titleKey: 'agents.versionSummary.title', subtitleKey: 'agents.versionSummary.subtitle', groupKey: 'agents.group.version', capabilityMode: 'model_only', icon: ListChecks },
  { key: 'tool_agent', titleKey: 'agents.toolAgent.title', subtitleKey: 'agents.toolAgent.subtitle', groupKey: 'agents.group.utility', capabilityMode: 'model_only', icon: Wrench },
  { key: 'automation', titleKey: 'agents.automation.title', subtitleKey: 'agents.automation.subtitle', groupKey: 'agents.group.utility', capabilityMode: 'tools', icon: Clock },
  { key: 'context_compaction', titleKey: 'agents.contextCompaction.title', subtitleKey: 'agents.contextCompaction.subtitle', groupKey: 'agents.group.utility', capabilityMode: 'model_only', icon: Archive },
]

export const DEEP_AGENT_PARENT_KEYS: DeepAgentParentKey[] = ['ide', 'interactive_story', 'config_manager', 'automation']

export const TOOL_ROWS: AgentToolDefinition[] = [
  { key: 'file_read', titleKey: 'agents.tool.fileRead.title', subtitleKey: 'agents.tool.fileRead.subtitle', icon: Search },
  { key: 'web_search', titleKey: 'agents.tool.webSearch.title', subtitleKey: 'agents.tool.webSearch.subtitle', icon: Globe2 },
  { key: 'file_write', titleKey: 'agents.tool.fileWrite.title', subtitleKey: 'agents.tool.fileWrite.subtitle', icon: FileText },
  { key: 'shell_execute', titleKey: 'agents.tool.shellExecute.title', subtitleKey: 'agents.tool.shellExecute.subtitle', icon: Terminal },
  { key: 'skills', titleKey: 'agents.tool.skills.title', subtitleKey: 'agents.tool.skills.subtitle', icon: FolderOpen },
  { key: 'lore_read', titleKey: 'agents.tool.loreRead.title', subtitleKey: 'agents.tool.loreRead.subtitle', icon: Database },
  { key: 'lore_write', titleKey: 'agents.tool.loreWrite.title', subtitleKey: 'agents.tool.loreWrite.subtitle', icon: Wrench },
  { key: 'todo', titleKey: 'agents.tool.todo.title', subtitleKey: 'agents.tool.todo.subtitle', icon: ListChecks },
  { key: 'agent_config_read', titleKey: 'agents.tool.agentConfigRead.title', subtitleKey: 'agents.tool.agentConfigRead.subtitle', icon: Settings2 },
  { key: 'agent_config_write', titleKey: 'agents.tool.agentConfigWrite.title', subtitleKey: 'agents.tool.agentConfigWrite.subtitle', icon: Settings2 },
]

export const BASE_TOOL_VALUES: Required<AgentToolOverride> = {
  file_read: true,
  web_search: true,
  file_write: true,
  shell_execute: true,
  skills: true,
  lore_read: true,
  lore_write: true,
  todo: true,
  agent_config_read: false,
  agent_config_write: false,
}

export const FALLBACK_AGENT_TOOL_VALUES: Record<VisibleAgentKey, Required<AgentToolOverride>> = {
  ide: { file_read: true, web_search: true, file_write: true, shell_execute: true, skills: true, lore_read: true, lore_write: true, todo: true, agent_config_read: false, agent_config_write: false },
  interactive_story: { file_read: true, web_search: false, file_write: true, shell_execute: true, skills: true, lore_read: true, lore_write: false, todo: false, agent_config_read: false, agent_config_write: false },
  config_manager: { file_read: true, web_search: true, file_write: true, shell_execute: false, skills: true, lore_read: true, lore_write: true, todo: true, agent_config_read: true, agent_config_write: true },
  interactive_state: disabledTools(),
  interactive_hot_choices: disabledTools(),
  version_summary: disabledTools(),
  tool_agent: disabledTools(),
  automation: { file_read: true, web_search: true, file_write: true, shell_execute: false, skills: true, lore_read: true, lore_write: true, todo: true, agent_config_read: false, agent_config_write: false },
  context_compaction: disabledTools(),
}

export function skillAvailableForAgent(skill: Pick<SkillSummary, 'name' | 'agent'>, agentKey: VisibleAgentKey, settings?: AgentSkillSettings) {
  const explicit = settings?.[agentKey]?.[skill.name] ?? settings?.default?.[skill.name]
  if (explicit !== undefined) return explicit
  return skillAgentFieldMatches(skill.agent, agentKey)
}

export function skillAgentFieldMatches(agentField: string | undefined, agentKey: VisibleAgentKey) {
  const value = (agentField || '').trim()
  if (!value) return true
  return value
    .split(/[,\s;]+/)
    .map((part) => part.trim())
    .filter(Boolean)
    .some((part) => part === '*' || part.toLowerCase() === 'all' || part === agentKey)
}

export function disabledTools(): Required<AgentToolOverride> {
  return { file_read: false, web_search: false, file_write: false, shell_execute: false, skills: false, lore_read: false, lore_write: false, todo: false, agent_config_read: false, agent_config_write: false }
}

export function resolveEffectiveTools(defaultTools: AgentToolOverride, tools: AgentToolOverride): Required<AgentToolOverride> {
  return {
    file_read: tools.file_read ?? defaultTools.file_read ?? BASE_TOOL_VALUES.file_read,
    web_search: tools.web_search ?? defaultTools.web_search ?? BASE_TOOL_VALUES.web_search,
    file_write: tools.file_write ?? defaultTools.file_write ?? BASE_TOOL_VALUES.file_write,
    shell_execute: tools.shell_execute ?? defaultTools.shell_execute ?? BASE_TOOL_VALUES.shell_execute,
    skills: tools.skills ?? defaultTools.skills ?? BASE_TOOL_VALUES.skills,
    lore_read: tools.lore_read ?? defaultTools.lore_read ?? BASE_TOOL_VALUES.lore_read,
    lore_write: tools.lore_write ?? defaultTools.lore_write ?? BASE_TOOL_VALUES.lore_write,
    todo: tools.todo ?? defaultTools.todo ?? BASE_TOOL_VALUES.todo,
    agent_config_read: tools.agent_config_read ?? defaultTools.agent_config_read ?? BASE_TOOL_VALUES.agent_config_read,
    agent_config_write: tools.agent_config_write ?? defaultTools.agent_config_write ?? BASE_TOOL_VALUES.agent_config_write,
  }
}
