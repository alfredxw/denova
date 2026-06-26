import type { ImageAPIProfileSettings } from './types'

export const DEFAULT_IMAGE_API_PROFILE_ID = 'default'
export const DEFAULT_IMAGE_API_PROVIDER = 'openai'
export const DEFAULT_IMAGE_API_BASE_URL = 'https://api.openai.com/v1'
export const DEFAULT_IMAGE_API_MODEL = 'gpt-image-1'
export const DEFAULT_IMAGE_API_SIZE = '1024x1024'

export function imageAPIProfileID(profile?: ImageAPIProfileSettings): string {
  return profile?.id?.trim() || profile?.openai_model?.trim() || ''
}

export function imageAPIProfileLabel(profile?: ImageAPIProfileSettings): string {
  return profile?.name?.trim() || profile?.openai_model?.trim() || imageAPIProfileID(profile)
}

export function defaultImageAPIProfileFromSettings(settings?: {
  image_api_key?: string
  image_api_base_url?: string
  image_api_model?: string
}): ImageAPIProfileSettings {
  return {
    id: DEFAULT_IMAGE_API_PROFILE_ID,
    provider: DEFAULT_IMAGE_API_PROVIDER,
    openai_api_key: settings?.image_api_key,
    openai_base_url: settings?.image_api_base_url || DEFAULT_IMAGE_API_BASE_URL,
    openai_model: settings?.image_api_model || DEFAULT_IMAGE_API_MODEL,
    default_size: DEFAULT_IMAGE_API_SIZE,
  }
}

export function imageAPIProfilesWithDefault(settings?: {
  image_api_key?: string
  image_api_base_url?: string
  image_api_model?: string
  image_api_profiles?: ImageAPIProfileSettings[]
}): ImageAPIProfileSettings[] {
  const profiles = settings?.image_api_profiles ?? []
  const defaultProfile = defaultImageAPIProfileFromSettings(settings)
  const out: ImageAPIProfileSettings[] = []
  let hasDefault = false
  for (const profile of profiles) {
    const id = imageAPIProfileID(profile)
    if (id === DEFAULT_IMAGE_API_PROFILE_ID) {
      hasDefault = true
      out.push({ ...defaultProfile, ...profile, id, provider: profile.provider || defaultProfile.provider })
    } else if (id) {
      out.push({ ...profile, id, provider: profile.provider || DEFAULT_IMAGE_API_PROVIDER })
    }
  }
  if (!hasDefault) {
    out.unshift(defaultProfile)
  }
  return out
}
