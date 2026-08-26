import type { ModelEndpointSettings, ModelProfileSettings } from './types'

export const DEFAULT_MODEL_PROFILE_ID = 'default'
export const DEFAULT_MODEL_ENDPOINT_ID = 'default'
export const MODEL_PROVIDER_OPENAI = 'openai'
export const MODEL_PROVIDER_DEEPSEEK = 'deepseek'
export const MODEL_PROVIDER_GOOGLE = 'google'
export const MODEL_PROVIDER_VOLCENGINE = 'volcengine'
export const MODEL_PROVIDER_OPENAI_COMPATIBLE = 'openai-compatible'
export const MODEL_PROTOCOL_CHAT_COMPLETIONS = 'openai-chat-completions'
export const MODEL_PROTOCOL_RESPONSES = 'openai-responses'
export const MODEL_PROTOCOL_ANTHROPIC_MESSAGES = 'anthropic-messages'

export const FALLBACK_MODEL_PROTOCOLS = [
  MODEL_PROTOCOL_CHAT_COMPLETIONS,
  MODEL_PROTOCOL_RESPONSES,
  MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
]

export function modelEndpointID(endpoint?: ModelEndpointSettings): string {
  return endpoint?.id?.trim() || ''
}

export function modelEndpointLabel(endpoint?: ModelEndpointSettings): string {
  return endpoint?.name?.trim() || endpoint?.provider?.trim() || modelEndpointID(endpoint)
}

export function modelProfileID(profile?: ModelProfileSettings): string {
  return profile?.id?.trim() || profile?.model?.trim() || ''
}

export function modelProfileLabel(profile?: ModelProfileSettings): string {
  return profile?.name?.trim() || profile?.model?.trim() || modelProfileID(profile)
}

export function inferModelProvider(baseUrl?: string): string {
  const normalized = baseUrl?.trim().toLowerCase() ?? ''
  if (normalized.includes('api.deepseek.com')) return MODEL_PROVIDER_DEEPSEEK
  if (normalized.includes('api.openai.com')) return MODEL_PROVIDER_OPENAI
  if (normalized.includes('ark.cn-beijing.volces.com')) return MODEL_PROVIDER_VOLCENGINE
  if (normalized.includes('generativelanguage.googleapis.com')) return MODEL_PROVIDER_GOOGLE
  return MODEL_PROVIDER_OPENAI_COMPATIBLE
}

function defaultModelEndpointFromSettings(settings?: {
  openai_api_key?: string
  openai_base_url?: string
}): ModelEndpointSettings {
  return {
    id: DEFAULT_MODEL_ENDPOINT_ID,
    name: 'Default endpoint',
    provider: inferModelProvider(settings?.openai_base_url),
    protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS,
    api_key: settings?.openai_api_key,
    base_url: settings?.openai_base_url,
  }
}

function defaultModelProfileFromSettings(settings?: {
  openai_model?: string
  openai_context_window_tokens?: number | null
}): ModelProfileSettings {
  return {
    id: DEFAULT_MODEL_PROFILE_ID,
    name: 'Default model',
    endpoint_id: DEFAULT_MODEL_ENDPOINT_ID,
    model: settings?.openai_model,
    context_window_tokens: settings?.openai_context_window_tokens,
  }
}

export function modelEndpointsWithDefault(settings?: {
  openai_api_key?: string
  openai_base_url?: string
  model_endpoints?: ModelEndpointSettings[]
  agent_models?: { default?: { profile_id?: string } }
}): ModelEndpointSettings[] {
  const endpoints = (settings?.model_endpoints ?? [])
    .map((endpoint) => ({ ...endpoint, id: modelEndpointID(endpoint) }))
    .filter((endpoint) => endpoint.id)
  if (endpoints.some((endpoint) => endpoint.id === DEFAULT_MODEL_ENDPOINT_ID)) return endpoints
  const selectedDefaultID = settings?.agent_models?.default?.profile_id?.trim() || DEFAULT_MODEL_PROFILE_ID
  if (endpoints.length === 0 || selectedDefaultID === DEFAULT_MODEL_PROFILE_ID) {
    return [defaultModelEndpointFromSettings(settings), ...endpoints]
  }
  return endpoints
}

export function modelProfilesWithDefault(settings?: {
  openai_model?: string
  openai_context_window_tokens?: number | null
  model_profiles?: ModelProfileSettings[]
  agent_models?: { default?: { profile_id?: string } }
}): ModelProfileSettings[] {
  const profiles = (settings?.model_profiles ?? [])
    .map((profile) => ({ ...profile, id: modelProfileID(profile) }))
    .filter((profile) => profile.id)
  if (profiles.some((profile) => profile.id === DEFAULT_MODEL_PROFILE_ID)) return profiles
  const selectedDefaultID = settings?.agent_models?.default?.profile_id?.trim() || DEFAULT_MODEL_PROFILE_ID
  if (profiles.length === 0 || selectedDefaultID === DEFAULT_MODEL_PROFILE_ID) {
    return [defaultModelProfileFromSettings(settings), ...profiles]
  }
  return profiles
}
