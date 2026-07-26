import { useEffect, useRef } from 'react'

import type { WorkspaceChangeEvent } from '@/features/changes/types'
import { subscribeWorkspaceFileEvents } from '@/features/workspace-events/client'

/** Connects the app to the origin-wide workspace event SharedWorker. */
export function useWorkspaceFileEvents(
  workspace: string,
  onChange: (event: WorkspaceChangeEvent) => void | Promise<void>,
) {
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  useEffect(() => {
    if (!workspace) return
    return subscribeWorkspaceFileEvents(workspace, async event => {
      window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: event }))
      await onChangeRef.current(event)
    })
  }, [workspace])
}
