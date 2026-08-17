import type { SSEEvent } from '@/lib/api-client/types'
import type { WorkspaceChangeEvent } from '@/features/changes/types'

import {
  isWorkspaceEventClientMessage,
  parseWorkspaceChangeSSE,
  type WorkspaceEventPort,
} from './protocol'

const INITIAL_RECONNECT_DELAY_MS = 250
const MAX_RECONNECT_DELAY_MS = 5_000

export interface ProjectEventStreamOptions {
  projectId: string
  signal: AbortSignal
  authorization?: string
}

export type ProjectEventStreamFactory = (
  options: ProjectEventStreamOptions,
) => Promise<ReadableStream<SSEEvent>>

export class ProjectEventStreamHTTPError extends Error {
  readonly status: number
  readonly basicAuthChallenge: boolean

  constructor(status: number, basicAuthChallenge: boolean) {
    super(`Project event stream returned HTTP ${status}`)
    this.name = 'ProjectEventStreamHTTPError'
    this.status = status
    this.basicAuthChallenge = basicAuthChallenge
  }
}

type StreamPhase = 'idle' | 'connecting' | 'open' | 'auth-required'

interface ProjectStreamState {
  phase: StreamPhase
  generation: number
  task: Promise<void> | null
  abortController: AbortController | null
  activeReader: ReadableStreamDefaultReader<SSEEvent> | null
}

/** Owns one shared SSE connection per subscribed Project across every
 * same-origin browser tab. Different Projects remain independently observable. */
export class SharedProjectEventHub {
  private readonly subscribers = new Map<WorkspaceEventPort, string>()
  private readonly streams = new Map<string, ProjectStreamState>()
  private readonly openStream: ProjectEventStreamFactory
  private authorization: string | undefined

  constructor(options: { openStream: ProjectEventStreamFactory }) {
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
          this.subscribe(port, message.projectId, message.authorization)
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

  private subscribe(port: WorkspaceEventPort, projectId: string, authorization?: string) {
    const previousProjectId = this.subscribers.get(port)
    if (previousProjectId && previousProjectId !== projectId) {
      this.subscribers.delete(port)
      this.stopIfUnused(previousProjectId)
    }
    const firstSubscriber = this.subscribers.size === 0
    this.subscribers.set(port, projectId)

    const state = this.projectState(projectId)
    const streamWasOpen = state.phase === 'open'
    const streamNeedsAuthorization = state.phase === 'auth-required'
    const shouldAdoptAuthorization = firstSubscriber || Boolean(authorization)
    const authorizationChanged = shouldAdoptAuthorization && authorization !== this.authorization
    if (shouldAdoptAuthorization) this.authorization = authorization

    if (authorizationChanged) this.restartAllStreams()
    else this.ensureStream(projectId)

    if (streamNeedsAuthorization && !authorizationChanged) {
      this.post(port, { type: 'remote-access-required' })
    }

    if (streamWasOpen) {
      this.post(port, {
        type: 'workspace-change',
        event: { project_id: projectId, source: 'shared-worker', resync: true, changes: [] },
      })
    }
  }

  private updateAuthorization(authorization?: string) {
    if (authorization === this.authorization && !this.hasAuthRequiredStream()) return
    this.authorization = authorization
    this.restartAllStreams()
  }

  private unsubscribe(port: WorkspaceEventPort) {
    const projectId = this.subscribers.get(port)
    if (!this.subscribers.delete(port)) return
    port.onmessage = null
    port.onmessageerror = null
    port.close()
    if (projectId) this.stopIfUnused(projectId)
    if (this.subscribers.size === 0) this.authorization = undefined
  }

  private projectState(projectId: string): ProjectStreamState {
    const existing = this.streams.get(projectId)
    if (existing) return existing
    const state: ProjectStreamState = {
      phase: 'idle',
      generation: 0,
      task: null,
      abortController: null,
      activeReader: null,
    }
    this.streams.set(projectId, state)
    return state
  }

  private ensureStream(projectId: string) {
    if (!this.hasSubscribers(projectId)) return
    const state = this.projectState(projectId)
    if (state.task || state.phase === 'auth-required') return
    const generation = ++state.generation
    const abortController = new AbortController()
    state.abortController = abortController
    const task = this.observe(projectId, state, generation, abortController.signal)
      .finally(() => {
        if (state.task !== task) return
        state.task = null
        state.abortController = null
        if (state.phase !== 'auth-required') state.phase = 'idle'
      })
    state.task = task
  }

  private restartAllStreams() {
    const projectIds = Array.from(new Set(this.subscribers.values()))
    for (const projectId of projectIds) this.stopStream(projectId)
    for (const projectId of projectIds) this.ensureStream(projectId)
  }

  private stopIfUnused(projectId: string) {
    if (this.hasSubscribers(projectId)) return
    this.stopStream(projectId)
    this.streams.delete(projectId)
  }

  private stopStream(projectId: string) {
    const state = this.streams.get(projectId)
    if (!state) return
    state.generation += 1
    state.phase = 'idle'
    state.abortController?.abort()
    state.abortController = null
    const reader = state.activeReader
    state.activeReader = null
    if (reader) void reader.cancel().catch(() => {})
    state.task = null
  }

  private async observe(projectId: string, state: ProjectStreamState, generation: number, signal: AbortSignal) {
    let reconnectDelay = INITIAL_RECONNECT_DELAY_MS
    while (this.isActive(projectId, state, generation, signal)) {
      let reader: ReadableStreamDefaultReader<SSEEvent> | null = null
      try {
        state.phase = 'connecting'
        const stream = await this.openStream({ projectId, signal, authorization: this.authorization })
        if (!this.isActive(projectId, state, generation, signal)) {
          await stream.cancel()
          return
        }
        reconnectDelay = INITIAL_RECONNECT_DELAY_MS
        state.phase = 'open'
        reader = stream.getReader()
        state.activeReader = reader
        while (this.isActive(projectId, state, generation, signal)) {
          const { done, value } = await reader.read()
          if (done) break
          const event = parseWorkspaceChangeSSE(value)
          if (event?.project_id === projectId) this.broadcastProjectChange(projectId, event)
        }
      } catch (error) {
        if (!this.isActive(projectId, state, generation, signal)) return
        if (error instanceof ProjectEventStreamHTTPError && error.status === 401 && error.basicAuthChallenge) {
          state.phase = 'auth-required'
          this.broadcastRemoteAccessRequired(projectId)
          return
        }
        console.warn('[workspace-events/shared-worker-hub.ts] Project event stream disconnected; retrying', {
          projectId,
          error,
        })
      } finally {
        if (reader) {
          if (state.activeReader === reader) state.activeReader = null
          try {
            reader.releaseLock()
          } catch {
            // cancel() can release the underlying reader during teardown.
          }
        }
      }

      if (!this.isActive(projectId, state, generation, signal)) return
      await waitForReconnect(reconnectDelay, signal)
      reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY_MS)
    }
  }

  private isActive(projectId: string, state: ProjectStreamState, generation: number, signal: AbortSignal) {
    return this.streams.get(projectId) === state && generation === state.generation && !signal.aborted && this.hasSubscribers(projectId)
  }

  private hasSubscribers(projectId: string) {
    for (const subscribedProjectId of this.subscribers.values()) {
      if (subscribedProjectId === projectId) return true
    }
    return false
  }

  private hasAuthRequiredStream() {
    for (const state of this.streams.values()) {
      if (state.phase === 'auth-required') return true
    }
    return false
  }

  private broadcastProjectChange(projectId: string, event: WorkspaceChangeEvent) {
    for (const [port, subscribedProjectId] of this.subscribers) {
      if (subscribedProjectId !== projectId) continue
      this.post(port, { type: 'workspace-change', event })
    }
  }

  private broadcastRemoteAccessRequired(projectId: string) {
    for (const [port, subscribedProjectId] of this.subscribers) {
      if (subscribedProjectId === projectId) this.post(port, { type: 'remote-access-required' })
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
