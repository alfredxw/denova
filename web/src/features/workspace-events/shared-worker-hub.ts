import type { SSEEvent } from '@/lib/api-client/types'
import type { WorkspaceChangeEvent } from '@/features/changes/types'

import {
  isWorkspaceEventClientMessage,
  parseWorkspaceChangeSSE,
  type WorkspaceEventPort,
} from './protocol'

const INITIAL_RECONNECT_DELAY_MS = 250
const MAX_RECONNECT_DELAY_MS = 5_000

export interface WorkspaceEventStreamOptions {
  signal: AbortSignal
  authorization?: string
}

export type WorkspaceEventStreamFactory = (
  options: WorkspaceEventStreamOptions,
) => Promise<ReadableStream<SSEEvent>>

export class WorkspaceEventStreamHTTPError extends Error {
  readonly status: number
  readonly basicAuthChallenge: boolean

  constructor(status: number, basicAuthChallenge: boolean) {
    super(`Workspace event stream returned HTTP ${status}`)
    this.name = 'WorkspaceEventStreamHTTPError'
    this.status = status
    this.basicAuthChallenge = basicAuthChallenge
  }
}

type StreamPhase = 'idle' | 'connecting' | 'open' | 'auth-required'

/** Owns the single SSE connection shared by every same-origin browser tab. */
export class SharedWorkspaceEventHub {
  private readonly subscribers = new Map<WorkspaceEventPort, string>()
  private readonly openStream: WorkspaceEventStreamFactory
  private authorization: string | undefined
  private streamPhase: StreamPhase = 'idle'
  private streamGeneration = 0
  private streamTask: Promise<void> | null = null
  private abortController: AbortController | null = null
  private activeReader: ReadableStreamDefaultReader<SSEEvent> | null = null

  constructor(options: { openStream: WorkspaceEventStreamFactory }) {
    this.openStream = options.openStream
  }

  connect(port: WorkspaceEventPort) {
    port.onmessage = event => {
      if (!isWorkspaceEventClientMessage(event.data)) {
        console.warn('[workspace-events/shared-worker-hub.ts] ignored malformed client message')
        return
      }
      const message = event.data
      switch (message.type) {
        case 'subscribe':
          this.subscribe(port, message.workspace, message.authorization)
          return
        case 'authorization':
          this.updateAuthorization(message.authorization)
          return
        case 'unsubscribe':
          this.unsubscribe(port)
          return
      }
    }
    port.onmessageerror = () => {
      console.warn('[workspace-events/shared-worker-hub.ts] client message could not be decoded; removing subscriber')
      this.unsubscribe(port)
    }
    port.start()
  }

  private subscribe(port: WorkspaceEventPort, workspace: string, authorization?: string) {
    const streamWasOpen = this.streamPhase === 'open'
    const streamNeedsAuthorization = this.streamPhase === 'auth-required'
    const firstSubscriber = this.subscribers.size === 0
    this.subscribers.set(port, workspace)

    const shouldAdoptAuthorization = firstSubscriber || Boolean(authorization)
    const authorizationChanged = shouldAdoptAuthorization && authorization !== this.authorization
    if (shouldAdoptAuthorization) this.authorization = authorization

    if (authorizationChanged && (this.streamTask || streamNeedsAuthorization)) this.restartStream()
    else this.ensureStream()

    if (streamNeedsAuthorization && !authorizationChanged) {
      this.post(port, { type: 'remote-access-required' })
    }

    // An established stream already emitted its backend-provided initial
    // resync. Give a later tab the same canonical-refresh guarantee.
    if (streamWasOpen) {
      this.post(port, {
        type: 'workspace-change',
        event: { workspace, source: 'shared-worker', resync: true, changes: [] },
      })
    }
  }

  private updateAuthorization(authorization?: string) {
    if (authorization === this.authorization && this.streamPhase !== 'auth-required') return
    this.authorization = authorization
    if (this.subscribers.size > 0) this.restartStream()
  }

  private unsubscribe(port: WorkspaceEventPort) {
    if (!this.subscribers.delete(port)) return
    port.onmessage = null
    port.onmessageerror = null
    port.close()
    if (this.subscribers.size === 0) {
      this.authorization = undefined
      this.stopStream()
    }
  }

  private ensureStream() {
    if (this.subscribers.size === 0 || this.streamTask || this.streamPhase === 'auth-required') return
    const generation = ++this.streamGeneration
    const abortController = new AbortController()
    this.abortController = abortController
    const task = this.observe(generation, abortController.signal)
      .finally(() => {
        if (this.streamTask !== task) return
        this.streamTask = null
        this.abortController = null
        if (this.streamPhase !== 'auth-required') this.streamPhase = 'idle'
      })
    this.streamTask = task
  }

  private restartStream() {
    this.stopStream()
    this.ensureStream()
  }

  private stopStream() {
    this.streamGeneration += 1
    this.streamPhase = 'idle'
    this.abortController?.abort()
    this.abortController = null
    const reader = this.activeReader
    this.activeReader = null
    if (reader) void reader.cancel().catch(() => {})
    this.streamTask = null
  }

  private async observe(generation: number, signal: AbortSignal) {
    let reconnectDelay = INITIAL_RECONNECT_DELAY_MS
    while (this.isActive(generation, signal)) {
      let reader: ReadableStreamDefaultReader<SSEEvent> | null = null
      try {
        this.streamPhase = 'connecting'
        const stream = await this.openStream({ signal, authorization: this.authorization })
        if (!this.isActive(generation, signal)) {
          await stream.cancel()
          return
        }
        reconnectDelay = INITIAL_RECONNECT_DELAY_MS
        this.streamPhase = 'open'
        reader = stream.getReader()
        this.activeReader = reader
        while (this.isActive(generation, signal)) {
          const { done, value } = await reader.read()
          if (done) break
          const event = parseWorkspaceChangeSSE(value)
          if (event) this.broadcastWorkspaceChange(event)
        }
      } catch (error) {
        if (!this.isActive(generation, signal)) return
        if (error instanceof WorkspaceEventStreamHTTPError && error.status === 401 && error.basicAuthChallenge) {
          this.streamPhase = 'auth-required'
          this.broadcastRemoteAccessRequired()
          return
        }
        console.warn('[workspace-events/shared-worker-hub.ts] workspace event stream disconnected; retrying', {
          workspaces: Array.from(new Set(this.subscribers.values())),
          error,
        })
      } finally {
        if (reader) {
          if (this.activeReader === reader) this.activeReader = null
          try {
            reader.releaseLock()
          } catch {
            // cancel() can release the underlying reader during teardown.
          }
        }
      }

      if (!this.isActive(generation, signal)) return
      await waitForReconnect(reconnectDelay, signal)
      reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY_MS)
    }
  }

  private isActive(generation: number, signal: AbortSignal) {
    return generation === this.streamGeneration && !signal.aborted && this.subscribers.size > 0
  }

  private broadcastWorkspaceChange(event: WorkspaceChangeEvent) {
    for (const [port, workspace] of this.subscribers) {
      if (event.workspace !== workspace) continue
      this.post(port, { type: 'workspace-change', event })
    }
  }

  private broadcastRemoteAccessRequired() {
    for (const port of this.subscribers.keys()) {
      this.post(port, { type: 'remote-access-required' })
    }
  }

  private post(port: WorkspaceEventPort, message: Parameters<WorkspaceEventPort['postMessage']>[0]) {
    try {
      port.postMessage(message)
    } catch (error) {
      console.warn('[workspace-events/shared-worker-hub.ts] failed to notify a tab; removing subscriber', { error })
      this.unsubscribe(port)
    }
  }
}

function waitForReconnect(delay: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  return new Promise(resolve => {
    const finish = () => {
      globalThis.clearTimeout(timer)
      signal.removeEventListener('abort', finish)
      resolve()
    }
    const timer = globalThis.setTimeout(finish, delay)
    signal.addEventListener('abort', finish, { once: true })
  })
}
