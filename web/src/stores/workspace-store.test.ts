import { beforeEach, describe, expect, it } from 'vitest'
import { useWorkspaceStore } from './workspace-store'

describe('useWorkspaceStore', () => {
  beforeEach(() => {
    useWorkspaceStore.setState({
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
})
