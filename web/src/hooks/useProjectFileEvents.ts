import { useEffect, useRef } from 'react'

import type { WorkspaceChangeEvent } from '@/features/changes/types'
import { subscribeProjectFileEvents } from '@/features/workspace-events/client'

type ProjectFileEventConsumer = (event: WorkspaceChangeEvent) => void | Promise<void>

interface ProjectFileEventSubscription {
  consumers: Set<ProjectFileEventConsumer>
  dispose: () => void
}

const projectFileEventSubscriptions = new Map<string, ProjectFileEventSubscription>()

function subscribeProjectFileEventConsumer(projectId: string, consumer: ProjectFileEventConsumer) {
  let subscription = projectFileEventSubscriptions.get(projectId)
  if (!subscription) {
    const consumers = new Set<ProjectFileEventConsumer>()
    subscription = { consumers, dispose: () => {} }
    projectFileEventSubscriptions.set(projectId, subscription)
    try {
      subscription.dispose = subscribeProjectFileEvents(projectId, async event => {
        window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: event }))
        const results = await Promise.allSettled([...consumers].map(handler => handler(event)))
        results.forEach((result) => {
          if (result.status === 'rejected') {
            console.error('[useProjectFileEvents.ts] Project event consumer failed', { projectId, error: result.reason })
          }
        })
      })
    } catch (error) {
      projectFileEventSubscriptions.delete(projectId)
      throw error
    }
  }
  subscription.consumers.add(consumer)

  return () => {
    const current = projectFileEventSubscriptions.get(projectId)
    if (!current) return
    current.consumers.delete(consumer)
    if (current.consumers.size > 0) return
    current.dispose()
    projectFileEventSubscriptions.delete(projectId)
  }
}

/** Connects a mounted surface to the origin-wide Project event SharedWorker. */
export function useProjectFileEvents(
  projectId: string,
  onChange: (event: WorkspaceChangeEvent) => void | Promise<void>,
) {
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  useEffect(() => {
    if (!projectId) return
    return subscribeProjectFileEventConsumer(projectId, event => onChangeRef.current(event))
  }, [projectId])
}
