import type { Settings, ModelProfileSettings, ModelEndpointSettings, ImageAPIProfileSettings, ImageAPIEndpointSettings } from './types'
import { DEFAULT_MODEL_PROFILE_ID, modelEndpointID, modelEndpointsWithDefault, modelProfileID, modelProfilesWithDefault } from './model-profiles'
import { DEFAULT_IMAGE_API_PROFILE_ID, imageAPIEndpointID, imageAPIEndpointsWithDefault, imageAPIProfileID, imageAPIProfilesWithDefault } from './image-profiles'

export function modelProfilesForEditor(draft: Settings, effective: Settings): ModelProfileSettings[] {
  const localProfiles = draft.model_profiles ?? []
  const hasLocalDefault = localProfiles.some((profile) => modelProfileID(profile) === DEFAULT_MODEL_PROFILE_ID)
  const hasLocalDefaultSelection = Boolean(draft.agent_models?.default?.profile_id?.trim())
  const hasLegacyDefault = Boolean(draft.openai_api_key || draft.openai_base_url || draft.openai_model || draft.openai_context_window_tokens)
  if (hasLocalDefault || hasLocalDefaultSelection || hasLegacyDefault) {
    return preserveDraftOnlyModelProfiles(modelProfilesWithDefault(draft), localProfiles)
  }
  const inherited = modelProfilesWithDefault(effective)
  const localIDs = new Set(localProfiles.map(modelProfileID).filter(Boolean))
  return [
    ...inherited.filter((profile) => !localIDs.has(modelProfileID(profile))),
    ...localProfiles,
  ]
}

export function modelEndpointsForEditor(draft: Settings, effective: Settings): ModelEndpointSettings[] {
  const localEndpoints = draft.model_endpoints ?? []
  const hasLocalDefault = localEndpoints.some((endpoint) => modelEndpointID(endpoint) === 'default')
  const hasLocalDefaultSelection = Boolean(draft.agent_models?.default?.profile_id?.trim())
  const hasLegacyDefault = Boolean(draft.openai_api_key || draft.openai_base_url)
  if (hasLocalDefault || hasLocalDefaultSelection || hasLegacyDefault) return modelEndpointsWithDefault(draft)
  const inherited = modelEndpointsWithDefault(effective)
  const localIDs = new Set(localEndpoints.map(modelEndpointID).filter(Boolean))
  return [
    ...inherited.filter((endpoint) => !localIDs.has(modelEndpointID(endpoint))).map(stripInheritedModelEndpointSecret),
    ...localEndpoints,
  ]
}

function preserveDraftOnlyModelProfiles(profiles: ModelProfileSettings[], draftProfiles: ModelProfileSettings[]): ModelProfileSettings[] {
  const draftOnlyProfiles = draftProfiles.filter((profile) => !modelProfileID(profile))
  if (draftOnlyProfiles.length === 0) return profiles
  return [...profiles, ...draftOnlyProfiles]
}

function stripInheritedModelEndpointSecret(endpoint: ModelEndpointSettings): ModelEndpointSettings {
  return { ...endpoint, api_key: '' }
}

export function imageAPIProfilesForEditor(draft: Settings, effective: Settings): ImageAPIProfileSettings[] {
  const localProfiles = draft.image_api_profiles ?? []
  const hasLocalDefault = localProfiles.some((profile) => imageAPIProfileID(profile) === DEFAULT_IMAGE_API_PROFILE_ID)
  const hasLocalDefaultSelection = Boolean(draft.default_image_api_profile_id?.trim())
  if (hasLocalDefault || hasLocalDefaultSelection) {
    return imageAPIProfilesWithDefault(draft)
  }
  const inherited = imageAPIProfilesWithDefault(effective)
  const localIDs = new Set(localProfiles.map(imageAPIProfileID).filter(Boolean))
  return [
    ...inherited.filter((profile) => !localIDs.has(imageAPIProfileID(profile))),
    ...localProfiles,
  ]
}

export function imageAPIEndpointsForEditor(draft: Settings, effective: Settings): ImageAPIEndpointSettings[] {
  const localEndpoints = draft.image_api_endpoints ?? []
  const hasLocalDefault = localEndpoints.some((endpoint) => imageAPIEndpointID(endpoint) === 'default')
  const hasLocalDefaultSelection = Boolean(draft.default_image_api_profile_id?.trim())
  if (hasLocalDefault || hasLocalDefaultSelection) return imageAPIEndpointsWithDefault(draft)
  const inherited = imageAPIEndpointsWithDefault(effective)
  const localIDs = new Set(localEndpoints.map(imageAPIEndpointID).filter(Boolean))
  return [
    ...inherited.filter((endpoint) => !localIDs.has(imageAPIEndpointID(endpoint))).map(stripInheritedImageAPIEndpointSecret),
    ...localEndpoints,
  ]
}

function stripInheritedImageAPIEndpointSecret(endpoint: ImageAPIEndpointSettings): ImageAPIEndpointSettings {
  return { ...endpoint, api_key: '' }
}
