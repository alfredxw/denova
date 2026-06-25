import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import {
  createSession,
  deleteSession,
  executeCommand,
  getActiveChatTask,
  getMessages,
  getSessions,
  getWorkspaceSummary,
  renameSession,
  saveSkillDocument,
  sendMessage,
  switchSession,
} from './api'
import { server } from '@/test/msw/server'

describe('api', () => {
  it('作品统计接口将空章节列表标准化为空数组', async () => {
    server.use(
      http.get('/api/workspace/summary', () => HttpResponse.json({
        title: '空作品',
        author: '',
        chapter_count: 0,
        total_words: 0,
        chapters: null,
      })),
    )

    await expect(getWorkspaceSummary()).resolves.toMatchObject({ chapters: [] })
  })

  it('通过 MSW 获取会话和活跃任务', async () => {
    await expect(getMessages()).resolves.toEqual([])
    await expect(getSessions()).resolves.toEqual([])
    await expect(getActiveChatTask()).resolves.toEqual({ active: false })
  })

  it('覆盖会话 CRUD、切换和指定会话消息读取', async () => {
    const requests: Array<{ path: string; body?: unknown }> = []
    server.use(
      http.get('/api/session/messages', ({ request }) => {
        requests.push({ path: new URL(request.url).pathname + new URL(request.url).search })
        return HttpResponse.json([{ type: 'message', role: 'user', content: '会话消息' }])
      }),
      http.get('/api/sessions', () => HttpResponse.json({
        sessions: [{ id: 'session-a', title: '会话 A', active: true, message_count: 1, created_at: '', updated_at: '' }],
      })),
      http.post('/api/sessions', async ({ request }) => {
        const body = await request.json()
        requests.push({ path: '/api/sessions', body })
        return HttpResponse.json({ id: 'session-b', title: '会话 B', active: true, message_count: 0, created_at: '', updated_at: '' })
      }),
      http.post('/api/sessions/switch', async ({ request }) => {
        const body = await request.json()
        requests.push({ path: '/api/sessions/switch', body })
        return HttpResponse.json({ id: 'session-a', title: '会话 A', active: true, message_count: 1, created_at: '', updated_at: '' })
      }),
      http.post('/api/sessions/rename', async ({ request }) => {
        const body = await request.json()
        requests.push({ path: '/api/sessions/rename', body })
        return HttpResponse.json({ status: 'ok' })
      }),
      http.post('/api/sessions/delete', async ({ request }) => {
        const body = await request.json()
        requests.push({ path: '/api/sessions/delete', body })
        return HttpResponse.json({ id: 'session-a', title: '会话 A', active: true, message_count: 1, created_at: '', updated_at: '' })
      }),
    )

    await expect(getSessions()).resolves.toHaveLength(1)
    await expect(createSession('会话 B')).resolves.toMatchObject({ id: 'session-b', active: true })
    await expect(switchSession('session-a')).resolves.toMatchObject({ id: 'session-a' })
    await expect(renameSession('session-a', '新标题')).resolves.toBeUndefined()
    await expect(deleteSession('session-b')).resolves.toMatchObject({ id: 'session-a' })
    await expect(getMessages('session-a')).resolves.toEqual([{ type: 'message', role: 'user', content: '会话消息' }])

    expect(requests).toEqual([
      { path: '/api/sessions', body: { title: '会话 B' } },
      { path: '/api/sessions/switch', body: { id: 'session-a' } },
      { path: '/api/sessions/rename', body: { id: 'session-a', title: '新标题' } },
      { path: '/api/sessions/delete', body: { id: 'session-b' } },
      { path: '/api/session/messages?session_id=session-a' },
    ])
  })

  it('发送命令时返回后端结果', async () => {
    await expect(executeCommand('status')).resolves.toBe('executed:status')
  })

  it('保存 Skill 配置时可提交目标 scope 和名称', async () => {
    let requestBody: unknown
    server.use(
      http.put('/api/skills/document', async ({ request }) => {
        requestBody = await request.json()
        return HttpResponse.json({
          name: 'beat-plan',
          description: 'Beat planning',
          scope: 'workspace',
          path: '/books/demo/.nova/skills/beat-plan/SKILL.md',
          editable: true,
          active: true,
          content: '---\nname: beat-plan\ndescription: Beat planning\n---\n',
        })
      }),
    )

    await expect(saveSkillDocument('user', 'draft-plan', 'content', { scope: 'workspace', name: 'beat-plan' })).resolves.toMatchObject({
      scope: 'workspace',
      name: 'beat-plan',
    })
    expect(requestBody).toEqual({
      scope: 'user',
      name: 'draft-plan',
      content: 'content',
      target_scope: 'workspace',
      target_name: 'beat-plan',
    })
  })

  it('发送聊天请求时提交引用、场景风格、选中文本和 planMode，并解析 SSE', async () => {
    let requestBody: unknown
    server.use(
      http.post('/api/chat', async ({ request }) => {
        requestBody = await request.json()
        return new Response(
          'event: chunk\ndata: {"content":"你好"}\n\n' +
          'event: done\ndata: {}\n\n',
          { headers: { 'Content-Type': 'text/event-stream' } },
        )
      }),
    )

    const stream = await sendMessage(
      '写下一章',
      ['chapters/ch01.md'],
      [],
      ['激烈打斗'],
      [{ fileName: 'chapters/ch02.md', startLine: 1, endLine: 2, content: '选中文本' }],
      undefined,
      true,
      'novel-heavy',
    )
    const reader = stream.getReader()

    await expect(reader.read()).resolves.toEqual({
      done: false,
      value: { event: 'chunk', data: '{"content":"你好"}' },
    })
    await expect(reader.read()).resolves.toEqual({
      done: false,
      value: { event: 'done', data: '{}' },
    })

    expect(requestBody).toEqual({
      message: '写下一章',
      references: ['chapters/ch01.md'],
      lore_references: [],
      style_scenes: ['激烈打斗'],
      selections: [{
        file_name: 'chapters/ch02.md',
        start_line: 1,
        end_line: 2,
        content: '选中文本',
      }],
      plan_mode: true,
      writing_skill: 'novel-heavy',
    })
  })

  it('聊天接口失败时抛出 HTTP 错误', async () => {
    server.use(
      http.post('/api/chat', () => HttpResponse.text('bad gateway', { status: 502 })),
    )

    await expect(sendMessage('失败场景')).rejects.toThrow('HTTP 502')
  })
})
