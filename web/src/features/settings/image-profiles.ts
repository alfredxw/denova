import type { ImageAPIProfileSettings } from './types'

export const DEFAULT_IMAGE_API_PROFILE_ID = 'default'
export const DEFAULT_IMAGE_API_PROVIDER = 'openai'
export const DEFAULT_IMAGE_API_BASE_URL = 'https://api.openai.com/v1'
export const DEFAULT_IMAGE_API_MODEL = 'gpt-image-1'

// MiniMax 原生图像生成 provider
export const MINIMAX_IMAGE_API_PROVIDER = 'minimax'
export const MINIMAX_IMAGE_API_BASE_URL = 'https://api.minimaxi.com/v1'
export const MINIMAX_IMAGE_API_MODEL = 'image-01'

// 各 provider 的默认配置，切换 provider 时用于预填表单
export const IMAGE_API_PROVIDER_DEFAULTS: Record<string, { baseUrl: string; model: string }> = {
  [DEFAULT_IMAGE_API_PROVIDER]: { baseUrl: DEFAULT_IMAGE_API_BASE_URL, model: DEFAULT_IMAGE_API_MODEL },
  [MINIMAX_IMAGE_API_PROVIDER]: { baseUrl: MINIMAX_IMAGE_API_BASE_URL, model: MINIMAX_IMAGE_API_MODEL },
}

// 所有可选的图像 provider（用于设置页下拉菜单）。
// 品牌名（OpenAI / MiniMax）在所有语言下保持原样，不需要走 i18n。
export const IMAGE_API_PROVIDERS = [
  { value: DEFAULT_IMAGE_API_PROVIDER, label: 'OpenAI' },
  { value: MINIMAX_IMAGE_API_PROVIDER, label: 'MiniMax' },
] as const

export function imageAPIProfileID(profile?: ImageAPIProfileSettings): string {
  return profile?.id?.trim() || profile?.openai_model?.trim() || ''
}

export function imageAPIProfileLabel(profile?: ImageAPIProfileSettings): string {
  return profile?.name?.trim() || profile?.openai_model?.trim() || imageAPIProfileID(profile)
}

function defaultImageAPIProfileFromSettings(settings?: {
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
