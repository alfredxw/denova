import { modelProfileID, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import type { ModelProfileSettings, Settings } from '@/features/settings/types'

type Translate = (key: string, options?: Record<string, unknown>) => string

export function buildAutomationModelProfileOptions(settings: Settings | null, selectedID: string | undefined, t: Translate): Array<{ id: string; label: string }> {
  const labels = modelProfileLabels(settings, t)
  const selected = selectedID?.trim()
  if (selected && !labels.has(selected)) {
    labels.set(selected, t('automations.model.unknownProfile', { id: selected }))
  }
  return Array.from(labels.entries()).map(([id, label]) => ({
    id,
    label: id === 'default' ? t('automations.model.defaultProfile', { label }) : t('automations.model.profile', { id, label }),
  }))
}

export function inheritedAutomationModelProfileLabel(settings: Settings | null, t: Translate) {
  const labels = modelProfileLabels(settings, t)
  const inheritedID = settings?.agent_models?.automation?.profile_id || settings?.agent_models?.default?.profile_id || 'default'
  return labels.get(inheritedID) || t('automations.model.unknownProfile', { id: inheritedID })
}

function modelProfileLabels(settings: Settings | null, t: Translate) {
  const profiles = new Map<string, string>()
  const add = (profile?: ModelProfileSettings) => {
    const id = modelProfileID(profile)
    if (!id) return
    profiles.set(id, modelProfileLabel(profile))
  }
  modelProfilesWithDefault(settings ?? undefined).forEach(add)
  if (!profiles.has('default')) profiles.set('default', t('automations.model.defaultModel'))
  return profiles
}
