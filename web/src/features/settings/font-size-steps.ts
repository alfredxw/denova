export const DEFAULT_UI_FONT_SIZE = 14
export const DEFAULT_READING_FONT_SIZE = 18

/** Stable persisted pixel values behind the user-facing Dynamic Type-style steps. */
export const UI_FONT_SIZE_STEPS = [11, 12, 13, DEFAULT_UI_FONT_SIZE, 15, 16, 18] as const
export const READING_FONT_SIZE_STEPS = [14, 16, 17, DEFAULT_READING_FONT_SIZE, 20, 23, 28] as const

export const TEXT_SIZE_LABEL_KEYS = [
  'settings.textSize.smallest',
  'settings.textSize.smaller',
  'settings.textSize.small',
  'settings.textSize.default',
  'settings.textSize.large',
  'settings.textSize.larger',
  'settings.textSize.largest',
] as const

export function nearestFontSizeStepIndex(
  value: number | null | undefined,
  steps: readonly number[],
  fallback: number,
) {
  const candidate = typeof value === 'number' && Number.isFinite(value) ? value : fallback
  let nearestIndex = 0
  let nearestDistance = Number.POSITIVE_INFINITY

  steps.forEach((step, index) => {
    const distance = Math.abs(step - candidate)
    if (distance < nearestDistance) {
      nearestDistance = distance
      nearestIndex = index
    }
  })

  return nearestIndex
}
