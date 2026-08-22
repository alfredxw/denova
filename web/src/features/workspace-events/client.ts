import type { WorkspaceChangeEvent } from '@/features/changes/types'
import { isProjectChangeForProject } from '@/features/changes/types'
import {
  getRemoteAccessAuthorization,
  handleRemoteAccessChallenge,
} from '@/lib/api-client/client'

import {
  isWorkspaceEventWorkerMessage,
  type WorkspaceEventClientMessage,
  type WorkspaceEventWorkerMessage,
} from './protocol'
import { GLOBAL_SETTINGS_TARGET, subscribeSettingsTarget } from '@/features/settings/query'

const SHARED_WORKER_NAME = 'denova-project-events-v2'

/** Subscribes one page to the origin-wide SharedWorker-owned event stream. */
export function subscribeProjectFileEvents(
  projectId: string,
  onChange: (event: WorkspaceChangeEvent) => void | Promise<void>,
): () => void {
  if (typeof SharedWorker === 'undefined') {
    console.warn('[workspace-events/client.ts] SharedWorker is unavailable; foreground refresh remains active', {
      projectId,
    })
    return () => {}
  }

  let worker: SharedWorker
  try {
    worker = new SharedWorker(new URL('./shared-worker.ts', import.meta.url), {
      name: SHARED_WORKER_NAME,
      type: 'module',
      credentials: 'same-origin',
    })
  } catch (error) {
    console.warn('[workspace-events/client.ts] failed to start SharedWorker; foreground refresh remains active', {
      projectId,
      error,
    })
    return () => {}
  }

  const port = worker.port
  let disposed = false
  let consuming = false
  let queuedEvent: WorkspaceChangeEvent | null = null

  const consume = async (firstEvent: WorkspaceChangeEvent) => {
    consuming = true
    let nextEvent: WorkspaceChangeEvent | null = firstEvent
    while (nextEvent && !disposed) {
      try {
        await onChange(nextEvent)
      } catch (error) {
        console.warn('[workspace-events/client.ts] Project event consumer failed', { projectId, error })
      }
      nextEvent = queuedEvent
      queuedEvent = null
    }
    consuming = false
  }

  const enqueue = (event: WorkspaceChangeEvent) => {
    if (!consuming) {
      void consume(event)
      return
    }
    // File events are invalidation hints. One canonical resync safely replaces
    // any number of events received while this page is still refreshing.
    queuedEvent = { project_id: projectId, source: 'shared-worker', resync: true, changes: [] }
  }

  port.onmessage = (messageEvent: MessageEvent<WorkspaceEventWorkerMessage>) => {
    if (!isWorkspaceEventWorkerMessage(messageEvent.data)) {
      console.warn('[workspace-events/client.ts] ignored malformed SharedWorker message', { projectId })
      return
    }
    const message = messageEvent.data
    if (message.type === 'remote-access-required') {
      handleRemoteAccessChallenge()
      return
    }
    if (!isProjectChangeForProject(message.event, projectId)) return
    enqueue(message.event)
  }
  port.onmessageerror = () => {
    console.warn('[workspace-events/client.ts] SharedWorker message could not be decoded', { projectId })
  }
  port.start()

  const post = (message: WorkspaceEventClientMessage) => port.postMessage(message)
  post({ type: 'subscribe', projectId, authorization: getRemoteAccessAuthorization() })

  const updateAuthorization = () => {
    post({ type: 'authorization', authorization: getRemoteAccessAuthorization() })
  }
  const unsubscribeSettings = subscribeSettingsTarget(GLOBAL_SETTINGS_TARGET, updateAuthorization)

  const dispose = () => {
    if (disposed) return
    disposed = true
    unsubscribeSettings()
    window.removeEventListener('pagehide', dispose)
    // The worker closes both ends after processing this message. Closing the
    // page-side port immediately can discard the queued unsubscribe.
    try {
      post({ type: 'unsubscribe' })
    } catch (error) {
      console.warn('[workspace-events/client.ts] failed to unsubscribe SharedWorker port', { projectId, error })
      port.close()
    }
    port.onmessage = null
    port.onmessageerror = null
  }
  window.addEventListener('pagehide', dispose, { once: true })
  return dispose
}
