import type { PresetResourceKind } from '@/features/interactive/preset-ownership'

export type ConfigurationPageOrigin = 'skills' | 'agents' | 'automation' | 'lore' | 'teller'

export interface ConfigurationPageContext {
  origin: ConfigurationPageOrigin
  resourceId?: string
  storyId?: string
  branchId?: string
  context?: Record<string, string>
}

const BUILTIN_AGENT_COMMAND = /^\/(?:new|compact|status|help|goal|plan)(?:\s|$)/i
const CONFIGURATION_CONTEXT_FIELD_LIMIT = 24
const CONFIGURATION_CONTEXT_VALUE_LIMIT = 2048
const PRESET_CONFIG_RESOURCES: Record<PresetResourceKind, string> = {
  teller: 'narrative_style',
  director: 'game_planning',
  event: 'event_package',
  rule: 'rule_system',
  'actor-state': 'state_system',
  image: 'image_preset',
}

/** Attach the Configuration Skill and bounded page provenance to one ordinary Project Agent turn. */
export function buildConfigurationAgentMessage(message: string, page: ConfigurationPageContext): string {
  const userMessage = message.trim()
  if (!userMessage || BUILTIN_AGENT_COMMAND.test(userMessage)) return userMessage

  // UI selection keys also scope quick prompts and drafts. Translate them only
  // at the model boundary so they cannot be mistaken for tool resource names/IDs.
  const presetKind = page.context?.active_resource_kind as PresetResourceKind | undefined
  const resource = page.origin === 'teller' && presetKind && Object.hasOwn(PRESET_CONFIG_RESOURCES, presetKind)
    ? PRESET_CONFIG_RESOURCES[presetKind]
    : undefined
  const resourceId = (resource ? page.context?.active_resource_id : page.resourceId)?.trim()
  const pageContext: Record<string, string> = {}
  const entries = Object.entries(page.context || {})
    .map(([key, value]) => [key.trim(), String(value ?? '').trim()] as const)
    .filter(([key, value]) => Boolean(key && value))
    .filter(([key]) => !resource || (key !== 'active_resource_kind' && key !== 'active_resource_id'))
    .sort(([left], [right]) => left.localeCompare(right))
    .slice(0, CONFIGURATION_CONTEXT_FIELD_LIMIT)
  for (const [key, value] of entries) {
    pageContext[key.slice(0, 128)] = value.slice(0, CONFIGURATION_CONTEXT_VALUE_LIMIT)
  }

  const metadata = {
    source: 'Denova configuration page',
    purpose: 'Manage the configuration resource currently visible to the user.',
    origin: page.origin === 'teller' ? 'presets' : page.origin.trim(),
    ...(resource ? { resource } : {}),
    ...(resourceId ? { resource_id: resourceId } : {}),
    ...(page.storyId?.trim() ? { story_id: page.storyId.trim() } : {}),
    ...(page.branchId?.trim() ? { branch_id: page.branchId.trim() } : {}),
    ...(Object.keys(pageContext).length ? { page_context: pageContext } : {}),
  }
  const invocation = /(^|\s)\/configuration(?:\s|$)/i.test(userMessage)
    ? userMessage
    : `/configuration\n\n${userMessage}`
  return `${invocation}\n\n[Configuration Page Context]\n${JSON.stringify(metadata, null, 2)}`
}
