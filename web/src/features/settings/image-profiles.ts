import type { ImageAPIProfileSettings } from './types'

export const DEFAULT_IMAGE_API_PROFILE_ID = 'default'
export const DEFAULT_IMAGE_API_PROVIDER = 'openai'
export const DEFAULT_IMAGE_API_PROTOCOL = 'openai-images'
export const DEFAULT_IMAGE_API_BASE_URL = 'https://api.openai.com/v1'
export const DEFAULT_IMAGE_API_MODEL = 'gpt-image-2'

export const IMAGE_API_PROTOCOLS = [
  'openai-images',
  'xai-images',
  'ark-images',
  'gemini-images',
  'comfyui-workflow',
] as const

export type ImageAPIProvider = 'openai' | 'xai' | 'comfyui' | 'volcengine' | 'google' | 'custom'

const PROVIDER_DEFAULTS: Record<ImageAPIProvider, ImageAPIProfileSettings> = {
  openai: {
    provider: 'openai', protocol: 'openai-images', base_url: DEFAULT_IMAGE_API_BASE_URL,
    model: DEFAULT_IMAGE_API_MODEL, default_quality: 'auto', default_output_format: 'png',
  },
  xai: {
    provider: 'xai', protocol: 'xai-images', base_url: 'https://api.x.ai/v1',
    model: 'grok-imagine-image-2.0', default_resolution: '1k', default_quality: 'medium',
  },
  comfyui: {
    provider: 'comfyui', protocol: 'comfyui-workflow', base_url: 'http://127.0.0.1:8188',
    default_size: '1024x1024', comfyui: { workflow_mode: 'builtin' },
  },
  volcengine: {
    provider: 'volcengine', protocol: 'ark-images', base_url: 'https://ark.cn-beijing.volces.com/api/v3',
    model: 'doubao-seedream-5-0-260128', default_resolution: '2K', default_output_format: 'png',
  },
  google: {
    provider: 'google', protocol: 'gemini-images', base_url: 'https://generativelanguage.googleapis.com/v1',
    model: 'gemini-3.1-flash-image', default_resolution: '1K',
  },
  custom: { provider: 'custom', protocol: 'openai-images' },
}

export function imageAPIProvider(value?: string): ImageAPIProvider {
  return value === 'xai' || value === 'comfyui' || value === 'volcengine' || value === 'google' || value === 'custom'
    ? value
    : 'openai'
}

export function newImageAPIProfile(provider: ImageAPIProvider = 'openai'): ImageAPIProfileSettings {
  const defaults = PROVIDER_DEFAULTS[provider]
  return {
    ...defaults,
    comfyui: defaults.comfyui ? { ...defaults.comfyui } : undefined,
  }
}

export function imageAPIProviderDefaults(provider?: string): ImageAPIProfileSettings {
  return newImageAPIProfile(imageAPIProvider(provider))
}

export function imageAPIProfileID(profile?: ImageAPIProfileSettings): string {
  return profile?.id?.trim() || profile?.model?.trim() || ''
}

export function imageAPIProfileLabel(profile?: ImageAPIProfileSettings): string {
  return profile?.name?.trim()
    || profile?.model?.trim()
    || profile?.comfyui?.workflow_name?.trim()
    || imageAPIProfileID(profile)
}

export function imageAPIProfilesWithDefault(settings?: {
  image_api_profiles?: ImageAPIProfileSettings[]
}): ImageAPIProfileSettings[] {
  const profiles = settings?.image_api_profiles ?? []
  const out: ImageAPIProfileSettings[] = []
  let hasDefault = false
  for (const profile of profiles) {
    const id = imageAPIProfileID(profile)
    if (!id) continue
    if (id === DEFAULT_IMAGE_API_PROFILE_ID) hasDefault = true
    out.push({ ...profile, id, provider: imageAPIProvider(profile.provider) })
  }
  if (!hasDefault) {
    out.unshift({ id: DEFAULT_IMAGE_API_PROFILE_ID, ...newImageAPIProfile() })
  }
  return out
}
