import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIError } from './client'
import {
  answerConfigManagerAsk,
  cancelConfigManagerAsk,
  clearConfigManagerSession,
  getActiveConfigManagerTask,
  reconnectConfigManagerStream,
  recoverConfigManagerRuntime,
  runConfigManagerStream,
} from './config-manager'

const scope = {
  origin: 'agent settings',
  resource_id: 'resource/1',
  story_id: 'story 1',
  branch_id: 'branch/main',
}
const projectId = 'project-config'

describe('Config Manager durable runtime API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('uses the exact scope for active inspection, stream attachment, and clear', async () => {
    const fetchMock = vi.fn(async (input: string, _init?: RequestInit) => {
      if (input.includes('/active')) {
        return new Response(JSON.stringify({ active: false, phase: 'idle' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (input.includes('/clear')) {
        return new Response(JSON.stringify({ status: 'ok' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response('', { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await getActiveConfigManagerTask(projectId, scope)
    await reconnectConfigManagerStream(projectId, scope, ' task-exact ', 17)
    await clearConfigManagerSession(projectId, scope)

    const calls = fetchMock.mock.calls as unknown as Array<[string, RequestInit | undefined]>
    expect(calls.map(([url]) => url)).toEqual([
      '/api/projects/project-config/config-manager/active?origin=agent+settings&resource_id=resource%2F1&story_id=story+1&branch_id=branch%2Fmain',
      '/api/projects/project-config/config-manager/stream?origin=agent+settings&resource_id=resource%2F1&story_id=story+1&branch_id=branch%2Fmain&task_id=task-exact&after=17',
      '/api/projects/project-config/config-manager/clear?origin=agent+settings&resource_id=resource%2F1&story_id=story+1&branch_id=branch%2Fmain',
    ])
    expect(calls[1][1]?.method).toBeUndefined()
    expect(calls[2][1]?.method).toBe('POST')
  })

  it('keeps Ask answer and cancellation inside the exact Config Manager scope', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ schema: 'ask.result.v1', id: 'ask/1', status: 'answered' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await answerConfigManagerAsk(projectId, scope, 'ask/1', [{ question_id: 'q1', custom_input: 'answer' }])
    await cancelConfigManagerAsk(projectId, scope, 'ask/1')

    const calls = fetchMock.mock.calls as unknown as Array<[string, RequestInit]>
    expect(calls.map(([url]) => url)).toEqual([
      '/api/projects/project-config/config-manager/asks/ask%2F1/answer?origin=agent+settings&resource_id=resource%2F1&story_id=story+1&branch_id=branch%2Fmain',
      '/api/projects/project-config/config-manager/asks/ask%2F1/cancel?origin=agent+settings&resource_id=resource%2F1&story_id=story+1&branch_id=branch%2Fmain',
    ])
    expect(JSON.parse(String(calls[0][1].body))).toEqual({ answers: [{ question_id: 'q1', custom_input: 'answer' }] })
    expect(JSON.parse(String(calls[1][1].body))).toEqual({ reason: 'user_cancelled' })
  })

  it('posts only the server-projected recovery identity', async () => {
    const action = { action_id: 'follow-up-action', kind: 'follow_up' as const, command_id: 'accepted-follow-up', operation_id: 'operation-1' }
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      task_id: 'recovery-task',
      recovery_action: action,
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await recoverConfigManagerRuntime(projectId, scope, action)

    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/projects/project-config/config-manager/recovery?origin=agent+settings&resource_id=resource%2F1&story_id=story+1&branch_id=branch%2Fmain')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({ action })
  })

  it('requires an exact task identity before reconnecting', async () => {
    vi.stubGlobal('fetch', vi.fn())

    await expect(reconnectConfigManagerStream(projectId, scope, '   ')).rejects.toThrow('exact task ID')
    expect(fetch).not.toHaveBeenCalled()
  })

  it('preserves typed HTTP errors after a streaming rejection', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: 'recovery required',
      code: 'agent_runtime.recovery_required',
    }), { status: 409, headers: { 'Content-Type': 'application/json' } })))

    const error = await runConfigManagerStream(projectId, {
      ...scope,
      command_id: 'config-command',
      instruction: 'update configuration',
    }).catch(reason => reason)

    expect(error).toBeInstanceOf(APIError)
    expect(error).toMatchObject({ status: 409, code: 'agent_runtime.recovery_required' })
  })
})
