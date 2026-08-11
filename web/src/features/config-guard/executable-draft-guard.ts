import { useEffect } from 'react'
import { create } from 'zustand'

export interface ExecutableDraftEntry {
  hasPending: boolean
  discard: () => void | Promise<void>
}

interface ExecutableDraftGuardState {
  entries: Record<string, ExecutableDraftEntry>
}

/** 可执行配置草稿守卫注册表：各配置面注册“是否有待保存草稿”与“如何放弃”。 */
export const useExecutableDraftGuard = create<ExecutableDraftGuardState>(() => ({
  entries: {},
}))

export function registerExecutableDraft(key: string, entry: ExecutableDraftEntry) {
  useExecutableDraftGuard.setState((state) => ({
    entries: { ...state.entries, [key]: entry },
  }))
}

export function unregisterExecutableDraft(key: string) {
  useExecutableDraftGuard.setState((state) => {
    const entries = { ...state.entries }
    delete entries[key]
    return { entries }
  })
}

export function hasPendingExecutableDraft(key: string): boolean {
  return Boolean(useExecutableDraftGuard.getState().entries[key]?.hasPending)
}

export function discardExecutableDraft(key: string): void | Promise<void> | undefined {
  return useExecutableDraftGuard.getState().entries[key]?.discard?.()
}

export function useExecutableDraftEntry(
  key: string,
  hasPending: boolean,
  discard: () => void | Promise<void>,
) {
  useEffect(() => {
    registerExecutableDraft(key, { hasPending, discard })
    return () => unregisterExecutableDraft(key)
  }, [discard, hasPending, key])
}
