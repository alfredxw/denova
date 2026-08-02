import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchAPI, requestJSON } from '@/lib/api-client'
import { recoverInteractiveAgentRuntime, sendInteractiveMessage, streamActiveInteractiveChat, submitInteractiveAgentCommand } from './api'

vi.mock('@/lib/api-client', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/api-client')>(),
  fetchAPI: vi.fn(),
  requestJSON: vi.fn(),
}))

describe('interactive agent command API', () => {
  beforeEach(() => {
    vi.mocked(fetchAPI).mockReset()
    vi.mocked(requestJSON).mockReset()
    vi.mocked(requestJSON).mockResolvedValue({ command_id: 'command-1', operation_id: 'operation-1', cursor: 9 })
  })

  it('sends the caller-owned command id before opening the initial game stream', async () => {
    vi.mocked(fetchAPI).mockResolvedValue(new Response('', { status: 200 }))

    await sendInteractiveMessage({
      command_id: 'game-start-1',
      mode: 'story',
      story_id: 'story-1',
      branch: 'main',
      message: '推开石门',
      style_scenes: ['雨夜'],
    })

    const init = vi.mocked(fetchAPI).mock.calls[0]?.[1]
    expect(JSON.parse(String(init?.body))).toEqual({
      command_id: 'game-start-1',
      mode: 'story',
      story_id: 'story-1',
      branch: 'main',
      message: '推开石门',
      style_scenes: ['雨夜'],
    })
  })

  it('preserves the HTTP status needed to classify initial acceptance', async () => {
    vi.mocked(fetchAPI).mockResolvedValue(new Response(JSON.stringify({ error: 'temporary failure' }), {
      status: 503,
      headers: { 'Content-Type': 'application/json' },
    }))

    await expect(sendInteractiveMessage({
      command_id: 'game-start-uncertain',
      mode: 'story',
      story_id: 'story-1',
      message: '推开石门',
    })).rejects.toMatchObject({ status: 503, message: 'temporary failure' })
  })

  it('does not attach an input object to abort', async () => {
    await submitInteractiveAgentCommand({
      type: 'abort',
      commandId: 'command-stop',
      targetOperationId: 'operation-1',
      storyId: 'story-1',
      branchId: 'main',
      reason: 'user_requested',
    })

    const init = vi.mocked(requestJSON).mock.calls[0]?.[1]
    expect(JSON.parse(String(init?.body))).toEqual({
      type: 'abort',
      command_id: 'command-stop',
      target_operation_id: 'operation-1',
      story_id: 'story-1',
      branch_id: 'main',
      reason: 'user_requested',
    })
  })

  it('submits game follow-ups and queued steering through the active operation', async () => {
    await submitInteractiveAgentCommand({
      type: 'follow_up',
      commandId: 'command-follow-up',
      targetOperationId: 'operation-1',
      storyId: 'story-1',
      branchId: 'main',
      input: { message: '先检查石门', styleScenes: ['雨夜'] },
    })
    await submitInteractiveAgentCommand({
      type: 'steer_queued',
      commandId: 'command-steer',
      targetOperationId: 'operation-1',
      targetCommandId: 'command-follow-up',
      storyId: 'story-1',
      branchId: 'main',
    })

    const followUpInit = vi.mocked(requestJSON).mock.calls[0]?.[1]
    expect(JSON.parse(String(followUpInit?.body))).toEqual({
      type: 'follow_up',
      command_id: 'command-follow-up',
      target_operation_id: 'operation-1',
      story_id: 'story-1',
      branch_id: 'main',
      input: { message: '先检查石门', style_scenes: ['雨夜'] },
    })
    const steerInit = vi.mocked(requestJSON).mock.calls[1]?.[1]
    expect(JSON.parse(String(steerInit?.body))).toEqual({
      type: 'steer_queued',
      command_id: 'command-steer',
      target_operation_id: 'operation-1',
      target_command_id: 'command-follow-up',
      story_id: 'story-1',
      branch_id: 'main',
    })
  })

  it('recovers game work with only the server-projected action and resource binding', async () => {
    vi.mocked(requestJSON).mockResolvedValue({
      task_id: 'recovery-task-1',
      status: 'running',
      stream_cursor: 0,
      cursor: 9,
      replayed: true,
      recovery_action: {
        kind: 'next_turn',
        command_id: 'next-1',
        operation_id: 'operation-1',
      },
    })
    await recoverInteractiveAgentRuntime({
      storyId: 'story-1',
      branchId: 'main',
      action: {
        kind: 'next_turn',
        command_id: 'next-1',
        operation_id: 'operation-1',
      },
    })

    const init = vi.mocked(requestJSON).mock.calls[0]?.[1]
    const body = JSON.parse(String(init?.body))
    expect(vi.mocked(requestJSON).mock.calls[0]?.[0]).toBe('/api/interactive/chat/recovery')
    expect(body).toEqual({
      action: {
        kind: 'next_turn',
        command_id: 'next-1',
        operation_id: 'operation-1',
      },
      story_id: 'story-1',
      branch: 'main',
    })
    expect(body).not.toHaveProperty('message')
  })

  it('refuses an unscoped game stream reconnect', async () => {
    await expect(streamActiveInteractiveChat({ storyId: 'story-1' }))
      .rejects.toThrow('exact Agent stream task')
  })
})
