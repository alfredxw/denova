import type { ImageAPIEndpointSettings, ImageAPIProfileSettings } from './types'

export const DEFAULT_IMAGE_API_PROFILE_ID = 'default'
export const DEFAULT_IMAGE_API_ENDPOINT_ID = 'default'
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

const ENDPOINT_DEFAULTS: Record<ImageAPIProvider, ImageAPIEndpointSettings> = {
  openai: { provider: 'openai', protocol: 'openai-images', base_url: DEFAULT_IMAGE_API_BASE_URL },
  xai: { provider: 'xai', protocol: 'xai-images', base_url: 'https://api.x.ai/v1' },
  comfyui: { provider: 'comfyui', protocol: 'comfyui-workflow', base_url: 'http://127.0.0.1:8188' },
  volcengine: { provider: 'volcengine', protocol: 'ark-images', base_url: 'https://ark.cn-beijing.volces.com/api/v3' },
  google: { provider: 'google', protocol: 'gemini-images', base_url: 'https://generativelanguage.googleapis.com/v1' },
  custom: { provider: 'custom', protocol: 'openai-images' },
}

const PROFILE_DEFAULTS: Record<ImageAPIProvider, ImageAPIProfileSettings> = {
  openai: { model: DEFAULT_IMAGE_API_MODEL, default_quality: 'auto', default_output_format: 'png' },
  xai: { model: 'grok-imagine-image-2.0', default_resolution: '1k', default_quality: 'medium' },
  comfyui: { default_size: '1024x1024', comfyui: { workflow_mode: 'remote' } },
  volcengine: { model: 'doubao-seedream-5-0-260128', default_resolution: '2K', default_output_format: 'png' },
  google: { model: 'gemini-3.1-flash-image', default_resolution: '1K' },
  custom: {},
}

export function imageAPIProvider(value?: string): ImageAPIProvider {
  return value === 'xai' || value === 'comfyui' || value === 'volcengine' || value === 'google' || value === 'custom'
    ? value
    : 'openai'
}

export function imageAPIEndpointID(endpoint?: ImageAPIEndpointSettings): string {
  return endpoint?.id?.trim() || ''
}

export function imageAPIEndpointLabel(endpoint?: ImageAPIEndpointSettings): string {
  return endpoint?.name?.trim() || endpoint?.provider?.trim() || imageAPIEndpointID(endpoint)
}

export function newImageAPIEndpoint(provider: ImageAPIProvider = 'openai'): ImageAPIEndpointSettings {
  return { ...ENDPOINT_DEFAULTS[provider] }
}

export function imageAPIEndpointDefaults(provider?: string): ImageAPIEndpointSettings {
  return newImageAPIEndpoint(imageAPIProvider(provider))
}

export function newImageAPIProfile(provider: ImageAPIProvider = 'openai', endpointID = ''): ImageAPIProfileSettings {
  const defaults = PROFILE_DEFAULTS[provider]
  return {
    ...defaults,
    endpoint_id: endpointID,
    comfyui: defaults.comfyui ? { ...defaults.comfyui } : undefined,
  }
}

export function imageAPIProfileDefaults(provider?: string): ImageAPIProfileSettings {
  return newImageAPIProfile(imageAPIProvider(provider))
}

export function imageAPIProfileID(profile?: ImageAPIProfileSettings): string {
  return profile?.id?.trim() || profile?.model?.trim() || ''
}

export function imageAPIProfileLabel(profile?: ImageAPIProfileSettings): string {
  return profile?.name?.trim()
    || profile?.comfyui?.workflow_name?.trim()
    || profile?.model?.trim()
    || imageAPIProfileID(profile)
}

export function imageAPIEndpointsWithDefault(settings?: {
  image_api_endpoints?: ImageAPIEndpointSettings[]
  default_image_api_profile_id?: string
}): ImageAPIEndpointSettings[] {
  const endpoints = (settings?.image_api_endpoints ?? [])
    .map((endpoint) => ({ ...endpoint, id: imageAPIEndpointID(endpoint), provider: imageAPIProvider(endpoint.provider) }))
    .filter((endpoint) => endpoint.id)
  if (endpoints.some((endpoint) => endpoint.id === DEFAULT_IMAGE_API_ENDPOINT_ID)) return endpoints
  const selectedDefaultID = settings?.default_image_api_profile_id?.trim() || DEFAULT_IMAGE_API_PROFILE_ID
  if (endpoints.length === 0 || selectedDefaultID === DEFAULT_IMAGE_API_PROFILE_ID) {
    return [{ id: DEFAULT_IMAGE_API_ENDPOINT_ID, name: 'Default image endpoint', ...newImageAPIEndpoint() }, ...endpoints]
  }
  return endpoints
}

export function imageAPIProfilesWithDefault(settings?: {
  default_image_api_profile_id?: string
  image_api_profiles?: ImageAPIProfileSettings[]
}): ImageAPIProfileSettings[] {
  const profiles = (settings?.image_api_profiles ?? [])
    .map((profile) => ({ ...profile, id: imageAPIProfileID(profile) }))
    .filter((profile) => profile.id)
  if (profiles.some((profile) => profile.id === DEFAULT_IMAGE_API_PROFILE_ID)) return profiles
  const selectedDefaultID = settings?.default_image_api_profile_id?.trim() || DEFAULT_IMAGE_API_PROFILE_ID
  if (profiles.length === 0 || selectedDefaultID === DEFAULT_IMAGE_API_PROFILE_ID) {
    return [{ id: DEFAULT_IMAGE_API_PROFILE_ID, name: 'Default image model', ...newImageAPIProfile('openai', DEFAULT_IMAGE_API_ENDPOINT_ID) }, ...profiles]
  }
  return profiles
}
