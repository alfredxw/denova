import type { TFunction } from 'i18next'
import type { ImagePreset } from './types'

const BUILTIN_IMAGE_PRESET_IDS = new Set(['game-cg', 'realistic', '2d-illustration'])

/** Resolve localized image preset name while preserving custom user presets. */
export function imagePresetName(
  preset: Pick<ImagePreset, 'id'> & Partial<Pick<ImagePreset, 'name' | 'custom' | 'builtin_overridden'>>,
  t: TFunction,
): string {
  if (preset.custom || preset.builtin_overridden) {
    return preset.name || preset.id
  }

  if (BUILTIN_IMAGE_PRESET_IDS.has(preset.id)) {
    const key = `settingPanel.imagePreset.builtin.${preset.id}.name`
    const localized = t(key, { defaultValue: '' })
    if (localized) return localized

    // Legacy fallback mapping
    if (preset.id === 'game-cg') return t('settingPanel.imagePreset.builtin.game-cg.name', { defaultValue: 'Игровой CG' })
    if (preset.id === 'realistic') return t('settingPanel.imagePreset.builtin.realistic.name', { defaultValue: 'Реализм' })
    if (preset.id === '2d-illustration') return t('settingPanel.imagePreset.builtin.2d-illustration.name', { defaultValue: '2D Иллюстрация' })
  }

  return preset.name || preset.id
}
