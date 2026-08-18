import type { LucideIcon } from 'lucide-react'
import {
  Bot,
  BookOpen,
  Database,
  FileText,
  FolderOpen,
  Globe2,
  ImagePlus,
  ListChecks,
  MessageSquareText,
  PenLine,
  Search,
  Settings2,
  Terminal,
  Wrench,
} from 'lucide-react'
import type {
  AgentModelSettings,
  AgentSkillSettings,
  AgentToolAvailability,
  AgentToolCapability,
  AgentToolDescriptorSummary,
  ResolvedAgentToolCapability,
} from '@/features/settings/types'
import type { SkillSummary } from '@/lib/api'

type AgentKey = keyof AgentModelSettings
export type VisibleAgentKey = Exclude<AgentKey, 'default'>
export type ToolKey = AgentToolCapability
type AgentCapabilityMode = 'tools' | 'built_in' | 'model_only'
export type SubAgentParentKey = Extract<VisibleAgentKey, 'general' | 'ide' | 'interactive_story' | 'config_manager'>

export interface AgentViewDefinition {
  key: VisibleAgentKey
  titleKey: string
  subtitleKey: string
  groupKey: string
  capabilityMode: AgentCapabilityMode
  icon: LucideIcon
}

export interface AgentToolDefinition {
  key: ToolKey
  titleKey: string
  subtitleKey: string
  toolNames: string[]
  allowed: boolean
  availability: AgentToolAvailability
  unavailableReasonKey?: string
  descriptor: AgentToolDescriptorSummary
  availableToSubAgents: boolean
  icon: LucideIcon
}

export const AGENTS: AgentViewDefinition[] = [
  {
    key: 'general',
    titleKey: 'agents.general.title',
    subtitleKey: 'agents.general.subtitle',
    groupKey: 'agents.group.general',
    capabilityMode: 'tools',
    icon: Bot,
  },
  {
    key: 'ide',
    titleKey: 'agents.ide.title',
    subtitleKey: 'agents.ide.subtitle',
    groupKey: 'agents.group.writing',
    capabilityMode: 'tools',
    icon: PenLine,
  },
  {
    key: 'config_manager',
    titleKey: 'agents.configManager.title',
    subtitleKey: 'agents.configManager.subtitle',
    groupKey: 'agents.group.writing',
    capabilityMode: 'tools',
    icon: Settings2,
  },
  {
    key: 'interactive_story',
    titleKey: 'agents.interactiveStory.title',
    subtitleKey: 'agents.interactiveStory.subtitle',
    groupKey: 'agents.group.interactive',
    capabilityMode: 'tools',
    icon: MessageSquareText,
  },
  {
    key: 'interactive_director',
    titleKey: 'agents.interactiveDirector.title',
    subtitleKey: 'agents.interactiveDirector.subtitle',
    groupKey: 'agents.group.interactive',
    capabilityMode: 'tools',
    icon: FileText,
  },
  {
    key: 'image',
    titleKey: 'agents.image.title',
    subtitleKey: 'agents.image.subtitle',
    groupKey: 'agents.group.utility',
    capabilityMode: 'tools',
    icon: ImagePlus,
  },
  {
    key: 'version_summary',
    titleKey: 'agents.versionSummary.title',
    subtitleKey: 'agents.versionSummary.subtitle',
    groupKey: 'agents.group.version',
    capabilityMode: 'model_only',
    icon: ListChecks,
  },
  {
    key: 'tool_agent',
    titleKey: 'agents.toolAgent.title',
    subtitleKey: 'agents.toolAgent.subtitle',
    groupKey: 'agents.group.utility',
    capabilityMode: 'model_only',
    icon: Wrench,
  },
]

export const SUB_AGENT_PARENT_KEYS: SubAgentParentKey[] = ['general', 'ide', 'interactive_story', 'config_manager']

const TOOL_ICONS: Partial<Record<AgentToolCapability, LucideIcon>> = {
  filesystem_read: Search,
  workspace_write: FileText,
  shell: Terminal,
  web_search: Globe2,
  web_fetch: Globe2,
  browser: Globe2,
  ask: MessageSquareText,
  todo: ListChecks,
  skills: FolderOpen,
  delegation: Wrench,
  config_read: Settings2,
  config_apply: Settings2,
  event_read: BookOpen,
  lore_read: Database,
  lore_write: Wrench,
  image_generation: ImagePlus,
}

// The backend manifest owns capability identity, order, effective policy and
// platform-specific tool names. Icons are intentionally the only UI mapping.
export function toolDefinitionsFromManifest(manifest?: readonly ResolvedAgentToolCapability[]): AgentToolDefinition[] {
  return (manifest ?? []).map((entry) => ({
    key: entry.capability,
    titleKey: entry.title_key,
    subtitleKey: entry.description_key,
    toolNames: [...entry.tool_names],
    allowed: entry.allowed,
    availability: entry.availability,
    unavailableReasonKey: entry.unavailable_reason_key,
    descriptor: entry.descriptor,
    availableToSubAgents: entry.available_to_subagents,
    icon: TOOL_ICONS[entry.capability] ?? Wrench,
  }))
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
