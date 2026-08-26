import type { Settings } from '@/features/settings/types'
import { modelEndpointsWithDefault, modelProfilesWithDefault } from '@/features/settings/model-profiles'

export function hasUsableLanguageModel(settings: Settings | undefined): boolean {
  const endpoints = new Map(modelEndpointsWithDefault(settings).map((endpoint) => [endpoint.id?.trim(), endpoint]))
  return modelProfilesWithDefault(settings).some((profile) => {
    const endpoint = endpoints.get(profile.endpoint_id?.trim())
    return Boolean(endpoint?.api_key?.trim()) && Boolean(profile.model?.trim())
  })
}
