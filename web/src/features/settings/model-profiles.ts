import type { ModelProfileSettings } from './types'

export const DEFAULT_MODEL_PROFILE_ID = 'default'

export function modelProfileID(profile?: ModelProfileSettings): string {
  return profile?.id?.trim() || profile?.openai_model?.trim() || ''
}

export function modelProfileLabel(profile?: ModelProfileSettings): string {
  return profile?.name?.trim() || profile?.openai_model?.trim() || modelProfileID(profile)
}

export function defaultModelProfileFromSettings(settings?: {
  openai_api_key?: string
  openai_base_url?: string
  openai_model?: string
  openai_context_window_tokens?: number | null
}): ModelProfileSettings {
  return {
    id: DEFAULT_MODEL_PROFILE_ID,
    openai_api_key: settings?.openai_api_key,
    openai_base_url: settings?.openai_base_url,
    openai_model: settings?.openai_model,
    context_window_tokens: settings?.openai_context_window_tokens,
  }
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
      out.push({ ...defaultProfile, ...profile, id })
    } else if (id) {
      out.push({ ...profile, id })
    }
  }
  if (!hasDefault) {
    out.unshift(defaultProfile)
  }
  return out
}
