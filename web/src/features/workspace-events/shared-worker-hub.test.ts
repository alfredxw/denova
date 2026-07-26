import { describe, expect, it, vi } from 'vitest'

import type { SSEEvent } from '@/lib/api-client/types'
import {
  SharedWorkspaceEventHub,
  WorkspaceEventStreamHTTPError,
  type WorkspaceEventStreamFactory,
} from './shared-worker-hub'
import type {
  WorkspaceEventClientMessage,
  WorkspaceEventPort,
  WorkspaceEventWorkerMessage,
} from './protocol'

describe('SharedWorkspaceEventHub', () => {
  it('shares one stream across ports and closes it only after the last subscriber leaves', async () => {
    const events = controlledStream<SSEEvent>()
    const openStream = vi.fn<WorkspaceEventStreamFactory>(async () => events.stream)
    const hub = new SharedWorkspaceEventHub({ openStream })
    const first = new FakePort()
    const second = new FakePort()

    hub.connect(first)
    first.receive({ type: 'subscribe', workspace: '/books/demo' })
    await eventually(() => expect(openStream).toHaveBeenCalledTimes(1))

    hub.connect(second)
    second.receive({ type: 'subscribe', workspace: '/books/demo' })
    await flushMicrotasks()
    expect(openStream).toHaveBeenCalledTimes(1)

    events.enqueue(workspaceChangeSSE('/books/demo', 'chapters/ch01.md'))
    await eventually(() => {
      expect(workspaceChanges(first)).toContainEqual(expect.objectContaining({
        changes: [{ path: 'chapters/ch01.md', type: 'updated' }],
      }))
      expect(workspaceChanges(second)).toContainEqual(expect.objectContaining({
        changes: [{ path: 'chapters/ch01.md', type: 'updated' }],
      }))
    })

    first.receive({ type: 'unsubscribe' })
    expect(events.cancel).not.toHaveBeenCalled()
    second.receive({ type: 'unsubscribe' })
    await eventually(() => expect(events.cancel).toHaveBeenCalledTimes(1))
  })

  it('filters events by workspace and resyncs a subscriber joining an established stream', async () => {
    const events = controlledStream<SSEEvent>()
    const openStream = vi.fn<WorkspaceEventStreamFactory>(async () => events.stream)
    const hub = new SharedWorkspaceEventHub({ openStream })
    const first = new FakePort()
    const second = new FakePort()

    hub.connect(first)
    first.receive({ type: 'subscribe', workspace: '/books/demo' })
    events.enqueue(workspaceChangeSSE('/books/demo', 'chapters/ch01.md'))
    await eventually(() => expect(workspaceChanges(first)).toHaveLength(1))

    hub.connect(second)
    second.receive({ type: 'subscribe', workspace: '/books/other' })
    await eventually(() => expect(workspaceChanges(second)).toEqual([{
      workspace: '/books/other',
      source: 'shared-worker',
      resync: true,
      changes: [],
    }]))

    events.enqueue(workspaceChangeSSE('/books/demo', 'chapters/ch02.md'))
    await eventually(() => expect(workspaceChanges(first)).toHaveLength(2))
    expect(workspaceChanges(second)).toHaveLength(1)

    first.receive({ type: 'unsubscribe' })
    second.receive({ type: 'unsubscribe' })
  })

  it('pauses on a Basic auth challenge and reconnects with updated credentials', async () => {
    const events = controlledStream<SSEEvent>()
    const openStream = vi.fn<WorkspaceEventStreamFactory>()
      .mockRejectedValueOnce(new WorkspaceEventStreamHTTPError(401, true))
      .mockResolvedValueOnce(events.stream)
    const hub = new SharedWorkspaceEventHub({ openStream })
    const port = new FakePort()

    hub.connect(port)
    port.receive({ type: 'subscribe', workspace: '/books/demo' })
    await eventually(() => expect(port.sent).toContainEqual({ type: 'remote-access-required' }))
    expect(openStream).toHaveBeenCalledTimes(1)

    port.receive({ type: 'authorization', authorization: 'Basic valid' })
    await eventually(() => expect(openStream).toHaveBeenCalledTimes(2))
    expect(openStream.mock.calls[1]?.[0].authorization).toBe('Basic valid')

    port.receive({ type: 'unsubscribe' })
  })
})

class FakePort implements WorkspaceEventPort {
  onmessage: ((event: MessageEvent<WorkspaceEventClientMessage>) => void) | null = null
  onmessageerror: (() => void) | null = null
  sent: WorkspaceEventWorkerMessage[] = []

  postMessage(message: WorkspaceEventWorkerMessage) {
    this.sent.push(message)
  }

  start() {}

  close() {}

  receive(message: WorkspaceEventClientMessage) {
    this.onmessage?.({ data: message } as MessageEvent<WorkspaceEventClientMessage>)
  }
}

function workspaceChanges(port: FakePort) {
  return port.sent.flatMap(message => message.type === 'workspace-change' ? [message.event] : [])
}

function workspaceChangeSSE(workspace: string, path: string): SSEEvent {
  return {
    event: 'workspace-change',
    data: JSON.stringify({
      workspace,
      source: 'watcher',
      changes: [{ path, type: 'updated' }],
      paths: [path],
    }),
  }
}

function controlledStream<T>() {
  let controller!: ReadableStreamDefaultController<T>
  const cancel = vi.fn()
  const stream = new ReadableStream<T>({
    start(nextController) {
      controller = nextController
    },
    cancel,
  })
  return {
    stream,
    cancel,
    enqueue(value: T) {
      controller.enqueue(value)
    },
  }
}

async function eventually(assertion: () => void) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      assertion()
      return
    } catch (error) {
      if (attempt === 19) throw error
      await flushMicrotasks()
    }
  }
}

async function flushMicrotasks() {
  await Promise.resolve()
  await Promise.resolve()
}
