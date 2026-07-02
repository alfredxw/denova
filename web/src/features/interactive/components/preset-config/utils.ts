export type PresetConfigViewMode = 'visual' | 'json'

const PRESET_CONFIG_VIEW_STORAGE_KEY = 'nova.settingPanel.presetConfigView.v1'

export function formatPresetJSON(value: unknown) {
  return JSON.stringify(value || {}, null, 2)
}

export function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

export function loadPresetConfigViewMode(sectionId: string): PresetConfigViewMode {
  if (typeof window === 'undefined') return 'visual'
  try {
    const raw = window.localStorage.getItem(PRESET_CONFIG_VIEW_STORAGE_KEY)
    if (!raw) return 'visual'
    const parsed = JSON.parse(raw) as Record<string, PresetConfigViewMode>
    return parsed[sectionId] === 'json' ? 'json' : 'visual'
  } catch {
    return 'visual'
  }
}

export function savePresetConfigViewMode(sectionId: string, mode: PresetConfigViewMode) {
  if (typeof window === 'undefined') return
  try {
    const raw = window.localStorage.getItem(PRESET_CONFIG_VIEW_STORAGE_KEY)
    const parsed = raw ? JSON.parse(raw) as Record<string, PresetConfigViewMode> : {}
    window.localStorage.setItem(PRESET_CONFIG_VIEW_STORAGE_KEY, JSON.stringify({ ...parsed, [sectionId]: mode }))
  } catch {
    // View preference is optional; ignore storage failures.
  }
}

export function parseNumberInput(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : undefined
}

export function parseIntegerInput(value: string) {
  const parsed = parseNumberInput(value)
  return parsed === undefined ? undefined : Math.trunc(parsed)
}

export function splitListInput(value: string) {
  return value
    .split(/[，,\n]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export function joinListInput(value: string[] | undefined) {
  return (value || []).join('，')
}

export function nextPresetId(prefix: string) {
  return `${prefix}-${Date.now().toString(36)}`
}

export function cloneWithNewId<T extends { id?: string; name?: string }>(item: T, fallbackPrefix: string): T {
  return {
    ...item,
    id: `${item.id || fallbackPrefix}-copy-${Date.now().toString(36)}`,
    name: item.name ? `${item.name} Copy` : undefined,
  }
}

export function itemKey(item: { id?: string }, index: number, prefix: string) {
  return item.id || `${prefix}-${index}`
}
