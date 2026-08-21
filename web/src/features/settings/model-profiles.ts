import type { ModelProfileSettings } from './types'

export const DEFAULT_MODEL_PROFILE_ID = 'default'
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

export function modelProfileID(profile?: ModelProfileSettings): string {
  return profile?.id?.trim() || profile?.model?.trim() || ''
}

export function modelProfileLabel(profile?: ModelProfileSettings): string {
  return profile?.name?.trim() || profile?.model?.trim() || modelProfileID(profile)
}

function defaultModelProfileFromSettings(settings?: {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  openai_context_window_tokens?: number | null
}): ModelProfileSettings {
  const provider = inferModelProvider(settings?.openai_base_url)
  return {
    id: DEFAULT_MODEL_PROFILE_ID,
    provider,
    protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS,
    api_key: settings?.openai_api_key,
    base_url: settings?.openai_base_url,
    model: settings?.openai_model,
    context_window_tokens: settings?.openai_context_window_tokens,
  }
}

export function inferModelProvider(baseUrl?: string): string {
  const normalized = baseUrl?.trim().toLowerCase() ?? ''
  if (normalized.includes('api.deepseek.com')) return MODEL_PROVIDER_DEEPSEEK
  if (normalized.includes('api.openai.com')) return MODEL_PROVIDER_OPENAI
  if (normalized.includes('ark.cn-beijing.volces.com')) return MODEL_PROVIDER_VOLCENGINE
  if (normalized.includes('generativelanguage.googleapis.com')) return MODEL_PROVIDER_GOOGLE
  return MODEL_PROVIDER_OPENAI_COMPATIBLE
}

function normalizeModelProfileRouting(profile: ModelProfileSettings, routingSource = profile): ModelProfileSettings {
  const explicitProvider = routingSource.provider?.trim()
  const provider = explicitProvider || inferModelProvider(profile.base_url)
  // Only legacy URL-based profiles are pinned to Chat Completions. Explicit
  // providers leave the protocol empty so the backend preset owns its default.
  const protocol = routingSource.protocol?.trim() || (explicitProvider ? '' : MODEL_PROTOCOL_CHAT_COMPLETIONS)
  return { ...profile, provider, protocol }
}

export function modelProfilesWithDefault(settings?: {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  openai_context_window_tokens?: number | null
  model_profiles?: ModelProfileSettings[]
  agent_models?: { default?: { profile_id?: string } }
}): ModelProfileSettings[] {
  const profiles = settings?.model_profiles ?? []
  const defaultProfile = defaultModelProfileFromSettings(settings)
  const out: ModelProfileSettings[] = []
  let hasDefault = false
  for (const profile of profiles) {
    const id = modelProfileID(profile)
    if (id === DEFAULT_MODEL_PROFILE_ID) {
      hasDefault = true
      const merged = { ...defaultProfile, ...profile, id }
      const explicitProvider = profile.provider?.trim()
      if (explicitProvider && explicitProvider !== defaultProfile.provider && !profile.base_url?.trim()) {
        merged.base_url = ''
      }
      out.push(normalizeModelProfileRouting(merged, profile))
    } else if (id) {
      out.push(normalizeModelProfileRouting({ ...profile, id }))
    }
  }
  const selectedDefaultID = settings?.agent_models?.default?.profile_id?.trim() || DEFAULT_MODEL_PROFILE_ID
  if (!hasDefault && (profiles.length === 0 || selectedDefaultID === DEFAULT_MODEL_PROFILE_ID)) {
    out.unshift(defaultProfile)
  }
  return out
}
