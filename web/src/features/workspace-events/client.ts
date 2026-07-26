import type { WorkspaceChangeEvent } from '@/features/changes/types'
import { isWorkspaceChangeForWorkspace } from '@/features/changes/types'
import {
  getRemoteAccessAuthorization,
  handleRemoteAccessChallenge,
} from '@/lib/api-client/client'

import {
  isWorkspaceEventWorkerMessage,
  type WorkspaceEventClientMessage,
  type WorkspaceEventWorkerMessage,
} from './protocol'

const SHARED_WORKER_NAME = 'denova-workspace-events-v1'
const SETTINGS_UPDATED_EVENT = 'nova:settings-updated'

/** Subscribes one page to the origin-wide SharedWorker-owned event stream. */
export function subscribeWorkspaceFileEvents(
  workspace: string,
  onChange: (event: WorkspaceChangeEvent) => void | Promise<void>,
): () => void {
  if (typeof SharedWorker === 'undefined') {
    console.warn('[workspace-events/client.ts] SharedWorker is unavailable; foreground refresh remains active', {
      workspace,
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
      workspace,
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
        console.warn('[workspace-events/client.ts] workspace event consumer failed', { workspace, error })
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
    queuedEvent = { workspace, source: 'shared-worker', resync: true, changes: [] }
  }

  port.onmessage = (messageEvent: MessageEvent<WorkspaceEventWorkerMessage>) => {
    if (!isWorkspaceEventWorkerMessage(messageEvent.data)) {
      console.warn('[workspace-events/client.ts] ignored malformed SharedWorker message', { workspace })
      return
    }
    const message = messageEvent.data
    if (message.type === 'remote-access-required') {
      handleRemoteAccessChallenge()
      return
    }
    if (!isWorkspaceChangeForWorkspace(message.event, workspace)) return
    enqueue(message.event)
  }
  port.onmessageerror = () => {
    console.warn('[workspace-events/client.ts] SharedWorker message could not be decoded', { workspace })
  }
  port.start()

  const post = (message: WorkspaceEventClientMessage) => port.postMessage(message)
  post({ type: 'subscribe', workspace, authorization: getRemoteAccessAuthorization() })

  const updateAuthorization = () => {
    post({ type: 'authorization', authorization: getRemoteAccessAuthorization() })
  }
  window.addEventListener(SETTINGS_UPDATED_EVENT, updateAuthorization)

  const dispose = () => {
    if (disposed) return
    disposed = true
    window.removeEventListener(SETTINGS_UPDATED_EVENT, updateAuthorization)
    window.removeEventListener('pagehide', dispose)
    // The worker closes both ends after processing this message. Closing the
    // page-side port immediately can discard the queued unsubscribe.
    try {
      post({ type: 'unsubscribe' })
    } catch (error) {
      console.warn('[workspace-events/client.ts] failed to unsubscribe SharedWorker port', { workspace, error })
      port.close()
    }
    port.onmessage = null
    port.onmessageerror = null
  }
  window.addEventListener('pagehide', dispose, { once: true })
  return dispose
}
