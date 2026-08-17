import type { Settings } from '@/features/settings/types'
import { modelProfilesWithDefault } from '@/features/settings/model-profiles'

export function hasUsableLanguageModel(settings: Settings | undefined): boolean {
  return modelProfilesWithDefault(settings).some((profile) =>
    Boolean(profile.api_key?.trim()) && Boolean(profile.model?.trim()),
  )
}
