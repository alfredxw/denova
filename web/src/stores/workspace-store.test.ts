import { beforeEach, describe, expect, it } from 'vitest'
import { useWorkspaceStore } from './workspace-store'

describe('useWorkspaceStore', () => {
  beforeEach(() => {
    window.localStorage.clear()
    useWorkspaceStore.setState({
      mode: 'ide',
      selectedProjectId: undefined,
      selectedChapterId: undefined,
      rightPanel: 'ai',
      bottomPanel: null,
      commandOpen: false,
    })
  })

  it('updates selectedChapterId', () => {
    useWorkspaceStore.getState().setSelectedChapterId('chapters/ch01.md')

    expect(useWorkspaceStore.getState().selectedChapterId).toBe('chapters/ch01.md')
  })

  it('keeps the bottom panel closed by default', () => {
    expect(useWorkspaceStore.getInitialState().bottomPanel).toBeNull()
  })

  it('persists the visible top-level mode and writing-side panel', () => {
    useWorkspaceStore.getState().setMode('interactive')
    useWorkspaceStore.getState().setMode('agents')
    useWorkspaceStore.getState().setRightPanel('versions')

    expect(window.localStorage.getItem('nova:mode')).toBe('agents')
    expect(window.localStorage.getItem('nova:content-mode')).toBe('interactive')
    expect(window.localStorage.getItem('nova:right-panel')).toBe('versions')

    useWorkspaceStore.getState().setRightPanel(null)
    expect(window.localStorage.getItem('nova:right-panel')).toBeNull()
  })
})
