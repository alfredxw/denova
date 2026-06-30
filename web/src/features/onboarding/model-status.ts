import type { Settings } from '@/features/settings/types'
import { modelProfilesWithDefault } from '@/features/settings/model-profiles'

export function hasUsableLanguageModel(settings: Settings | undefined): boolean {
  return modelProfilesWithDefault(settings).some((profile) =>
    Boolean(profile.openai_api_key?.trim()) && Boolean(profile.openai_model?.trim()),
  )
}
