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

async function openProjectEventStream(options: { projectId: string; signal: AbortSignal; authorization?: string }) {
  const headers = new Headers({ Accept: 'text/event-stream' })
  if (options.authorization) headers.set('Authorization', options.authorization)
  const response = await fetch(`/api/projects/${encodeURIComponent(options.projectId)}/events`, {
    headers,
    signal: options.signal,
  })
  if (!response.ok) {
    const challenge = response.headers.get('WWW-Authenticate')?.toLowerCase() ?? ''
    throw new ProjectEventStreamHTTPError(response.status, challenge.includes('basic'))
  }
  if (!response.body) throw new Error('Project event stream has no response body')
  return parseSSEStream(response.body)
}
