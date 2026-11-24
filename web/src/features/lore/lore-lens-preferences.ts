import { useCallback, useContext, useEffect, useState } from 'react'
import { QueryClientContext } from '@tanstack/react-query'
import { workspaceChangePaths, type WorkspaceChangeEvent } from '@/features/changes/types'
import { getProjectLoreItems, type LoreCategory, type LoreItem } from '@/lib/api'
import { getLoreCategories } from '@/lib/api-client/lore'
import { isLoreItemsPath } from '@/lib/workspace-path'
import { LORE_UPDATED_EVENT, type LoreUpdatedDetail } from './events'
import { DEFAULT_LORE_CATEGORIES } from './knowledge-sections'
import { loreCategoriesKey } from './use-lore-categories'

export const EDITOR_SETTINGS_STORAGE_KEY = 'nova.editor.settings'
export const EDITOR_SETTINGS_UPDATED_EVENT = 'nova:editor-settings-updated'

export interface LoreLensPreferences {
  loreLensEnabled: boolean
  loreLensMatchKeywords: boolean
}

export const DEFAULT_LORE_LENS_PREFERENCES: LoreLensPreferences = {
  loreLensEnabled: true,
  loreLensMatchKeywords: false,
}

const EMPTY_LORE_ITEMS: LoreItem[] = []

export function readLoreLensPreferences(): LoreLensPreferences {
  try {
    const raw = window.localStorage.getItem(EDITOR_SETTINGS_STORAGE_KEY)
    if (!raw) return DEFAULT_LORE_LENS_PREFERENCES
    const parsed = JSON.parse(raw) as Partial<LoreLensPreferences>
    return {
      loreLensEnabled: typeof parsed.loreLensEnabled === 'boolean'
        ? parsed.loreLensEnabled
        : DEFAULT_LORE_LENS_PREFERENCES.loreLensEnabled,
      loreLensMatchKeywords: typeof parsed.loreLensMatchKeywords === 'boolean'
        ? parsed.loreLensMatchKeywords
        : DEFAULT_LORE_LENS_PREFERENCES.loreLensMatchKeywords,
    }
  } catch {
    return DEFAULT_LORE_LENS_PREFERENCES
  }
}

export function notifyEditorSettingsUpdated() {
  window.dispatchEvent(new CustomEvent(EDITOR_SETTINGS_UPDATED_EVENT))
}

export function updateLoreLensPreferences(patch: Partial<LoreLensPreferences>) {
  try {
    const raw = window.localStorage.getItem(EDITOR_SETTINGS_STORAGE_KEY)
    const current = raw ? (JSON.parse(raw) as Record<string, unknown>) : {}
    const next = { ...current, ...patch }
    window.localStorage.setItem(EDITOR_SETTINGS_STORAGE_KEY, JSON.stringify(next))
  } catch (error) {
    console.warn('[lore-lens-preferences] failed to persist lore lens preference', error)
  }
  notifyEditorSettingsUpdated()
}

/**
 * Shared local preference hook keeping Writing editor tabs and Game stage in sync
 * without adding server-side configuration fields or storage migrations.
 */
export function useLoreLensPreferences() {
  const [preferences, setPreferences] = useState<LoreLensPreferences>(() => readLoreLensPreferences())

  useEffect(() => {
    const sync = () => setPreferences(readLoreLensPreferences())
    const onStorage = (event: StorageEvent) => {
      if (event.key === EDITOR_SETTINGS_STORAGE_KEY) sync()
    }
    window.addEventListener(EDITOR_SETTINGS_UPDATED_EVENT, sync)
    window.addEventListener('storage', onStorage)
    return () => {
      window.removeEventListener(EDITOR_SETTINGS_UPDATED_EVENT, sync)
      window.removeEventListener('storage', onStorage)
    }
  }, [])

  const update = useCallback((patch: Partial<LoreLensPreferences>) => {
    setPreferences((current) => ({ ...current, ...patch }))
    updateLoreLensPreferences(patch)
  }, [])

  return { preferences, update }
}

/**
 * Reads cached Project Lore items using the app's canonical `['lore-items', projectId]`
 * query cache when a QueryClient is mounted, and gracefully degrades to an empty
 * list in isolated component tests without a QueryClientProvider.
 */
export function useProjectLoreItems(projectId: string, provided?: readonly LoreItem[]): readonly LoreItem[] {
  const queryClient = useContext(QueryClientContext)
  const [items, setItems] = useState<LoreItem[]>(() => {
    if (!queryClient || !projectId) return EMPTY_LORE_ITEMS
    return queryClient.getQueryData<LoreItem[]>(['lore-items', projectId]) ?? EMPTY_LORE_ITEMS
  })

  useEffect(() => {
    if (provided !== undefined || !queryClient || !projectId) {
      setItems(EMPTY_LORE_ITEMS)
      return
    }
    const queryKey = ['lore-items', projectId] as const
    let cancelled = false

    const syncFromCache = () => {
      if (cancelled) return
      setItems(queryClient.getQueryData<LoreItem[]>(queryKey) ?? EMPTY_LORE_ITEMS)
    }

    syncFromCache()
    void queryClient.ensureQueryData({
      queryKey,
      queryFn: () => getProjectLoreItems(projectId),
      staleTime: 30_000,
    }).then((data) => {
      if (!cancelled) setItems(data)
    }).catch((error) => {
      console.warn('[lore-lens] failed to load project lore items', { projectId, error })
    })

    const unsubscribe = queryClient.getQueryCache().subscribe((event) => {
      const key = event?.query?.queryKey
      if (Array.isArray(key) && key[0] === 'lore-items' && key[1] === projectId) {
        syncFromCache()
      }
    })

    const refetch = () => {
      void queryClient.invalidateQueries({ queryKey }, { cancelRefetch: false })
    }
    const onLoreUpdated = (event: Event) => {
      const detail = (event as CustomEvent<LoreUpdatedDetail>).detail
      if (detail?.projectId === projectId) refetch()
    }
    const onWorkspaceChange = (event: Event) => {
      const detail = (event as CustomEvent<WorkspaceChangeEvent>).detail
      if (detail?.project_id === projectId && (detail.resync || workspaceChangePaths(detail).some(isLoreItemsPath))) {
        refetch()
      }
    }
    window.addEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
    window.addEventListener('nova:workspace-change', onWorkspaceChange)
    return () => {
      cancelled = true
      unsubscribe()
      window.removeEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
      window.removeEventListener('nova:workspace-change', onWorkspaceChange)
    }
  }, [projectId, provided, queryClient])

  return provided ?? items
}

/** Reads project Lore categories safely even when mounted outside a QueryClientProvider. */
export function useOptionalLoreCategories(projectId: string): LoreCategory[] {
  const queryClient = useContext(QueryClientContext)
  const [categories, setCategories] = useState<LoreCategory[]>(() => {
    if (!queryClient || !projectId) return DEFAULT_LORE_CATEGORIES
    return queryClient.getQueryData<LoreCategory[]>(loreCategoriesKey(projectId)) ?? DEFAULT_LORE_CATEGORIES
  })

  useEffect(() => {
    if (!queryClient || !projectId) {
      setCategories(DEFAULT_LORE_CATEGORIES)
      return
    }
    const queryKey = loreCategoriesKey(projectId)
    let cancelled = false
    const syncFromCache = () => {
      if (cancelled) return
      setCategories(queryClient.getQueryData<LoreCategory[]>(queryKey) ?? DEFAULT_LORE_CATEGORIES)
    }
    syncFromCache()
    void queryClient.ensureQueryData({
      queryKey,
      queryFn: () => getLoreCategories(projectId),
      staleTime: 30_000,
    }).then((data) => {
      if (!cancelled) setCategories(data)
    }).catch((error) => {
      console.warn('[lore-lens] failed to load project lore categories', { projectId, error })
    })
    const unsubscribe = queryClient.getQueryCache().subscribe((event) => {
      const key = event?.query?.queryKey
      if (Array.isArray(key) && key[0] === 'lore-categories' && key[1] === projectId) {
        syncFromCache()
      }
    })
    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [projectId, queryClient])

  return categories
}
