import { useEffect, useRef } from 'react'

import type { WorkspaceChangeEvent } from '@/features/changes/types'
import { subscribeProjectFileEvents } from '@/features/workspace-events/client'

/** Connects a mounted surface to the origin-wide Project event SharedWorker. */
export function useProjectFileEvents(
  projectId: string,
  onChange: (event: WorkspaceChangeEvent) => void | Promise<void>,
) {
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  useEffect(() => {
    if (!projectId) return
    return subscribeProjectFileEvents(projectId, async event => {
      window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: event }))
      await onChangeRef.current(event)
    })
  }, [projectId])
}
