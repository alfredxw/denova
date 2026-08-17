import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { WorkspaceChangeEvent } from '@/features/changes/types'

import { useProjectFileEvents } from './useProjectFileEvents'

const workspaceEventMocks = vi.hoisted(() => ({
  subscribe: vi.fn(),
  callback: undefined as ((event: WorkspaceChangeEvent) => void | Promise<void>) | undefined,
  dispose: vi.fn(),
}))

vi.mock('@/features/workspace-events/client', () => ({
  subscribeProjectFileEvents: workspaceEventMocks.subscribe,
}))

describe('useProjectFileEvents', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    workspaceEventMocks.callback = undefined
    workspaceEventMocks.subscribe.mockImplementation((_projectId, callback) => {
      workspaceEventMocks.callback = callback
      return workspaceEventMocks.dispose
    })
  })

  it('shares one Project subscription and one global event across mounted consumers', async () => {
    const firstConsumer = vi.fn()
    const secondConsumer = vi.fn()
    const globalConsumer = vi.fn()
    window.addEventListener('nova:workspace-change', globalConsumer)
    const view = render(
      <>
        <Harness projectId="project-1" onChange={firstConsumer} />
        <Harness projectId="project-1" onChange={secondConsumer} />
      </>,
    )

    try {
      expect(workspaceEventMocks.subscribe).toHaveBeenCalledTimes(1)
      const event: WorkspaceChangeEvent = {
        project_id: 'project-1',
        source: 'watcher',
        paths: ['chapters/ch01.md'],
      }
      await act(async () => {
        await workspaceEventMocks.callback?.(event)
      })

      expect(globalConsumer).toHaveBeenCalledTimes(1)
      expect(firstConsumer).toHaveBeenCalledWith(event)
      expect(secondConsumer).toHaveBeenCalledWith(event)

      view.rerender(<Harness projectId="project-1" onChange={firstConsumer} />)
      expect(workspaceEventMocks.dispose).not.toHaveBeenCalled()
    } finally {
      view.unmount()
      window.removeEventListener('nova:workspace-change', globalConsumer)
    }
    expect(workspaceEventMocks.dispose).toHaveBeenCalledTimes(1)
  })
})

function Harness({ projectId, onChange }: { projectId: string; onChange: (event: WorkspaceChangeEvent) => void }) {
  useProjectFileEvents(projectId, onChange)
  return null
}
