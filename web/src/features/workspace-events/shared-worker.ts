import { parseSSEStream } from '@/lib/api-client/sse'

import type { WorkspaceEventPort } from './protocol'
import { ProjectEventStreamHTTPError, SharedProjectEventHub } from './shared-worker-hub'

interface SharedWorkerRuntimeScope {
  onconnect: ((event: MessageEvent) => void) | null
}

const hub = new SharedProjectEventHub({ openStream: openProjectEventStream })
const scope = globalThis as unknown as SharedWorkerRuntimeScope

scope.onconnect = event => {
  const port = event.ports[0]
  if (!port) {
    console.warn('[workspace-events/shared-worker.ts] SharedWorker connection arrived without a MessagePort')
    return
  }
  hub.connect(port as unknown as WorkspaceEventPort)
}

async function openProjectEventStream(options: { projectId: string; signal: AbortSignal }) {
  const headers = new Headers({ Accept: 'text/event-stream' })
  const response = await fetch(`/api/projects/${encodeURIComponent(options.projectId)}/events`, {
    headers,
    credentials: 'same-origin',
    signal: options.signal,
  })
  if (!response.ok) {
    throw new ProjectEventStreamHTTPError(response.status, response.headers.get('X-Denova-Auth') === 'required')
  }
  if (!response.body) throw new Error('Project event stream has no response body')
  return parseSSEStream(response.body)
}
