import type { ModelProfileSettings } from './types'

export const DEFAULT_MODEL_PROFILE_ID = 'default'
export const MODEL_PROVIDER_OPENAI = 'openai'
export const MODEL_PROVIDER_DEEPSEEK = 'deepseek'
export const MODEL_PROVIDER_OPENAI_COMPATIBLE = 'openai-compatible'
export const MODEL_PROTOCOL_CHAT_COMPLETIONS = 'openai-chat-completions'
export const MODEL_PROTOCOL_RESPONSES = 'openai-responses'

export function modelProfileID(profile?: ModelProfileSettings): string {
  return profile?.id?.trim() || profile?.openai_model?.trim() || ''
}

export function modelProfileLabel(profile?: ModelProfileSettings): string {
  return profile?.name?.trim() || profile?.openai_model?.trim() || modelProfileID(profile)
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
    openai_api_key: settings?.openai_api_key,
    openai_base_url: settings?.openai_base_url,
    openai_model: settings?.openai_model,
    context_window_tokens: settings?.openai_context_window_tokens,
  }
}

export function inferModelProvider(baseUrl?: string): string {
  const normalized = baseUrl?.trim().toLowerCase() ?? ''
  if (normalized.includes('api.deepseek.com')) return MODEL_PROVIDER_DEEPSEEK
  if (normalized.includes('api.openai.com')) return MODEL_PROVIDER_OPENAI
  return MODEL_PROVIDER_OPENAI_COMPATIBLE
}

export function defaultModelProviderBaseURL(provider?: string): string {
  switch (provider?.trim()) {
    case MODEL_PROVIDER_OPENAI:
      return 'https://api.openai.com/v1'
    case MODEL_PROVIDER_DEEPSEEK:
      return 'https://api.deepseek.com'
    default:
      return ''
  }
}

function normalizeModelProfileRouting(profile: ModelProfileSettings, routingSource = profile): ModelProfileSettings {
  const explicitProvider = routingSource.provider?.trim()
  const provider = explicitProvider || inferModelProvider(profile.openai_base_url)
  const protocol = routingSource.protocol?.trim() || (
    explicitProvider === MODEL_PROVIDER_OPENAI
      ? MODEL_PROTOCOL_RESPONSES
      : MODEL_PROTOCOL_CHAT_COMPLETIONS
  )
  return { ...profile, provider, protocol }
}

export function modelProfilesWithDefault(settings?: {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  openai_context_window_tokens?: number | null
  model_profiles?: ModelProfileSettings[]
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
      if (explicitProvider && explicitProvider !== defaultProfile.provider && !profile.openai_base_url?.trim()) {
        merged.openai_base_url = defaultModelProviderBaseURL(explicitProvider)
      }
      out.push(normalizeModelProfileRouting(merged, profile))
    } else if (id) {
      out.push(normalizeModelProfileRouting({ ...profile, id }))
    }
  }
  if (!hasDefault) {
    out.unshift(defaultProfile)
  }
  return out
}
