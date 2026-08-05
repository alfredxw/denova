import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createAgentCommandID,
  getActiveConfigManagerTask,
  reconnectConfigManagerStream,
  recoverConfigManagerRuntime,
  runConfigManagerStream,
} from '@/lib/api'
import type { ActiveChatTask, AgentRuntimeRecoveryAction } from '@/lib/api'
import { ConfigManagerChat } from './ConfigManagerChat'

const streamMock = vi.hoisted(() => ({ consume: vi.fn(), setMessages: vi.fn() }))
const projectID = 'project-book'

vi.mock('@/lib/api', () => ({
  clearConfigManagerSession: vi.fn(),
  createAgentCommandID: vi.fn(),
  getActiveConfigManagerTask: vi.fn(),
  getConfigManagerMessagesPage: vi.fn().mockResolvedValue({ messages: [], nextBefore: '0', hasMore: false, total: 0 }),
  reconnectConfigManagerStream: vi.fn(),
  recoverConfigManagerRuntime: vi.fn(),
  runConfigManagerStream: vi.fn(),
}))

vi.mock('@/hooks/useAgentUIMessageStream', () => ({
  createAgentDataMessage: (_type: string, data: Record<string, unknown>) => ({ id: 'data', role: 'assistant', parts: [{ type: 'data', data }] }),
  createAgentTextMessage: (role: string, text: string) => ({ id: text, role, parts: [{ type: 'text', text }] }),
  useAgentUIMessageStream: () => ({
    messages: [],
    setMessages: streamMock.setMessages,
    isStreaming: false,
    consumeAgentUIStream: streamMock.consume,
  }),
}))

vi.mock('@/hooks/useSkillCommands', () => ({ useSkillCommands: () => [] }))
vi.mock('./MessageList', () => ({ MessageList: () => null }))
vi.mock('./InputArea', () => ({
  InputArea: ({
    onSend,
    onStop,
    sendBlocked,
  }: {
    onSend: (value: string) => void
    onStop?: () => void
    sendBlocked?: boolean
  }) => (
    <>
      <button type="button" disabled={sendBlocked} onClick={() => onSend('update configuration')}>send config</button>
      {onStop ? <button type="button" onClick={onStop}>stop recovery</button> : null}
    </>
  ),
}))

function emptyStream() {
  return new ReadableStream({ start(controller) { controller.close() } })
}

function idleProjection(): ActiveChatTask {
  return {
    active: false,
    phase: 'idle',
    runtime_recoverable: false,
    stream_attached: false,
    recovery_actions: [],
  }
}

function recoveryAction(
  kind: AgentRuntimeRecoveryAction['kind'],
  commandID: string,
  operationID = 'config-operation',
): AgentRuntimeRecoveryAction {
  return { kind, command_id: commandID, operation_id: operationID }
}

function receipt(action: AgentRuntimeRecoveryAction, taskID = 'config-recovery-task') {
  return {
    task_id: taskID,
    status: 'running',
    stream_cursor: 0,
    cursor: 12,
    replayed: false,
    recovery_action: action,
  }
}

function neverSettles() {
  return new Promise<void>(() => {})
}

describe('ConfigManagerChat durable runtime control', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(createAgentCommandID).mockReset().mockReturnValueOnce('config-command-1').mockReturnValueOnce('config-command-2')
    vi.mocked(getActiveConfigManagerTask).mockResolvedValue(idleProjection())
    vi.mocked(reconnectConfigManagerStream).mockResolvedValue(emptyStream())
    streamMock.consume.mockResolvedValue(undefined)
  })

  it('blocks a new start until the exact scope inspection has settled', async () => {
    let resolveInspection!: (projection: ActiveChatTask) => void
    vi.mocked(getActiveConfigManagerTask).mockReturnValue(new Promise((resolve) => { resolveInspection = resolve }))
    render(<ConfigManagerChat projectId={projectID} origin="settings" />)

    expect(screen.getByRole('button', { name: 'send config' })).toBeDisabled()
    resolveInspection(idleProjection())
    await waitFor(() => expect(screen.getByRole('button', { name: 'send config' })).toBeEnabled())
  })

  it('retains command_id after an uncertain network failure', async () => {
    vi.mocked(runConfigManagerStream)
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce(emptyStream())
    render(<ConfigManagerChat projectId={projectID} origin="settings" />)
    await waitFor(() => expect(getActiveConfigManagerTask).toHaveBeenCalledTimes(1))

    fireEvent.click(screen.getByRole('button', { name: 'send config' }))
    await waitFor(() => expect(runConfigManagerStream).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByRole('button', { name: 'send config' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'send config' }))
    await waitFor(() => expect(runConfigManagerStream).toHaveBeenCalledTimes(2))

    expect(vi.mocked(runConfigManagerStream).mock.calls.map(([, request]) => request.command_id)).toEqual([
      'config-command-1',
      'config-command-1',
    ])
  })

  it('releases command_id after a definite 4xx rejection', async () => {
    vi.mocked(runConfigManagerStream)
      .mockRejectedValueOnce(Object.assign(new Error('rejected'), { status: 400 }))
      .mockResolvedValueOnce(emptyStream())
    render(<ConfigManagerChat projectId={projectID} origin="settings" />)
    await waitFor(() => expect(getActiveConfigManagerTask).toHaveBeenCalledTimes(1))

    fireEvent.click(screen.getByRole('button', { name: 'send config' }))
    await waitFor(() => expect(runConfigManagerStream).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByRole('button', { name: 'send config' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'send config' }))
    await waitFor(() => expect(runConfigManagerStream).toHaveBeenCalledTimes(2))

    expect(vi.mocked(runConfigManagerStream).mock.calls.map(([, request]) => request.command_id)).toEqual([
      'config-command-1',
      'config-command-2',
    ])
  })

  it('drops local command identity as soon as a normal run is accepted', async () => {
    vi.mocked(runConfigManagerStream).mockResolvedValue(emptyStream())
    render(<ConfigManagerChat projectId={projectID} origin="settings" />)
    await waitFor(() => expect(getActiveConfigManagerTask).toHaveBeenCalledTimes(1))

    fireEvent.click(screen.getByRole('button', { name: 'send config' }))
    await waitFor(() => expect(runConfigManagerStream).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByRole('button', { name: 'send config' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'send config' }))
    await waitFor(() => expect(runConfigManagerStream).toHaveBeenCalledTimes(2))

    expect(vi.mocked(runConfigManagerStream).mock.calls.map(([, request]) => request.command_id)).toEqual([
      'config-command-1',
      'config-command-2',
    ])
  })

  it('keeps the accepted POST stream when its active projection refresh fails', async () => {
    vi.mocked(getActiveConfigManagerTask)
      .mockResolvedValueOnce(idleProjection())
      .mockRejectedValueOnce(new TypeError('projection connection reset'))
    vi.mocked(runConfigManagerStream).mockResolvedValue(emptyStream())
    streamMock.consume.mockImplementation(neverSettles)
    render(<ConfigManagerChat projectId={projectID} origin="settings" />)
    await waitFor(() => expect(screen.getByRole('button', { name: 'send config' })).toBeEnabled())

    fireEvent.click(screen.getByRole('button', { name: 'send config' }))

    await waitFor(() => expect(streamMock.consume).toHaveBeenCalledTimes(1))
    expect(reconnectConfigManagerStream).not.toHaveBeenCalled()
    expect(createAgentCommandID).toHaveBeenCalledTimes(1)
  })

  it('remounts onto the server-projected task after a normal run was accepted', async () => {
    const active = { ...idleProjection(), active: true, status: 'running', task_id: 'accepted-config-task', stream_attached: true }
    vi.mocked(runConfigManagerStream).mockResolvedValue(emptyStream())
    streamMock.consume.mockImplementation(neverSettles)
    const first = render(<ConfigManagerChat projectId={projectID} origin="settings" />)
    await waitFor(() => expect(getActiveConfigManagerTask).toHaveBeenCalledTimes(1))
    vi.mocked(getActiveConfigManagerTask).mockResolvedValue(active)

    fireEvent.click(screen.getByRole('button', { name: 'send config' }))
    await waitFor(() => expect(streamMock.consume).toHaveBeenCalledTimes(1))
    expect(reconnectConfigManagerStream).not.toHaveBeenCalled()
    first.unmount()

    render(<ConfigManagerChat projectId={projectID} origin="settings" />)
    await waitFor(() => expect(reconnectConfigManagerStream).toHaveBeenCalledWith(
      projectID,
      expect.objectContaining({ origin: 'settings' }),
      'accepted-config-task',
    ))
    expect(runConfigManagerStream).toHaveBeenCalledTimes(1)
  })

  it('attaches and resumes a cold recovery using only server-projected identities', async () => {
    const attach = recoveryAction('start_turn', 'attach-config')
    const followUp = recoveryAction('follow_up', 'accepted-follow-up')
    vi.mocked(getActiveConfigManagerTask).mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      recovery_actions: [attach, recoveryAction('abort', 'abort-config'), followUp],
    })
    vi.mocked(recoverConfigManagerRuntime)
      .mockResolvedValueOnce(receipt(attach))
      .mockResolvedValueOnce(receipt(followUp))
    streamMock.consume.mockImplementation(neverSettles)

    render(<ConfigManagerChat projectId={projectID} origin="settings" resourceId="resource-1" />)

    await waitFor(() => expect(recoverConfigManagerRuntime).toHaveBeenCalledTimes(2))
    expect(vi.mocked(recoverConfigManagerRuntime).mock.calls).toEqual([
      [projectID, expect.objectContaining({ origin: 'settings', resource_id: 'resource-1' }), attach],
      [projectID, expect.objectContaining({ origin: 'settings', resource_id: 'resource-1' }), followUp],
    ])
    expect(reconnectConfigManagerStream).toHaveBeenCalledTimes(1)
    expect(reconnectConfigManagerStream).toHaveBeenCalledWith(projectID, expect.any(Object), 'config-recovery-task')
    expect(runConfigManagerStream).not.toHaveBeenCalled()
  })

  it('keeps attach-only recovery stoppable and aborts on the same display task', async () => {
    const attach = recoveryAction('start_turn', 'attach-config')
    const abort = recoveryAction('abort', 'abort-config')
    vi.mocked(getActiveConfigManagerTask).mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      recovery_actions: [attach, abort],
    })
    vi.mocked(recoverConfigManagerRuntime)
      .mockResolvedValueOnce(receipt(attach))
      .mockResolvedValueOnce(receipt(abort))
    streamMock.consume.mockImplementation(neverSettles)
    render(<ConfigManagerChat projectId={projectID} origin="settings" />)

    await waitFor(() => expect(screen.getByRole('button', { name: 'stop recovery' })).toBeInTheDocument())
    expect(reconnectConfigManagerStream).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'stop recovery' }))

    await waitFor(() => expect(recoverConfigManagerRuntime).toHaveBeenCalledTimes(2))
    expect(vi.mocked(recoverConfigManagerRuntime).mock.calls.map(([, , action]) => action)).toEqual([attach, abort])
    expect(reconnectConfigManagerStream).toHaveBeenCalledTimes(1)
  })

  it('immediately reattaches one time when an active task stream disconnects', async () => {
    const active = {
      ...idleProjection(),
      active: true,
      status: 'running',
      task_id: 'disconnect-task',
      stream_attached: true,
      stream_cursor: 9,
    }
    vi.mocked(getActiveConfigManagerTask).mockResolvedValue(active)
    streamMock.consume
      .mockRejectedValueOnce(new TypeError('connection reset'))
      .mockImplementationOnce(neverSettles)
    render(<ConfigManagerChat projectId={projectID} origin="settings" />)

    await waitFor(() => expect(reconnectConfigManagerStream).toHaveBeenCalledTimes(2))
    expect(reconnectConfigManagerStream).toHaveBeenNthCalledWith(1, projectID, expect.any(Object), 'disconnect-task')
    expect(reconnectConfigManagerStream).toHaveBeenNthCalledWith(2, projectID, expect.any(Object), 'disconnect-task')

    fireEvent(window, new Event('focus'))
    await waitFor(() => expect(getActiveConfigManagerTask).toHaveBeenCalledTimes(3))
    expect(reconnectConfigManagerStream).toHaveBeenCalledTimes(2)
  })

  it('inspects a newly selected scope without waiting for the previous scope request', async () => {
    let resolveFirst!: (projection: ActiveChatTask) => void
    const firstProjection = new Promise<ActiveChatTask>((resolve) => { resolveFirst = resolve })
    vi.mocked(getActiveConfigManagerTask)
      .mockReturnValueOnce(firstProjection)
      .mockResolvedValueOnce({
        ...idleProjection(),
        active: true,
        status: 'running',
        task_id: 'scope-b-task',
        stream_attached: true,
      })
    streamMock.consume.mockImplementation(neverSettles)
    const view = render(<ConfigManagerChat projectId={projectID} origin="scope-a" />)
    await waitFor(() => expect(getActiveConfigManagerTask).toHaveBeenCalledTimes(1))

    view.rerender(<ConfigManagerChat projectId={projectID} origin="scope-b" />)

    await waitFor(() => expect(getActiveConfigManagerTask).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(reconnectConfigManagerStream).toHaveBeenCalledWith(
      projectID,
      expect.objectContaining({ origin: 'scope-b' }),
      'scope-b-task',
    ))
    resolveFirst(idleProjection())
    expect(vi.mocked(getActiveConfigManagerTask).mock.calls.map(([, scope]) => scope?.origin)).toEqual(['scope-a', 'scope-b'])
  })
})
