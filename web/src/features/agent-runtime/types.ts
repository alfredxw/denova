export type AgentEngineID = 'native' | 'codex' | 'claude'
/** API profiles and CLI models are exclusive sources; secrets remain in endpoints. */
export type RuntimeModelSettings = { model: string; effort?: string; profile_id?: never } | { profile_id: string; model?: never; effort?: never }
export type CodexSandbox = 'read-only' | 'workspace-write' | 'danger-full-access'
export type CodexRuntimeSettings = RuntimeModelSettings & { sandbox?: CodexSandbox }
export type ClaudeRuntimeSettings = RuntimeModelSettings
export interface RuntimePreferences { selected?: AgentEngineID; codex?: CodexRuntimeSettings; claude?: ClaudeRuntimeSettings }
export type RuntimeSelection = { kind: 'native' } | { kind: 'codex'; codex: CodexRuntimeSettings } | { kind: 'claude'; claude: ClaudeRuntimeSettings }
export interface EngineCapabilities {
  ask_user: boolean
  cancel: boolean
  interactive_approval: boolean
  delegation: boolean
  goal: boolean
  queue: boolean
  steer: boolean
  pause: boolean
}
export type ConfigurationSectionID = 'shared.instructions' | 'shared.skills' | 'shared.context_sources' | 'shared.input_budget'
  | 'native.model' | 'native.permissions' | 'native.context_policy' | 'native.checkpoint' | 'native.subagents'
  | 'codex.model' | 'codex.execution_policy' | 'claude.model' | 'claude.execution_policy'
export interface ConfigurationSection {
  id: ConfigurationSectionID
  owner: 'shared' | AgentEngineID
  state: 'editable' | 'read_only' | 'inactive' | 'unavailable'
  reason_key?: string
}
export interface AgentConfiguration { selected: AgentEngineID; sections: ConfigurationSection[]; tool_manifest?: import('@/features/settings/types').ResolvedAgentToolCapability[] }
export interface EngineDescriptor {
  id: AgentEngineID
  name_key: string
  status: 'unchecked' | 'not_installed' | 'auth_required' | 'ready' | 'unavailable' | 'incompatible'
  reason_key?: string
  capabilities: EngineCapabilities
  configuration_sections: ConfigurationSection[]
}
export interface EngineModel { id: string; display_name: string; efforts: string[]; default_effort?: string }
export interface EngineModels { items: EngineModel[]; default_id?: string }

/** Selectors inherit independently; model/effort is one atomic engine branch. */
export function resolveRuntimePreferences(parent?: RuntimePreferences, own?: RuntimePreferences): RuntimePreferences {
  return { selected: own?.selected ?? parent?.selected ?? 'native', codex: own?.codex ?? parent?.codex, claude: own?.claude ?? parent?.claude }
}

/** Project only the selected branch; retained settings never enter execution. */
export function runtimeSelection(preferences: RuntimePreferences): RuntimeSelection | null {
  switch (preferences.selected ?? 'native') {
    case 'native': return { kind: 'native' }
    case 'codex': return preferences.codex ? { kind: 'codex', codex: preferences.codex } : null
    case 'claude': return preferences.claude ? { kind: 'claude', claude: preferences.claude } : null
  }
}

export function runtimeModel(selection?: RuntimeSelection) {
  if (!selection || selection.kind === 'native') return undefined
  return selection.kind === 'codex' ? selection.codex : selection.claude
}

export function runtimeModelKey(settings?: RuntimeModelSettings): string {
  return settings?.profile_id ? `profile:${settings.profile_id}` : settings?.model ? `cli:${settings.model}` : ''
}

export function runtimeModelFromKey(key: string): RuntimeModelSettings {
  return key.startsWith('profile:') ? { profile_id: key.slice(8) } : { model: key.slice(4) }
}
