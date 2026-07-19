import { useEffect, useMemo } from 'react'
import { normalizeEditorText } from '@/components/Editor/editorDocument'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { getLoreItems, updateLoreItem } from '@/lib/api'
import type { LoreItem } from '@/lib/api'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { isRevisionConflict } from '@/lib/revision-conflict'

export interface LoreAutosaveDraft extends LoreItem {
  tag_draft: string
}

type LoreAutosavePayload = Omit<LoreItem, 'created_at' | 'updated_at' | 'provenance'>

interface LoreItemAutosaveOptions {
  draft: LoreItem | null
  tagDraft: string
  baseline: LoreAutosaveDraft | null
  active: boolean
  workspace: string
  onSaved: (item: LoreItem, submitted: LoreAutosaveDraft) => void
  onAutoSaveError: (error: unknown) => void
}

/** Keeps Lore item autosave policy separate from the large knowledge-library surface. */
export function useLoreItemAutosave({
  draft,
  tagDraft,
  baseline,
  active,
  workspace,
  onSaved,
  onAutoSaveError,
}: LoreItemAutosaveOptions) {
  const autosaveDraft = useMemo<LoreAutosaveDraft | null>(() => draft ? {
    ...draft,
    tag_draft: tagDraft,
  } : null, [draft, tagDraft])

  const autosave = useResourceAutosave<LoreAutosaveDraft, LoreAutosavePayload, LoreItem>({
    draft: autosaveDraft,
    active,
    scopeKey: workspace,
    makePayload: loreAutosavePayload,
    baselineFromSaved: (saved) => loreAutosaveDraft(saved),
    signature: loreResourceSignature,
    save: updateLoreItem,
    resolveConflict: async ({ error, baseline: previous, draft: submitted, baseRevision }) => {
      if (!isRevisionConflict(error)) return null
      const latest = (await getLoreItems()).find((item) => item.id === submitted.id)
      if (!latest) throw new Error(`Lore item ${submitted.id} no longer exists`)
      const latestDraft = loreAutosaveDraft(latest)
      const rebased = await rebaseJSONWithRecovery({
        resource: 'lore_item',
        scope: workspace,
        id: submitted.id,
        baseline: {
          revision: previous?.updated_at || baseRevision || latest.updated_at,
          value: previous ?? latestDraft,
        },
        local: {
          revision: submitted.updated_at || baseRevision,
          value: submitted,
        },
        external: {
          revision: latest.updated_at,
          value: latestDraft,
        },
      })
      return {
        payload: loreAutosavePayload(rebased),
        baseRevision: latest.updated_at,
      }
    },
    onSaved: (item, _mode, submitted) => onSaved(item, submitted),
    onAutoSaveError,
  })

  useEffect(() => {
    autosave.resetBaseline(baseline)
  }, [autosave.resetBaseline, baseline])

  return autosave
}

export function loreAutosavePayload(draft: LoreAutosaveDraft): LoreAutosavePayload {
  const {
    tag_draft: tagDraft,
    created_at: _createdAt,
    updated_at: _updatedAt,
    provenance: _provenance,
    ...item
  } = draft
  return {
    ...item,
    content: normalizeEditorText(item.content || '').trimEnd(),
    tags: splitLoreTags(tagDraft),
  }
}

/** Matches the rich Markdown editor's canonical text so format-only updates never look user-authored. */
export function loreAutosaveDraft(item: LoreItem): LoreAutosaveDraft {
  return {
    ...item,
    content: normalizeEditorText(item.content || ''),
    tags: [...(item.tags || [])],
    tag_draft: (item.tags || []).join('，'),
  }
}

export function loreResourceSignature(value: Partial<LoreAutosaveDraft> | Partial<LoreItem>) {
  const {
    tag_draft: tagDraft,
    created_at: _createdAt,
    updated_at: _updatedAt,
    provenance: _provenance,
    ...item
  } = value as Partial<LoreAutosaveDraft>
  return JSON.stringify({
    ...item,
    content: typeof item.content === 'string' ? normalizeEditorText(item.content) : item.content,
    tags: tagDraft === undefined ? (item.tags || []) : splitLoreTags(tagDraft),
  })
}

function splitLoreTags(value: string) {
  return value
    .split(/[，,]/)
    .map((tag) => tag.trim())
    .filter(Boolean)
}
