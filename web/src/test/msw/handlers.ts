import { http, HttpResponse } from 'msw'

export const handlers = [
  http.get('/api/messages', () =>
    HttpResponse.json({
      items: [],
      unread_count: 0,
    }),
  ),
  http.post('/api/messages/:id/read', ({ params }) =>
    HttpResponse.json({
      id: String(params.id),
      type: 'changelog',
      title: 'Unreleased',
      summary: '',
      body: '',
      read_at: new Date().toISOString(),
    }),
  ),
  http.post('/api/messages/read-all', () =>
    HttpResponse.json({
      items: [],
      unread_count: 0,
    }),
  ),
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
          reply_target_chars: 2000,
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
			event_frequency: 'balanced',
          tags: ['通用'],
          custom: false,
        },
      ],
    }),
  ),
  http.get('/api/styles', () =>
    HttpResponse.json({
      styles: [
        {
          name: '克制细腻',
          description: '动作、对白和停顿承载情绪',
          path: '/tmp/.denova/styles/restraint.md',
          display_path: '.denova/styles/restraint.md',
        },
      ],
    }),
  ),
  http.post('/api/styles', async ({ request }) => {
    const body = await request.json() as { name?: string; filename?: string }
    const filename = body.filename || 'style.md'
    return HttpResponse.json({
      name: body.name || filename,
      description: '',
      path: `/tmp/.denova/styles/${filename}`,
      display_path: `.denova/styles/${filename}`,
    })
  }),
  http.get('/api/styles/file', ({ request }) => {
    const path = new URL(request.url).searchParams.get('path') || '.denova/styles/restraint.md'
    return HttpResponse.json({
      reference: {
        name: '克制细腻',
        description: '动作、对白和停顿承载情绪',
        path: `/tmp/${path.replace(/^\.denova\//, '.denova/')}`,
        display_path: path,
      },
      content: '# 克制细腻\n\n动作、对白和停顿承载情绪。\n',
      revision: 'r1',
    })
  }),
  http.put('/api/styles/file', async ({ request }) => {
    const body = await request.json() as { path?: string; content?: string }
    const path = body.path || '.denova/styles/restraint.md'
    return HttpResponse.json({
      reference: {
        name: '克制细腻',
        description: '动作、对白和停顿承载情绪',
        path: `/tmp/${path.replace(/^\.denova\//, '.denova/')}`,
        display_path: path,
      },
      content: body.content || '',
      revision: 'r2',
    })
  }),
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
          editable_system_prompt: 'search_story_history',
        },
      },
      builtin_agent_prompt_sources: {
        ide: {
          sources: [
            { id: 'runtime_contract', title: '运行契约', source: 'Denova runtime', content: '运行契约测试' },
            { id: 'output_protocol', title: '输出格式', source: 'Denova runtime', content: '输出格式测试' },
            { id: 'creator', title: 'CREATOR.md', source: 'CREATOR.md', content: '创作者指令测试' },
            { id: 'flow', title: '流程规则', source: 'Denova built-in', content: '默认流程测试', editable: true, field: 'flow_prompt' },
            { id: 'custom', title: '用户自定义', source: 'user/workspace config', content: '', editable: true, field: 'system_prompt' },
          ],
        },
        interactive_story: {
          sources: [
            { id: 'runtime_contract', title: '互动运行契约', source: 'Denova runtime', content: '互动运行契约测试' },
            { id: 'output_protocol', title: '互动输出格式', source: 'Denova runtime', content: '互动输出格式测试' },
            { id: 'flow', title: '流程规则', source: 'Denova built-in', content: 'search_story_history', editable: true, field: 'flow_prompt' },
            { id: 'custom', title: '用户自定义', source: 'user/workspace config', content: '', editable: true, field: 'system_prompt' },
          ],
        },
      },
      paths: { nova_dir: '', user_config: '', workspace_config: '' },
    }),
  ),
  http.get('/api/lore/items', () => HttpResponse.json({ items: [] })),
  http.get('/api/config-manager/messages', () => HttpResponse.json([])),
  http.post('/api/command', async ({ request }) => {
    const body = (await request.json()) as { command?: string }
    return HttpResponse.json({ result: `executed:${body.command || ''}` })
  }),
]
