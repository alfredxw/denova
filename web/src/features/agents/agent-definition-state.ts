import { imageAPIProfileID, imageAPIProfileLabel, imageAPIProfilesWithDefault } from '@/features/settings/image-profiles'
import { modelProfileID, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import type {
  AgentSkillOverride,
  AgentSkillPolicy,
  AgentToolOverride,
  CustomAgentConfig,
  ImageAPIProfileSettings,
  LayeredSettings,
  ModelProfileSettings,
  Settings,
  SettingsLayer,
} from '@/features/settings/types'
import { runtimeKindForContract } from './agent-contracts'
import { mergeAgentModelOverride, mergeAgentPromptOverride } from './agent-configuration-sections'

type Translate = (key: string, options?: Record<string, unknown>) => string
type ProfileOption = { id: string; label: string }

export function resolveInheritedImageProfileID(layered: LayeredSettings | null, layer: SettingsLayer): string {
  let value = 'default'
  for (const settings of inheritedLayers(layered, layer)) {
    const candidate = settings?.default_image_api_profile_id?.trim()
    if (candidate) value = candidate
  }
  return value
}

export function buildProfileOptions(draft: Settings, effective: Settings, t: Translate): ProfileOption[] {
  const profiles = new Map<string, string>()
  function add(profile?: ModelProfileSettings): void {
    const id = modelProfileID(profile)
    if (id) profiles.set(id, modelProfileLabel(profile))
  }
  modelProfilesWithDefault(effective).forEach(add)
  ;(draft.model_profiles ?? []).forEach(add)
  if (!profiles.has('default')) profiles.set('default', t('agents.option.defaultModel'))
  return formatProfileOptions(profiles, t)
}

export function buildImageProfileOptions(draft: Settings, effective: Settings, t: Translate): ProfileOption[] {
  const profiles = new Map<string, string>()
  function add(profile?: ImageAPIProfileSettings): void {
    const id = imageAPIProfileID(profile)
    if (id) profiles.set(id, imageAPIProfileLabel(profile))
  }
  imageAPIProfilesWithDefault(effective).forEach(add)
  ;(draft.image_api_profiles ?? []).forEach(add)
  return formatProfileOptions(profiles, t)
}

export function skillOverrideToPolicy(value: AgentSkillOverride): AgentSkillPolicy {
  const pinned: string[] = []
  const blocked: string[] = []
  for (const [name, enabled] of Object.entries(value)) {
    if (enabled) pinned.push(name)
    else if (enabled === false) blocked.push(name)
  }
  return { mode: 'managed', pinned, blocked }
}

export function cloneBuiltInAgent(
  seed: CustomAgentConfig & { id: string },
  layered: LayeredSettings,
  effective: Settings,
): CustomAgentConfig & { id: string } {
  const runtimeKind = runtimeKindForContract(seed.contract) ?? 'ide'
  const prompt = mergeAgentPromptOverride(effective.agent_prompts?.default ?? {}, effective.agent_prompts?.[runtimeKind] ?? {})
  const builtInFlow = layered.builtin_agent_prompt_sources?.[runtimeKind]?.sources?.find((source) => source.field === 'flow_prompt')?.content
    ?? layered.builtin_agent_prompt_blocks?.[runtimeKind]?.editable_system_prompt
    ?? ''
  const instructions = [prompt.flow_prompt?.trim() || builtInFlow.trim(), prompt.system_prompt?.trim()]
    .filter(Boolean)
    .join('\n\n## Additional Agent Rules\n\n')
  const model = mergeAgentModelOverride(effective.agent_models?.default ?? {}, effective.agent_models?.[runtimeKind] ?? {})
  const manifest = layered.resolved_agent_tool_manifests?.[runtimeKind] ?? []
  const tools = Object.fromEntries(manifest.map((entry) => [entry.capability, entry.allowed])) as AgentToolOverride
  const skillOverrides = {
    ...(effective.agent_skills?.default ?? {}),
    ...(effective.agent_skills?.[runtimeKind] ?? {}),
  }
  const context = layered.resolved_agent_contexts?.[runtimeKind]

  return {
    ...seed,
    instructions,
    model: { ...model, profile_id: model.profile_id || 'default' },
    tools,
    tool_guidance: {},
    skill_policy: skillOverrideToPolicy(skillOverrides),
    runtime_context: context ? {
      compaction_enabled: context.compaction_enabled,
      compaction_threshold: context.compaction_threshold,
      tool_result_context_enabled: context.tool_result_context_enabled,
      max_fragment_bytes: context.max_fragment_bytes,
      max_total_injected_bytes: context.max_total_injected_bytes,
      max_fragments: context.max_fragments,
      max_metadata_field_bytes: context.max_metadata_field_bytes,
      max_provider_input_bytes: context.max_provider_input_bytes,
    } : {},
    context_bindings: [],
    delegation: { mode: 'compatible', agent_ids: [] },
    image_api_profile_id: runtimeKind === 'image' ? effective.default_image_api_profile_id || 'default' : undefined,
  }
}

function inheritedLayers(layered: LayeredSettings | null, layer: SettingsLayer): Array<Settings | undefined> {
  return layer === 'workspace'
    ? [layered?.default, layered?.global, layered?.user]
    : [layered?.default, layered?.global]
}

function formatProfileOptions(profiles: Map<string, string>, t: Translate): ProfileOption[] {
  return Array.from(profiles.entries()).map(([id, label]) => ({
    id,
    label: id === 'default' ? t('agents.option.defaultProfile', { label }) : t('agents.option.profile', { id, label }),
  }))
}
