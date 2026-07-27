import { StrictMode } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatTerminalTab } from '../types'
import type { TerminalSessionInfo } from './api'
import { TerminalTabView } from './TerminalTabView'

const apiMocks = vi.hoisted(() => ({
  closeTerminalSession: vi.fn(),
  createTerminalSession: vi.fn(),
  getTerminalRuntimeStatus: vi.fn(),
}))

const connectionMocks = vi.hoisted(() => ({
  handlers: [] as Array<{
    onControl: (frame: { type: string; code?: number; error?: string }) => void
  }>,
}))

vi.mock('./api', () => ({
  ...apiMocks,
  terminalAttachURL: (id: string) => `ws://terminal/${id}`,
}))

vi.mock('./connection', () => ({
  TerminalConnection: class {
    constructor(_url: string, handlers: { onControl: (frame: { type: string }) => void }) {
      connectionMocks.handlers.push(handlers)
    }

    close() {}
    resize() {}
    send() {}
  },
}))

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80
    rows = 24
    options: Record<string, unknown> = {}

    dispose() {}
    loadAddon() {}
    onData() {}
    onResize() {}
    open() {}
    reset() {}
    write() {}
  },
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit() {}
  },
}))

vi.mock('@xterm/addon-webgl', () => ({
  WebglAddon: class {
    dispose() {}
    onContextLoss() {}
  },
}))

const tab: AgentChatTerminalTab = {
  kind: 'terminal',
  id: 'terminal-tab-1',
  workspace: '/books/a',
  profileId: 'shell',
  title: '',
}

function session(id: string): TerminalSessionInfo {
  return {
    id,
    owner_tab_id: tab.id,
    profile_id: 'shell',
    title: 'shell',
    command: '/bin/sh',
    args: [],
    cwd: tab.workspace,
    workspace: tab.workspace,
    cols: 80,
    rows: 24,
    created_at: '2026-07-26T00:00:00Z',
    attached: 0,
    exited: false,
    exit_code: 0,
    token: `token-${id}`,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((complete) => { resolve = complete })
  return { promise, resolve }
}

describe('TerminalTabView session lifecycle', () => {
  beforeEach(() => {
    apiMocks.closeTerminalSession.mockReset().mockResolvedValue(undefined)
    apiMocks.createTerminalSession.mockReset()
    apiMocks.getTerminalRuntimeStatus.mockReset().mockResolvedValue({ sessions: [] })
    connectionMocks.handlers.length = 0
  })

  it('creates one backend session under a StrictMode effect replay', async () => {
    const creation = deferred<TerminalSessionInfo>()
    apiMocks.createTerminalSession.mockReturnValue(creation.promise)
    const onSessionEstablished = vi.fn(() => true)

    render(
      <StrictMode>
        <TerminalTabView tab={tab} active onSessionEstablished={onSessionEstablished} />
      </StrictMode>,
    )

    await waitFor(() => expect(apiMocks.createTerminalSession).toHaveBeenCalledTimes(1))
    expect(apiMocks.createTerminalSession).toHaveBeenCalledWith(expect.objectContaining({ owner_tab_id: tab.id }))
    const request = apiMocks.createTerminalSession.mock.calls[0][0]
    expect(request).not.toHaveProperty('command')
    expect(request).not.toHaveProperty('args')
    await act(async () => { creation.resolve(session('session-1')); await creation.promise })
    await waitFor(() => expect(onSessionEstablished).toHaveBeenCalled())
    expect(apiMocks.createTerminalSession).toHaveBeenCalledTimes(1)
  })

  it('reattaches a persisted session after reload without creating another process', async () => {
    const persisted = session('persisted-session')
    apiMocks.getTerminalRuntimeStatus.mockResolvedValue({ sessions: [persisted] })
    const onSessionEstablished = vi.fn(() => true)

    render(
      <TerminalTabView
        tab={{ ...tab, terminalSessionId: persisted.id }}
        active
        onSessionEstablished={onSessionEstablished}
      />,
    )

    await waitFor(() => expect(onSessionEstablished).toHaveBeenCalledWith(tab.id, persisted))
    expect(apiMocks.getTerminalRuntimeStatus).toHaveBeenCalledTimes(1)
    expect(apiMocks.createTerminalSession).not.toHaveBeenCalled()
  })

  it('releases a session whose tab closes before creation finishes', async () => {
    const creation = deferred<TerminalSessionInfo>()
    apiMocks.createTerminalSession.mockReturnValue(creation.promise)
    let owned = true
    const view = render(
      <TerminalTabView tab={tab} active onSessionEstablished={() => owned} />,
    )
    await waitFor(() => expect(apiMocks.createTerminalSession).toHaveBeenCalledTimes(1))

    owned = false
    view.unmount()
    await act(async () => { creation.resolve(session('abandoned-session')); await creation.promise })

    await waitFor(() => expect(apiMocks.closeTerminalSession).toHaveBeenCalledWith('abandoned-session'))
  })

  it('closes the old process before restarting the same terminal tab', async () => {
    const user = userEvent.setup()
    apiMocks.createTerminalSession
      .mockResolvedValueOnce(session('session-1'))
      .mockResolvedValueOnce(session('session-2'))

    render(<TerminalTabView tab={tab} active onSessionEstablished={() => true} />)
    await waitFor(() => expect(connectionMocks.handlers).toHaveLength(1))
    act(() => connectionMocks.handlers[0].onControl({ type: 'exit', code: 0, error: '' }))

    await user.click(await screen.findByRole('button', { name: '重新启动' }))

    await waitFor(() => expect(apiMocks.closeTerminalSession).toHaveBeenCalledWith('session-1'))
    await waitFor(() => expect(apiMocks.createTerminalSession).toHaveBeenCalledTimes(2))
    expect(apiMocks.createTerminalSession).toHaveBeenLastCalledWith(expect.objectContaining({ owner_tab_id: tab.id }))
  })
})
