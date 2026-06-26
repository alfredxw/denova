import type { ModelProfileSettings } from './types'

export function modelProfileID(profile?: ModelProfileSettings): string {
  return profile?.id?.trim() || profile?.openai_model?.trim() || ''
}

export function modelProfileLabel(profile?: ModelProfileSettings): string {
  return profile?.name?.trim() || profile?.openai_model?.trim() || modelProfileID(profile)
}
