import { afterEach, describe, expect, it, vi } from 'vitest'

import type { WorkspaceChangeEvent } from '@/features/changes/types'

import { subscribeWorkspaceFileEvents } from './client'
import type { WorkspaceEventClientMessage, WorkspaceEventWorkerMessage } from './protocol'

describe('subscribeWorkspaceFileEvents', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    window.sessionStorage.clear()
  })

  it('uses one named SharedWorker port and sends an explicit unsubscribe', () => {
    const port = new FakePagePort()
    const constructorCalls: Array<{ url: string; options?: WorkerOptions }> = []
    installSharedWorker(port, constructorCalls)

    const dispose = subscribeWorkspaceFileEvents('/books/demo', vi.fn())

    expect(constructorCalls).toEqual([{
      url: expect.stringContaining('/workspace-events/shared-worker.ts'),
      options: expect.objectContaining({ name: 'denova-workspace-events-v1', type: 'module' }),
    }])
    expect(port.sent).toEqual([{
      type: 'subscribe',
      workspace: '/books/demo',
      authorization: undefined,
    }])

    dispose()
    expect(port.sent.at(-1)).toEqual({ type: 'unsubscribe' })
  })

  it('bounds events received during a slow refresh by replacing them with one resync', async () => {
    const port = new FakePagePort()
    installSharedWorker(port, [])
    const firstRefresh = deferred<void>()
    const onChange = vi.fn()
      .mockReturnValueOnce(firstRefresh.promise)
      .mockResolvedValue(undefined)
    const dispose = subscribeWorkspaceFileEvents('/books/demo', onChange)

    port.emit(workspaceChange('chapters/ch01.md'))
    await eventually(() => expect(onChange).toHaveBeenCalledTimes(1))
    port.emit(workspaceChange('chapters/ch02.md'))
    port.emit(workspaceChange('chapters/ch03.md'))
    expect(onChange).toHaveBeenCalledTimes(1)

    firstRefresh.resolve()
    await eventually(() => expect(onChange).toHaveBeenCalledTimes(2))
    expect(onChange.mock.calls[1]?.[0]).toEqual({
      workspace: '/books/demo',
      source: 'shared-worker',
      resync: true,
      changes: [],
    })

    dispose()
  })
})

class FakePagePort {
  onmessage: ((event: MessageEvent<WorkspaceEventWorkerMessage>) => void) | null = null
  onmessageerror: (() => void) | null = null
  sent: WorkspaceEventClientMessage[] = []

  postMessage(message: WorkspaceEventClientMessage) {
    this.sent.push(message)
  }

  start() {}

  close() {}

  emit(message: WorkspaceEventWorkerMessage) {
    this.onmessage?.(new MessageEvent('message', { data: message }))
  }
}

function installSharedWorker(
  port: FakePagePort,
  calls: Array<{ url: string; options?: WorkerOptions }>,
) {
  class SharedWorkerMock {
    readonly port = port

    constructor(url: URL, options?: WorkerOptions) {
      calls.push({ url: url.toString(), options })
    }
  }
  vi.stubGlobal('SharedWorker', SharedWorkerMock)
}

function workspaceChange(path: string): WorkspaceEventWorkerMessage {
  const event: WorkspaceChangeEvent = {
    workspace: '/books/demo',
    source: 'watcher',
    changes: [{ path, type: 'updated' }],
    paths: [path],
  }
  return { type: 'workspace-change', event }
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>(nextResolve => {
    resolve = nextResolve
  })
  return { promise, resolve }
}

async function eventually(assertion: () => void) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      assertion()
      return
    } catch (error) {
      if (attempt === 19) throw error
      await Promise.resolve()
    }
  }
}
