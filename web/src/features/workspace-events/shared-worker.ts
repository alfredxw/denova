import { parseSSEStream } from '@/lib/api-client/sse'

import type { WorkspaceEventPort } from './protocol'
import { SharedWorkspaceEventHub, WorkspaceEventStreamHTTPError } from './shared-worker-hub'

interface SharedWorkerRuntimeScope {
  onconnect: ((event: MessageEvent) => void) | null
}

const hub = new SharedWorkspaceEventHub({ openStream: openWorkspaceEventStream })
const scope = globalThis as unknown as SharedWorkerRuntimeScope

scope.onconnect = event => {
  const port = event.ports[0]
  if (!port) {
    console.warn('[workspace-events/shared-worker.ts] SharedWorker connection arrived without a MessagePort')
    return
  }
  hub.connect(port as unknown as WorkspaceEventPort)
}

async function openWorkspaceEventStream(options: { signal: AbortSignal; authorization?: string }) {
  const headers = new Headers({ Accept: 'text/event-stream' })
  if (options.authorization) headers.set('Authorization', options.authorization)
  const response = await fetch('/api/workspace/events', {
    headers,
    signal: options.signal,
  })
  if (!response.ok) {
    const challenge = response.headers.get('WWW-Authenticate')?.toLowerCase() ?? ''
    throw new WorkspaceEventStreamHTTPError(response.status, challenge.includes('basic'))
  }
  if (!response.body) throw new Error('Workspace event stream has no response body')
  return parseSSEStream(response.body)
}
