import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  answerSessionAsk,
  cancelSessionAsk,
  createSession,
  deleteSession,
  executeCommand,
  getActiveChatTask,
  getMessages,
  getMessagesPage,
  getSessions,
  projectSkillTarget,
  renameSession,
  recoverChatAgentRuntime,
  saveSkillDocument,
  sendMessage,
  switchSession,
} from './api'
import { apiRoute, installApiMock, jsonResponse, textResponse } from '@/test/api-mock'

let apiMock: ReturnType<typeof installApiMock>

describe('api', () => {
  beforeEach(() => {
    apiMock = installApiMock(
      apiRoute.get('/api/session/messages', () => jsonResponse([])),
      apiRoute.get('/api/sessions', () => jsonResponse({ sessions: [] })),
      apiRoute.get('/api/chat/active', () => jsonResponse({ active: false })),
      apiRoute.post('/api/command', async ({ request }) => {
        const body = await request.json() as { command?: string }
        return jsonResponse({ result: `executed:${body.command || ''}` })
      }),
    )
  })

  afterEach(() => vi.unstubAllGlobals())

  it('answers and cancels only the exact session Ask identity', async () => {
    const requests: Array<{ path: string; body: unknown }> = []
    apiMock.use(
      apiRoute.post('/api/session/asks/:askID/answer', async ({ request }) => {
        requests.push({ path: new URL(request.url).pathname, body: await request.json() })
        return jsonResponse({ schema: 'ask.result.v1', id: 'ask/1', status: 'answered' })
      }),
      apiRoute.post('/api/session/asks/:askID/cancel', async ({ request }) => {
        requests.push({ path: new URL(request.url).pathname, body: await request.json() })
        return jsonResponse({ schema: 'ask.result.v1', id: 'ask/1', status: 'cancelled' })
      }),
    )

    await answerSessionAsk('session-a', 'ask/1', [{ question_id: 'q1', selected_option_ids: ['safe'] }])
    await cancelSessionAsk('session-a', 'ask/1')

    expect(requests).toEqual([
      { path: '/api/session/asks/ask%2F1/answer', body: { session_id: 'session-a', answers: [{ question_id: 'q1', selected_option_ids: ['safe'] }] } },
      { path: '/api/session/asks/ask%2F1/cancel', body: { session_id: 'session-a', reason: 'user_cancelled' } },
    ])
  })

  it('通过 MSW 获取会话和活跃任务', async () => {
    await expect(getMessages()).resolves.toEqual([])
    await expect(getSessions()).resolves.toEqual([])
    await expect(getActiveChatTask('session-a')).resolves.toEqual({ active: false })
  })

  it('恢复已接受运行时只回传服务器投影的 identity', async () => {
    let requestBody: unknown
    apiMock.use(
      apiRoute.post('/api/chat/recovery', async ({ request }) => {
        requestBody = await request.json()
        return jsonResponse({
          task_id: 'recovery-task-1',
          status: 'running',
          stream_cursor: 0,
          cursor: 7,
          recovery_action: {
            kind: 'follow_up',
            command_id: 'follow-command-1',
            operation_id: 'operation-1',
          },
        }, { status: 202 })
      }),
    )
    const action = {
      kind: 'follow_up' as const,
      command_id: 'follow-command-1',
      operation_id: 'operation-1',
    }

    await expect(recoverChatAgentRuntime(action, 'session-a')).resolves.toMatchObject({ task_id: 'recovery-task-1' })
    expect(requestBody).toEqual({ session_id: 'session-a', action })
    expect(JSON.stringify(requestBody)).not.toContain('message')
  })

  it('覆盖会话 CRUD、切换和指定会话消息读取', async () => {
    const requests: Array<{ path: string; body?: unknown }> = []
    apiMock.use(
      apiRoute.get('/api/session/messages', ({ request }) => {
        requests.push({
          path: new URL(request.url).pathname + new URL(request.url).search,
        })
        return jsonResponse([
          {
            id: 'message-1',
            role: 'user',
            parts: [{ type: 'text', text: '会话消息' }],
          },
        ])
      }),
      apiRoute.get('/api/sessions', () =>
        jsonResponse({
          sessions: [
            {
              id: 'session-a',
              title: '会话 A',
              active: true,
              message_count: 1,
              created_at: '',
              updated_at: '',
            },
          ],
        }),
      ),
      apiRoute.post('/api/sessions', async ({ request }) => {
        const body = await request.json()
        requests.push({ path: '/api/sessions', body })
        return jsonResponse({
          id: 'session-b',
          title: '会话 B',
          active: true,
          message_count: 0,
          created_at: '',
          updated_at: '',
        })
      }),
      apiRoute.post('/api/sessions/switch', async ({ request }) => {
        const body = await request.json()
        requests.push({ path: '/api/sessions/switch', body })
        return jsonResponse({
          id: 'session-a',
          title: '会话 A',
          active: true,
          message_count: 1,
          created_at: '',
          updated_at: '',
        })
      }),
      apiRoute.post('/api/sessions/rename', async ({ request }) => {
        const body = await request.json()
        requests.push({ path: '/api/sessions/rename', body })
        return jsonResponse({ status: 'ok' })
      }),
      apiRoute.post('/api/sessions/delete', async ({ request }) => {
        const body = await request.json()
        requests.push({ path: '/api/sessions/delete', body })
        return jsonResponse({
          id: 'session-a',
          title: '会话 A',
          active: true,
          message_count: 1,
          created_at: '',
          updated_at: '',
        })
      }),
    )

    await expect(getSessions()).resolves.toHaveLength(1)
    await expect(createSession('会话 B')).resolves.toMatchObject({
      id: 'session-b',
      active: true,
    })
    await expect(switchSession('session-a')).resolves.toMatchObject({
      id: 'session-a',
    })
    await expect(renameSession('session-a', '新标题')).resolves.toBeUndefined()
    await expect(deleteSession('session-b')).resolves.toMatchObject({
      id: 'session-a',
    })
    await expect(getMessages('session-a')).resolves.toEqual([
      {
        id: 'message-1',
        role: 'user',
        parts: [{ type: 'text', text: '会话消息' }],
      },
    ])

    expect(requests).toEqual([
      { path: '/api/sessions', body: { title: '会话 B' } },
      { path: '/api/sessions/switch', body: { id: 'session-a' } },
      {
        path: '/api/sessions/rename',
        body: { id: 'session-a', title: '新标题' },
      },
      { path: '/api/sessions/delete', body: { id: 'session-b' } },
      { path: '/api/session/messages?session_id=session-a' },
    ])
  })

  it('读取 AI SDK UI 消息历史时使用 canonical 消息接口', async () => {
    const requests: string[] = []
    apiMock.use(
      apiRoute.get('/api/session/messages', ({ request }) => {
        requests.push(new URL(request.url).pathname + new URL(request.url).search)
        return jsonResponse([
          {
            id: 'message-1',
            role: 'assistant',
            parts: [{ type: 'text', text: '你好', state: 'done' }],
          },
        ])
      }),
    )

    await expect(getMessages('session-ui')).resolves.toEqual([
      {
        id: 'message-1',
        role: 'assistant',
        parts: [{ type: 'text', text: '你好', state: 'done' }],
      },
    ])
    expect(requests).toEqual(['/api/session/messages?session_id=session-ui'])
  })

  it('从最新消息向前分页读取会话展示历史', async () => {
    let requestPath = ''
    apiMock.use(
      apiRoute.get('/api/session/messages', ({ request }) => {
        requestPath = new URL(request.url).pathname + new URL(request.url).search
        return jsonResponse({
          messages: [
            {
              id: 'message-older',
              role: 'user',
              parts: [{ type: 'text', text: '更早消息' }],
            },
          ],
          page: { next_before: '25', has_more: true, total: 125 },
        })
      }),
    )

    await expect(getMessagesPage('session-ui', { limit: 50, before: '75' })).resolves.toEqual({
      messages: [
        {
          id: 'message-older',
          role: 'user',
          parts: [{ type: 'text', text: '更早消息' }],
        },
      ],
      nextBefore: '25',
      hasMore: true,
      total: 125,
    })
    expect(requestPath).toBe('/api/session/messages?session_id=session-ui&limit=50&before=75')
  })

  it('发送命令时返回后端结果', async () => {
    await expect(executeCommand('status')).resolves.toBe('executed:status')
  })

  it('保存 Skill 配置时可提交目标 scope、名称和基础 revision', async () => {
    let requestBody: unknown
    apiMock.use(
      apiRoute.put('/api/projects/project-demo/skills/document', async ({ request }) => {
        requestBody = await request.json()
        return jsonResponse({
          name: 'beat-plan',
          description: 'Beat planning',
          scope: 'workspace',
          path: '/books/demo/.nova/skills/beat-plan/SKILL.md',
          editable: true,
          active: true,
          content: '---\nname: beat-plan\ndescription: Beat planning\n---\n',
          revision: 'skill-r2',
        })
      }),
    )

    await expect(saveSkillDocument(projectSkillTarget('project-demo'), 'user', 'draft-plan', 'content', { scope: 'workspace', name: 'beat-plan' }, 'skill-r1')).resolves.toMatchObject({
      scope: 'workspace',
      name: 'beat-plan',
    })
    expect(requestBody).toEqual({
      scope: 'user',
      name: 'draft-plan',
      content: 'content',
      target_scope: 'workspace',
      target_name: 'beat-plan',
      base_revision: 'skill-r1',
    })
  })

  it('发送聊天请求时提交引用、场景风格、选中文本和 planMode，并解析 SSE', async () => {
    let requestBody: unknown
    apiMock.use(
      apiRoute.post('/api/chat', async ({ request }) => {
        requestBody = await request.json()
        return new Response(
          'data: {"type":"start","messageId":"assistant-1"}\n\n' +
            'data: {"type":"text-start","id":"text-1"}\n\n' +
            'data: {"type":"text-delta","id":"text-1","delta":"你好"}\n\n' +
            'data: {"type":"text-end","id":"text-1"}\n\n' +
            'data: {"type":"finish","finishReason":"stop"}\n\n' +
            'data: [DONE]\n\n',
          { headers: { 'Content-Type': 'text/event-stream' } },
        )
      }),
    )

    const stream = await sendMessage(
      'session-a',
      '写下一章',
      ['chapters/ch01.md'],
      [],
      ['激烈打斗'],
      [
        {
          fileName: 'chapters/ch02.md',
          startLine: 1,
          endLine: 2,
          content: '选中文本',
        },
      ],
      undefined,
      true,
      'scene-first',
      {
        currentFile: 'chapters/ch02.md',
        openFiles: ['chapters/ch01.md', 'chapters/ch02.md'],
      },
      'game-cg',
      'slow-burn',
      'command-direct-chat',
    )
    const reader = stream.getReader()

    await expect(reader.read()).resolves.toEqual({
      done: false,
      value: { type: 'start', messageId: 'assistant-1' },
    })
    await expect(reader.read()).resolves.toEqual({
      done: false,
      value: { type: 'text-start', id: 'text-1' },
    })

    expect(requestBody).toEqual({
      session_id: 'session-a',
      command_id: 'command-direct-chat',
      message: '写下一章',
      references: ['chapters/ch01.md'],
      lore_references: [],
      style_scenes: ['激烈打斗'],
      selections: [
        {
          file_name: 'chapters/ch02.md',
          start_line: 1,
          end_line: 2,
          content: '选中文本',
        },
      ],
      ide_context: {
        current_file: 'chapters/ch02.md',
        open_files: ['chapters/ch01.md', 'chapters/ch02.md'],
      },
      plan_mode: true,
      writing_skill: 'scene-first',
      image_preset_id: 'game-cg',
      teller_id: 'slow-burn',
    })
  })

  it('聊天接口失败时抛出 HTTP 错误', async () => {
    apiMock.use(apiRoute.post('/api/chat', () => textResponse('bad gateway', { status: 502 })))

    await expect(sendMessage('session-a', '失败场景')).rejects.toThrow('HTTP 502')
  })
})
