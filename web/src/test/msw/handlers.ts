import { http, HttpResponse } from 'msw'

export const handlers = [
  http.get('/api/session/messages', () => HttpResponse.json([])),
  http.get('/api/sessions', () => HttpResponse.json({ sessions: [] })),
  http.post('/api/sessions', async ({ request }) => {
    const body = (await request.json()) as { title?: string }
    return HttpResponse.json({
      id: 'session-new',
      title: body.title || '新会话',
      created_at: '2026-05-17T00:00:00Z',
      updated_at: '2026-05-17T00:00:00Z',
      active: true,
      message_count: 0,
    })
  }),
  http.post('/api/sessions/switch', async ({ request }) => {
    const body = (await request.json()) as { id?: string }
    return HttpResponse.json({
      id: body.id || 'session-a',
      title: '目标会话',
      created_at: '2026-05-17T00:00:00Z',
      updated_at: '2026-05-17T00:00:00Z',
      active: true,
      message_count: 1,
    })
  }),
  http.post('/api/sessions/rename', () => HttpResponse.json({ status: 'ok' })),
  http.post('/api/sessions/delete', () =>
    HttpResponse.json({
      id: 'session-fallback',
      title: '剩余会话',
      created_at: '2026-05-17T00:00:00Z',
      updated_at: '2026-05-17T00:00:00Z',
      active: true,
      message_count: 0,
    }),
  ),
  http.get('/api/chat/active', () => HttpResponse.json({ active: false })),
  http.get('/api/skills', () => HttpResponse.json({ skills: [] })),
  http.get('/api/interactive/stories', () =>
    HttpResponse.json({
      current_story_id: 'st_1',
      stories: [
        {
          id: 'st_1',
          title: '末日开端',
          origin: '',
          story_teller_id: 'classic',
          reply_target_chars: 1200,
          created_at: '',
          updated_at: '',
          branches: 1,
          events: 0,
        },
      ],
    }),
  ),
  http.get('/api/interactive/stories/:id/snapshot', () =>
    HttpResponse.json({
      story_id: 'st_1',
      branch_id: 'main',
      turns: [],
      state: { on_stage: [], characters: {}, events: [] },
    }),
  ),
  http.get('/api/interactive/stories/:id/memory', ({ params, request }) => {
    const branch = new URL(request.url).searchParams.get('branch') || 'main'
    return HttpResponse.json({
      story_id: params.id,
      branch_id: branch,
      entries: [],
      sync_status: '',
    })
  }),
  http.get('/api/interactive/stories/:id/branches', () =>
    HttpResponse.json({
      branches: [{ id: 'main', head: '', created_at: '', current: true }],
    }),
  ),
  http.get('/api/interactive/tellers', () =>
    HttpResponse.json({
      tellers: [
        {
          id: 'classic',
          name: '经典导演',
          description: '平衡叙事',
          random_event_rate: 0.15,
          tags: ['通用'],
          custom: false,
        },
      ],
    }),
  ),
  http.get('/api/workspace/file', () =>
    HttpResponse.json({
      path: 'setting/characters.md',
      content: '# Characters',
    }),
  ),
  http.get('/api/workspace/summary', () =>
    HttpResponse.json({
      title: '末日开端',
      author: '',
      chapter_count: 0,
      total_words: 0,
      chapters: [],
    }),
  ),
  http.get('/api/settings', () =>
    HttpResponse.json({
      default: {},
      global: {},
      user: {},
      workspace: {},
      effective: {
        max_open_tabs: 5,
        ui_font_family: 'apple-system',
        ui_font_size: 14,
        reading_font_family: 'source-han-serif',
        reading_font_size: 18,
        interactive_stage_line_height: 1.78,
      },
      builtin_agent_prompt_blocks: {
        ide: {
          runtime_contract: '运行契约测试',
          output_protocol: '输出格式测试',
          editable_system_prompt: '默认流程测试',
        },
        interactive_story: {
          runtime_contract: '互动运行契约测试',
          output_protocol: '互动输出格式测试',
          editable_system_prompt: 'list_interactive_memories read_interactive_memories',
        },
      },
      builtin_agent_prompt_sources: {
        ide: {
          sources: [
            { id: 'runtime_contract', title: '运行契约', source: 'Nova runtime', content: '运行契约测试' },
            { id: 'output_protocol', title: '输出格式', source: 'Nova runtime', content: '输出格式测试' },
            { id: 'creator', title: 'CREATOR.md', source: 'CREATOR.md', content: '创作者指令测试' },
            { id: 'flow', title: '流程规则', source: 'Nova built-in', content: '默认流程测试', editable: true, field: 'flow_prompt' },
            { id: 'custom', title: '用户自定义', source: 'user/workspace config', content: '', editable: true, field: 'system_prompt' },
          ],
        },
        interactive_story: {
          sources: [
            { id: 'runtime_contract', title: '互动运行契约', source: 'Nova runtime', content: '互动运行契约测试' },
            { id: 'output_protocol', title: '互动输出格式', source: 'Nova runtime', content: '互动输出格式测试' },
            { id: 'flow', title: '流程规则', source: 'Nova built-in', content: 'list_interactive_memories read_interactive_memories', editable: true, field: 'flow_prompt' },
            { id: 'custom', title: '用户自定义', source: 'user/workspace config', content: '', editable: true, field: 'system_prompt' },
          ],
        },
      },
      paths: { nova_dir: '', user_config: '', workspace_config: '' },
    }),
  ),
  http.get('/api/lore/items', () => HttpResponse.json({ items: [] })),
  http.post('/api/lore/agent', () =>
    HttpResponse.json({
      message: '已更新资料库',
      items: [],
      created: [],
      updated: [],
      deleted_ids: [],
    }),
  ),
  http.get('/api/styles', () => HttpResponse.json({ styles: ['古龙.md', '番茄.txt'] })),
  http.post('/api/command', async ({ request }) => {
    const body = (await request.json()) as { command?: string }
    return HttpResponse.json({ result: `executed:${body.command || ''}` })
  }),
]
