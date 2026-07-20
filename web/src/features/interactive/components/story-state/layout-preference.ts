import type { LedgerFieldGroup } from './model'

export const STORY_STATE_LAYOUT_STORAGE_KEY = 'nova.interactive.storyStateLayout.v1'

export interface StoryStateLayoutGroup {
  key: string
  field_ids: string[]
}

export interface StoryStateTemplateLayout {
  groups: StoryStateLayoutGroup[]
}

export type StoryStateLayouts = Record<string, StoryStateTemplateLayout>

type StoredStoryStateLayouts = Record<string, StoryStateLayouts>

export function defaultStoryStateLayout(groups: LedgerFieldGroup[]): StoryStateTemplateLayout {
  return {
    groups: groups.map((group) => ({
      key: group.key,
      field_ids: group.fields.map((field) => field.id),
    })),
  }
}

/** Reconciles a UI-only preference with the current schema before rendering. */
export function applyStoryStateLayout(groups: LedgerFieldGroup[], preference?: StoryStateTemplateLayout | null): LedgerFieldGroup[] {
  const layout = reconcileStoryStateLayout(groups, preference)
  const groupByKey = new Map(groups.map((group) => [group.key, group]))
  const fieldById = new Map(groups.flatMap((group) => group.fields.map((field) => [field.id, field] as const)))
  return layout.groups.map((group) => ({
    ...groupByKey.get(group.key)!,
    fields: group.field_ids.map((fieldId) => fieldById.get(fieldId)!).filter(Boolean),
  })).filter((group) => group.fields.length > 0)
}

/** Keeps empty current groups available in the editor while pruning stale schema entries. */
export function reconcileStoryStateLayout(groups: LedgerFieldGroup[], preference?: StoryStateTemplateLayout | null): StoryStateTemplateLayout {
  if (!preference) return defaultStoryStateLayout(groups)
  const groupByKey = new Map(groups.map((group) => [group.key, group]))
  const fieldById = new Map(groups.flatMap((group) => group.fields.map((field) => [field.id, field] as const)))
  const saved = normalizeTemplateLayout(preference)
  const orderedKeys = unique([
    ...saved.groups.map((group) => group.key).filter((key) => groupByKey.has(key)),
    ...groups.map((group) => group.key),
  ])
  const savedGroupByKey = new Map(saved.groups.map((group) => [group.key, group]))
  const savedAssignment = new Map<string, string>()
  for (const group of saved.groups) {
    if (!groupByKey.has(group.key)) continue
    for (const fieldId of group.field_ids) {
      if (fieldById.has(fieldId) && !savedAssignment.has(fieldId)) savedAssignment.set(fieldId, group.key)
    }
  }

  return { groups: orderedKeys.map((key) => {
    const defaultGroup = groupByKey.get(key)!
    const savedFieldIds = savedGroupByKey.get(key)?.field_ids || []
    const orderedFieldIds = unique([
      ...savedFieldIds.filter((fieldId) => fieldById.has(fieldId) && savedAssignment.get(fieldId) === key),
      ...defaultGroup.fields
        .map((field) => field.id)
        .filter((fieldId) => !savedAssignment.has(fieldId)),
    ])
    return { key, field_ids: orderedFieldIds }
  }) }
}

export function moveStoryStateLayoutGroup(layout: StoryStateTemplateLayout, activeKey: string, overKey: string): StoryStateTemplateLayout {
  const groups = normalizeTemplateLayout(layout).groups
  const from = groups.findIndex((group) => group.key === activeKey)
  const to = groups.findIndex((group) => group.key === overKey)
  if (from < 0 || to < 0 || from === to) return { groups }
  const next = [...groups]
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return { groups: next }
}

export function moveStoryStateLayoutField(layout: StoryStateTemplateLayout, fieldId: string, targetGroupKey: string, targetIndex: number): StoryStateTemplateLayout {
  const normalized = normalizeTemplateLayout(layout)
  if (!normalized.groups.some((group) => group.key === targetGroupKey)) return normalized
  if (!normalized.groups.some((group) => group.field_ids.includes(fieldId))) return normalized
  const groups = normalized.groups.map((group) => ({
    ...group,
    field_ids: group.field_ids.filter((candidate) => candidate !== fieldId),
  }))
  const target = groups.find((group) => group.key === targetGroupKey)
  if (!target) return normalized
  const index = Math.max(0, Math.min(Math.floor(targetIndex), target.field_ids.length))
  target.field_ids.splice(index, 0, fieldId)
  return { groups }
}

export function readStoryStateLayouts(storyId: string): StoryStateLayouts {
  if (typeof window === 'undefined' || !storyId) return {}
  try {
    const stored = readStoredLayouts()
    return normalizeStoryLayouts(stored[storyId])
  } catch (error) {
    console.warn('[interactive-story-state] failed to read layout preferences', { storyId, key: STORY_STATE_LAYOUT_STORAGE_KEY, error })
    return {}
  }
}

export function writeStoryStateTemplateLayout(storyId: string, templateId: string, layout: StoryStateTemplateLayout | null) {
  if (typeof window === 'undefined' || !storyId || !templateId) return
  try {
    const stored = readStoredLayouts()
    const storyLayouts = normalizeStoryLayouts(stored[storyId])
    if (layout) storyLayouts[templateId] = normalizeTemplateLayout(layout)
    else delete storyLayouts[templateId]
    if (Object.keys(storyLayouts).length > 0) stored[storyId] = storyLayouts
    else delete stored[storyId]
    window.localStorage.setItem(STORY_STATE_LAYOUT_STORAGE_KEY, JSON.stringify(stored))
  } catch (error) {
    console.warn('[interactive-story-state] failed to persist layout preference', { storyId, templateId, key: STORY_STATE_LAYOUT_STORAGE_KEY, error })
  }
}

function readStoredLayouts(): StoredStoryStateLayouts {
  const raw = window.localStorage.getItem(STORY_STATE_LAYOUT_STORAGE_KEY)
  if (!raw) return {}
  const parsed: unknown = JSON.parse(raw)
  if (!isRecord(parsed)) return {}
  return parsed as StoredStoryStateLayouts
}

function normalizeStoryLayouts(value: unknown): StoryStateLayouts {
  if (!isRecord(value)) return {}
  return Object.fromEntries(Object.entries(value)
    .filter(([, layout]) => isRecord(layout))
    .map(([templateId, layout]) => [templateId, normalizeTemplateLayout(layout)]))
}

function normalizeTemplateLayout(value: unknown): StoryStateTemplateLayout {
  if (!isRecord(value) || !Array.isArray(value.groups)) return { groups: [] }
  const seenGroups = new Set<string>()
  const seenFields = new Set<string>()
  const groups: StoryStateLayoutGroup[] = []
  for (const candidate of value.groups) {
    if (!isRecord(candidate)) continue
    const key = typeof candidate.key === 'string' ? candidate.key.trim() : ''
    if (!key || seenGroups.has(key)) continue
    seenGroups.add(key)
    const fieldIds = Array.isArray(candidate.field_ids)
      ? candidate.field_ids
        .map((fieldId) => typeof fieldId === 'string' ? fieldId.trim() : '')
        .filter((fieldId) => {
          if (!fieldId || seenFields.has(fieldId)) return false
          seenFields.add(fieldId)
          return true
        })
      : []
    groups.push({ key, field_ids: fieldIds })
  }
  return { groups }
}

function unique(values: string[]) {
  return Array.from(new Set(values))
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
