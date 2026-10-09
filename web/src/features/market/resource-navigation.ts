import { create } from 'zustand'
import { useWorkspaceStore } from '@/stores/workspace-store'

/** Resource destinations are transient UI selection, never a second installation registry. */
export type ResourceDestination =
  | { section: 'discover' }
  | { section: 'acquired'; installationID?: string }
  | { section: 'extensions'; extensionKey?: string }
  | { section: 'updates' }

export const useResourceNavigation = create<{ destination: ResourceDestination }>(() => ({
  destination: { section: typeof window !== 'undefined' && window.localStorage.getItem('nova:mode') === 'extensions' ? 'extensions' : 'discover' },
}))

export function openResourceDestination(destination: ResourceDestination) {
  useResourceNavigation.setState({ destination })
  useWorkspaceStore.getState().setMode('market')
}
