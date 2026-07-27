import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import {
  createLoreItem,
  getLoreItems,
  type LoreItem,
  type LoreItemInput,
} from '@/lib/api'
import { rebaseJSONValue } from '@/lib/three-way-rebase'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import type { DocumentReviewSnapshot } from '@/components/Editor/documentReviewAnchors'
import { firstVisibleLoreItemId } from './knowledge-sections'
import {
  LORE_UPDATED_EVENT,
  notifyLoreUpdated,
  type LoreUpdatedDetail,
} from './events'
import {
  loreAutosaveDraft,
  useLoreItemAutosave,
  type LoreAutosaveDraft,
} from './use-lore-item-autosave'

const SELECTION_STORAGE_PREFIX = 'nova.lore.workspace.selection:'
const EVENT_SOURCE = 'writing-lore-tab'

interface UseLoreWorkspaceOptions {
  workspace: string
  onFlushHandlerChange: (handler: EditorFlushHandler | null) => void
}

/** State and persistence boundary for the focused writing-workspace lore tab. */
export function useLoreWorkspace({
  workspace,
  onFlushHandlerChange,
}: UseLoreWorkspaceOptions) {
  const { t } = useTranslation()
  const [items, setItems] = useState<LoreItem[]>([])
  const [activeId, setActiveId] = useState('')
  const [draft, setDraft] = useState<LoreItem | null>(null)
  const [tagDraft, setTagDraft] = useState('')
  const [baseline, setBaseline] = useState<LoreAutosaveDraft | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const requestRef = useRef(0)
  const rebaseSequenceRef = useRef(0)
  const draftRef = useRef<LoreItem | null>(null)
  const tagDraftRef = useRef('')
  const baselineRef = useRef<LoreAutosaveDraft | null>(null)

  draftRef.current = draft
  tagDraftRef.current = tagDraft
  baselineRef.current = baseline

  const applyCanonicalItems = useCallback(
    async (nextItems: LoreItem[], preferredID?: string) => {
      const sequence = ++rebaseSequenceRef.current
      const storedID = preferredID || readSelectedLoreID(workspace)
      const nextID = nextItems.some((item) => item.id === storedID)
        ? storedID
        : firstVisibleLoreItemId(nextItems) || ''
      const nextItem = nextItems.find((item) => item.id === nextID) || null
      const nextBaseline = nextItem ? loreAutosaveDraft(nextItem) : null
      const current = draftRef.current
      const previousBaseline = baselineRef.current
      let rebasedFromDraft = current
      let rebasedFromTags = tagDraftRef.current
      let rebasedFromAutosave = current && nextItem && current.id === nextItem.id
        ? {
          ...current,
          tags: [...(current.tags || [])],
          tag_draft: tagDraftRef.current,
        }
        : null
      if (current && previousBaseline?.id === current.id && !nextItems.some((item) => item.id === current.id)) {
        const deletedLocal = {
          ...current,
          tags: [...(current.tags || [])],
          tag_draft: tagDraftRef.current,
        }
        // A remote deletion cannot be merged into an editable item. Archive a
        // dirty local copy before moving to the next available resource.
        await rebaseJSONWithRecovery<LoreAutosaveDraft | null>({
          resource: 'lore_item',
          scope: workspace,
          id: current.id,
          baseline: { revision: previousBaseline.updated_at, value: previousBaseline },
          local: { revision: previousBaseline.updated_at, value: deletedLocal },
          external: { revision: 'deleted', value: null },
        })
      }
      let rebased = nextBaseline
        ? previousBaseline?.id === nextBaseline.id && rebasedFromAutosave
          ? await rebaseJSONWithRecovery({
              resource: 'lore_item',
              scope: workspace,
              id: nextBaseline.id,
              baseline: { revision: previousBaseline.updated_at, value: previousBaseline },
              local: { revision: previousBaseline.updated_at, value: rebasedFromAutosave },
              external: { revision: nextBaseline.updated_at, value: nextBaseline },
            })
          : nextBaseline
        : null
      // Conflict archival can be asynchronous. Rebase edits made while it is
      // pending before publishing the canonical state, so no keystroke is lost.
      while (sequence === rebaseSequenceRef.current && rebased && rebasedFromAutosave?.id === rebased.id) {
        const latestDraft = draftRef.current
        const latestTags = tagDraftRef.current
        if (!latestDraft || latestDraft.id !== rebased.id) break
        if (Object.is(latestDraft, rebasedFromDraft) && latestTags === rebasedFromTags) break
        const latestAutosave = {
          ...latestDraft,
          tags: [...(latestDraft.tags || [])],
          tag_draft: latestTags,
        }
        rebased = await rebaseJSONWithRecovery({
          resource: 'lore_item',
          scope: workspace,
          id: rebased.id,
          baseline: { revision: rebasedFromAutosave.updated_at, value: rebasedFromAutosave },
          local: { revision: rebasedFromAutosave.updated_at, value: latestAutosave },
          external: { revision: rebased.updated_at, value: rebased },
        })
        rebasedFromDraft = latestDraft
        rebasedFromTags = latestTags
        rebasedFromAutosave = latestAutosave
      }
      if (sequence !== rebaseSequenceRef.current) return
      setItems(nextItems)
      setActiveId(nextID)
      persistSelectedLoreID(workspace, nextID)
      if (rebased) {
        const { tag_draft: nextTags, ...nextDraft } = rebased
        setDraft(nextDraft)
        setTagDraft(nextTags)
      } else {
        setDraft(null)
        setTagDraft('')
      }
      setBaseline(nextBaseline)
    },
    [workspace],
  )

  const reload = useCallback(
    async (preferredID?: string) => {
      const request = ++requestRef.current
      if (!workspace) {
        await applyCanonicalItems([])
        setLoading(false)
        return
      }
      setLoading(true)
      setError('')
      try {
        const nextItems = await getLoreItems(workspace)
        if (request !== requestRef.current) return
        await applyCanonicalItems(nextItems, preferredID)
      } catch (cause) {
        if (request !== requestRef.current) return
        console.error('[LoreWorkspaceTab] failed to load lore items', {
          workspace,
          cause,
        })
        setError(cause instanceof Error ? cause.message : String(cause))
      } finally {
        if (request === requestRef.current) setLoading(false)
      }
    },
    [applyCanonicalItems, workspace],
  )

  const autosave = useLoreItemAutosave({
    draft,
    tagDraft,
    baseline,
    active: Boolean(workspace && draft),
    workspace,
    onSaved: (saved, submitted) => {
      const savedBaseline = loreAutosaveDraft(saved)
      const current = draftRef.current
      const local =
        current?.id === saved.id
          ? {
              ...current,
              tags: [...(current.tags || [])],
              tag_draft: tagDraftRef.current,
            }
          : submitted
      const rebased = rebaseJSONValue(submitted, local, savedBaseline)
      const { tag_draft: nextTags, ...nextDraft } = rebased
      setItems((currentItems) =>
        currentItems.map((item) => (item.id === saved.id ? saved : item)),
      )
      setDraft(nextDraft)
      setTagDraft(nextTags)
      setBaseline(savedBaseline)
      notifyLoreUpdated({ ids: [saved.id], source: EVENT_SOURCE })
    },
    onAutoSaveError: (cause) => {
      console.error('[LoreWorkspaceTab] failed to autosave lore item', {
        workspace,
        itemID: draftRef.current?.id,
        cause,
      })
      toast.error(
        cause instanceof Error ? cause.message : t('editor.saveFailed'),
      )
    },
  })

  const flush = useCallback(async (): Promise<boolean> => {
    if (!draftRef.current) return true
    try {
      await autosave.saveNow('auto')
      return true
    } catch (cause) {
      console.error('[LoreWorkspaceTab] failed to flush lore draft', {
        workspace,
        itemID: draftRef.current?.id,
        cause,
      })
      toast.error(
        cause instanceof Error ? cause.message : t('editor.saveFailed'),
      )
      return false
    }
  }, [autosave.saveNow, t, workspace])

  useEffect(() => {
    onFlushHandlerChange(flush)
    return () => onFlushHandlerChange(null)
  }, [flush, onFlushHandlerChange])

  useEffect(() => {
    setItems([])
    setActiveId('')
    setDraft(null)
    setTagDraft('')
    setBaseline(null)
    rebaseSequenceRef.current += 1
    void reload()
    return () => {
      requestRef.current += 1
    }
  }, [reload, workspace])

  useEffect(() => {
    const onLoreUpdated = (event: Event) => {
      const detail = (event as CustomEvent<LoreUpdatedDetail>).detail
      if (detail?.source === EVENT_SOURCE) return
      void reload(activeId)
    }
    window.addEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
    return () => window.removeEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
  }, [activeId, reload])

  const selectItem = useCallback(
    async (id: string) => {
      if (id === activeId) return true
      if (!(await flush())) return false
      const item = items.find((entry) => entry.id === id)
      if (!item) return false
      rebaseSequenceRef.current += 1
      const nextBaseline = loreAutosaveDraft(item)
      setActiveId(id)
      setDraft({ ...item, tags: [...(item.tags || [])] })
      setTagDraft((item.tags || []).join('，'))
      setBaseline(nextBaseline)
      persistSelectedLoreID(workspace, id)
      return true
    },
    [activeId, flush, items, workspace],
  )

  const createItem = useCallback(
    async (input: Partial<LoreItemInput>) => {
      if (!(await flush())) return null
      try {
        const created = await createLoreItem(workspace, input)
        const nextBaseline = loreAutosaveDraft(created)
        rebaseSequenceRef.current += 1
        // Preserve any state committed by the flush above. Building a list from
        // this callback's render-time snapshot could roll the just-saved item
        // back in the UI while leaving the server canonical state unchanged.
        setItems((current) => [
          ...current.filter((item) => item.id !== created.id),
          created,
        ])
        setActiveId(created.id)
        setDraft({ ...created, tags: [...(created.tags || [])] })
        setTagDraft((created.tags || []).join('，'))
        setBaseline(nextBaseline)
        persistSelectedLoreID(workspace, created.id)
        notifyLoreUpdated({ ids: [created.id], source: EVENT_SOURCE })
        return created
      } catch (cause) {
        console.error('[LoreWorkspaceTab] failed to create lore item', {
          workspace,
          cause,
        })
        toast.error(
          cause instanceof Error ? cause.message : t('settingPanel.saveFailed'),
        )
        return null
      }
    },
    [flush, t, workspace],
  )

  const prepareSnapshot =
    useCallback(async (): Promise<DocumentReviewSnapshot> => {
      const itemID = draftRef.current?.id
      if (!itemID || !(await flush()))
        throw new Error('The lore draft could not be saved')
      const canonical = (await getLoreItems(workspace)).find(
        (item) => item.id === itemID,
      )
      if (!canonical?.updated_at)
        throw new Error('The canonical lore snapshot is unavailable')
      setItems((current) =>
        current.map((item) => (item.id === canonical.id ? canonical : item)),
      )
      return {
        content: canonical.content || '',
        revision: canonical.updated_at,
      }
    }, [flush, workspace])

  return useMemo(
    () => ({
      items,
      activeId,
      draft,
      tagDraft,
      loading,
      error,
      autosaveStatus: autosave.status,
      autosaveError: autosave.error,
      setDraft,
      setTagDraft,
      selectItem,
      createItem,
      prepareSnapshot,
      flush,
      reload,
    }),
    [
      activeId,
      autosave.error,
      autosave.status,
      createItem,
      draft,
      error,
      flush,
      items,
      loading,
      prepareSnapshot,
      reload,
      selectItem,
      tagDraft,
    ],
  )
}

function readSelectedLoreID(workspace: string): string {
  if (!workspace || typeof window === 'undefined') return ''
  return window.localStorage.getItem(SELECTION_STORAGE_PREFIX + workspace) || ''
}

function persistSelectedLoreID(workspace: string, id: string) {
  if (!workspace || typeof window === 'undefined') return
  if (id) window.localStorage.setItem(SELECTION_STORAGE_PREFIX + workspace, id)
  else window.localStorage.removeItem(SELECTION_STORAGE_PREFIX + workspace)
}
