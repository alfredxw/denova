import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIError } from './client'
import { abortAutomationRun, streamAutomationRun, streamAutomationRunMessage } from './automations'

describe('automation Agent command identity', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('sends caller command IDs for initial runs and follow-ups', async () => {
    const fetchMock = vi.fn(async () => new Response('', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await streamAutomationRun('task/1', 'run-command', undefined, [{ source: 'test', title: 'evidence' }])
    await streamAutomationRunMessage('run/1', 'follow-up-command', 'continue')

    const calls = fetchMock.mock.calls as unknown as Array<[string, RequestInit]>
    expect(calls[0][0]).toBe('/api/automations/task%2F1/run/stream')
    expect(JSON.parse(String(calls[0][1].body))).toEqual({
      command_id: 'run-command',
      trigger_evidence: [{ source: 'test', title: 'evidence' }],
    })
    expect(calls[1][0]).toBe('/api/automations/runs/run%2F1/chat/stream')
    expect(JSON.parse(String(calls[1][1].body))).toEqual({ command_id: 'follow-up-command', message: 'continue' })
  })

  it('targets one durable operation when aborting a run', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      command_id: 'abort-command', operation_id: 'operation-1', cursor: 12,
    }), { status: 202, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(abortAutomationRun('run/1', 'abort-command', 'operation-1')).resolves.toMatchObject({ cursor: 12 })

    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/automations/runs/run%2F1/abort')
    expect(JSON.parse(String(init.body))).toEqual({
      command_id: 'abort-command',
      target_operation_id: 'operation-1',
      reason: 'user_requested',
    })
  })

  it('keeps HTTP status on definite streaming rejection', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'missing command' }), {
      status: 400,
      headers: { 'Content-Type': 'application/json' },
    })))

    const error = await streamAutomationRun('task', '', undefined).catch(reason => reason)
    expect(error).toBeInstanceOf(APIError)
    expect(error).toMatchObject({ status: 400, message: 'missing command' })
  })
})
