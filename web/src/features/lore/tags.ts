export const LORE_PROTAGONIST_TAG = '主角'

export function splitLoreTags(value: string): string[] {
  return value
    .split(/[，,]/)
    .map((tag) => tag.trim())
    .filter(Boolean)
}

export function isLoreProtagonistTag(tag: string): boolean {
  const normalized = tag.trim().toLowerCase()
  return normalized === LORE_PROTAGONIST_TAG || normalized === 'protagonist'
}

export function hasLoreProtagonistTag(tags: readonly string[]): boolean {
  return tags.some(isLoreProtagonistTag)
}

export function toggleLoreProtagonistTag(tags: readonly string[]): string[] {
  if (hasLoreProtagonistTag(tags)) return tags.filter((tag) => !isLoreProtagonistTag(tag))
  return [...tags, LORE_PROTAGONIST_TAG]
}
