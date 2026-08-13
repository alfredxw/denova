import { describe, expect, it } from 'vitest'
import type { AgentUIMessage } from './agent-ui'
import {
  agentViewStableKey,
  agentViewToRenderMessage,
  buildAgentMessageViews,
  countCompletedAgentTurnSignals,
  hasCompletedAgentTurn,
  isAgentTraceView,
  selectAgentTokenUsageRecords,
} from './agent-message-view'

describe('agent-message-view', () => {
  it('工具名和参数流到达时立即更新同一张 Writing 工具卡', () => {
    const message = (state: 'input-streaming' | 'input-available', input: unknown) => ({
      id: 'assistant-live-tool',
      role: 'assistant' as const,
      metadata: { run_id: 'run-live-tool' },
      parts: [{
        type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-live', state, input,
      }],
    }) as AgentUIMessage

    const live = buildAgentMessageViews([message('input-streaming', `{"path":"draft`)])[0]
    const started = buildAgentMessageViews([message('input-available', { path: 'draft.md' })])[0]

    expect(agentViewToRenderMessage(live)).toMatchObject({
      id: 'tool-live', role: 'tool_call', name: 'read', args: `{"path":"draft`, streaming: true,
    })
    expect(agentViewToRenderMessage(started)).toMatchObject({
      id: 'tool-live', role: 'tool_call', name: 'read', args: '{"path":"draft.md"}', streaming: false,
    })
    expect(agentViewStableKey(started)).toBe(agentViewStableKey(live))
  })

  it('uses provider display segment identity across live and persisted message shapes', () => {
    const view = (messageId: string) => buildAgentMessageViews([{
      id: messageId,
      role: 'assistant',
      parts: [{
        type: 'reasoning',
        text: '正在检查。',
        providerMetadata: { agent: { run_id: 'run-stable', display_segment_id: 'reasoning-stable' } },
      }],
    } as AgentUIMessage])[0]

    const live = view('live-message')
    const persisted = view('persisted-message')
    expect(live.metadata.display_segment_id).toBe('reasoning-stable')
    expect(agentViewStableKey(persisted)).toBe(agentViewStableKey(live))
  })

  it('复用未变化消息的 view，只重建正在变化的流式消息', () => {
    const [historyMessage, firstStreamingMessage] = [
      {
        id: 'history-assistant',
        role: 'assistant',
        parts: [{ type: 'text', text: '已经渲染的历史正文' }],
      },
      {
        id: 'active-assistant',
        role: 'assistant',
        parts: [{ type: 'text', text: '第一段', state: 'streaming' }],
      },
    ] as AgentUIMessage[]

    const firstViews = buildAgentMessageViews([historyMessage, firstStreamingMessage])
    const secondViews = buildAgentMessageViews([
      historyMessage,
      {
        ...firstStreamingMessage,
        parts: [{ type: 'text', text: '第一段，继续生成', state: 'streaming' }],
      },
    ] as AgentUIMessage[])

    expect(secondViews[0]).toBe(firstViews[0])
    expect(secondViews[1]).not.toBe(firstViews[1])
    expect(secondViews[1].content).toBe('第一段，继续生成')
  })

  it('从 AgentUIMessage parts 生成稳定渲染 view', () => {
    const messages: AgentUIMessage[] = [
      { id: 'hidden-user', role: 'user', metadata: { display_hidden: true }, parts: [{ type: 'text', text: 'hidden' }] },
      { id: 'user-1', role: 'user', metadata: { turn_id: 'turn-1' }, parts: [{ type: 'text', text: '写下一章' }] },
      {
        id: 'assistant-1',
        role: 'assistant',
        metadata: { run_id: 'run-1', display_phase: 'final' },
        parts: [
          { type: 'reasoning', id: 'reason-1', text: '先分析', state: 'streaming' },
          { type: 'text', id: 'text-1', text: '正文', state: 'done' },
          { type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-1', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
          { type: 'data-agent-context-compaction', id: 'compact-1', data: { content: '压缩上下文', status: 'running', tokens_before: 100 } },
          { type: 'data-agent-proposed-plan', id: 'plan-1', data: { content: '# Plan', status: 'success' } },
          { type: 'data-agent-token-usage', id: 'usage-1', data: { run_id: 'run-1', model_calls: 1, total_tokens: 42, usage_calls: [{ index: 1, total_tokens: 42 }] } },
          { type: 'data-agent-rule-roll', id: 'roll-1', data: { rule_roll: { label: '检定', total: 18 } } },
          {
            type: 'data-agent-interactive-image',
            id: 'image-1',
            providerMetadata: { agent: { tool_presentation: { call: 'image', result: 'interactive_media' } } },
            data: {
              name: 'generate_interactive_image',
              status: 'success',
              interactive_image: { schema: 'interactive_image.v1', story_id: 'story', branch_id: 'main', turn_id: 'turn-1', image_path: 'assets/scene.png', meta_path: 'assets/scene.json' },
            },
          },
          { type: 'data-agent-error', id: 'error-1', data: { content: '失败' } },
          { type: 'data-agent-clear', id: 'clear-1', data: { created_at: '2026-01-01T00:00:00Z' } },
        ],
      },
    ] as AgentUIMessage[]

    const views = buildAgentMessageViews(messages)

    expect(views.map((view) => view.kind)).toEqual([
      'user',
      'reasoning',
      'assistant',
      'tool',
      'context-compaction',
      'proposed-plan',
      'token-usage',
      'rule-roll',
      'interactive-image',
      'error',
      'clear',
    ])
    expect(views[0]).toMatchObject({ messageId: 'user-1', content: '写下一章', metadata: { turn_id: 'turn-1' } })
    expect(views[1]).toMatchObject({ partId: 'reason-1', streaming: true, metadata: { run_id: 'run-1' } })
    expect(views[2]).toMatchObject({ partId: 'text-1', metadata: { run_id: 'run-1', display_phase: 'final' } })
    expect(views[3]).toMatchObject({ partId: 'tool-1', toolName: 'read', status: 'success' })
    expect(views[8]).toMatchObject({
      partId: 'image-1',
      metadata: { tool_presentation: { call: 'image', result: 'interactive_media' } },
    })
    expect(views[5].ref).toEqual({ messageId: 'assistant-1', partId: 'plan-1', partIndex: 4, type: 'data-agent-proposed-plan' })
  })

  it('按 AI SDK resultProviderMetadata 覆盖工具结果展示契约', () => {
    const views = buildAgentMessageViews([{
      id: 'assistant-result-metadata',
      role: 'assistant',
      parts: [{
        type: 'dynamic-tool',
        toolName: 'renamed_media_generator',
        toolCallId: 'media-call',
        state: 'output-available',
        input: { purpose: 'interactive_image' },
        output: { image_path: 'assets/interactive/images/scene.png' },
        callProviderMetadata: { agent: { tool_presentation: { call: 'image', result: 'image' } } },
        resultProviderMetadata: { agent: { tool_presentation: { call: 'image', result: 'interactive_media' } } },
      }],
    }] as AgentUIMessage[])

    expect(views).toHaveLength(1)
    expect(views[0].metadata.tool_presentation).toEqual({ call: 'image', result: 'interactive_media' })
    expect(agentViewToRenderMessage(views[0])).toMatchObject({
      role: 'tool_call',
      name: 'renamed_media_generator',
      tool_presentation: { call: 'image', result: 'interactive_media' },
    })
  })

  it('专用互动图片 data part 覆盖同一工具的标准 result card', () => {
    const views = buildAgentMessageViews([{
      id: 'assistant-live-media',
      role: 'assistant',
      metadata: { run_id: 'run-media' },
      parts: [
        {
          type: 'dynamic-tool',
          toolName: 'renamed_media_generator',
          toolCallId: 'media-call',
          state: 'output-available',
          input: { purpose: 'interactive_image' },
          output: { schema: 'interactive_image.v1', image_path: 'assets/interactive/images/scene.png' },
          resultProviderMetadata: { agent: { run_id: 'run-media', tool_presentation: { call: 'image', result: 'interactive_media' } } },
        },
        {
          type: 'data-agent-interactive-image',
          id: 'media-call',
          providerMetadata: { agent: { run_id: 'run-media', tool_presentation: { call: 'image', result: 'interactive_media' } } },
          data: {
            name: 'renamed_media_generator',
            status: 'success',
            interactive_image: { schema: 'interactive_image.v1', image_path: 'assets/interactive/images/scene.png' },
          },
        },
      ],
    }] as AgentUIMessage[])

    expect(views).toHaveLength(1)
    expect(views[0]).toMatchObject({ kind: 'interactive-image', partId: 'media-call' })
  })

  it('todo 在同一 run 内替换旧计划，空计划会清除当前卡片', () => {
    const plan = (id: string, step: string, outputPlan: Array<{ step: string; status: string }>) => ({
      id,
      role: 'assistant' as const,
      metadata: { run_id: 'run-todo', tool_presentation: { call: 'todo', result: 'todo' } },
      parts: [{
        type: 'dynamic-tool', toolName: 'todo', toolCallId: id, state: 'output-available',
        input: { action: 'update', expected_revision: 0, mutations: [{ id, text: step, status: 'pending' }] },
        output: { schema: 'agent.todo.v1', revision: 1, items: outputPlan.map((item, index) => ({ id: `${id}-${index}`, text: item.step, status: item.status })) },
      }],
    })

    const replaced = buildAgentMessageViews([
      plan('todo-1', '旧计划', [{ step: '旧计划', status: 'pending' }]),
      plan('todo-2', '新计划', [{ step: '新计划', status: 'in_progress' }]),
    ] as AgentUIMessage[])
    expect(replaced.filter((view) => view.toolName === 'todo')).toHaveLength(1)
    expect(replaced.find((view) => view.toolName === 'todo')?.partId).toBe('todo-2')

    const cleared = buildAgentMessageViews([
      plan('todo-1', '旧计划', [{ step: '旧计划', status: 'pending' }]),
      plan('todo-clear', '', []),
    ] as AgentUIMessage[])
    expect(cleared.some((view) => view.toolName === 'todo')).toBe(false)
  })

  it('todo 计划按 root/subagent run 隔离替换状态', () => {
    const messages = [
      {
        id: 'root-plan', role: 'assistant', metadata: { run_id: 'run-1', run_path: ['root'], tool_presentation: { call: 'todo', result: 'todo' } },
        parts: [{ type: 'dynamic-tool', toolName: 'todo', toolCallId: 'root-todo', state: 'output-available', input: { action: 'update', mutations: [] }, output: { schema: 'agent.todo.v1', revision: 1, items: [{ id: 'root', text: 'root', status: 'pending' }] } }],
      },
      {
        id: 'child-plan', role: 'assistant', metadata: { run_id: 'run-1', run_path: ['root', 'child'], subagent: true, tool_presentation: { call: 'todo', result: 'todo' } },
        parts: [{ type: 'dynamic-tool', toolName: 'todo', toolCallId: 'child-todo', state: 'output-available', input: { action: 'update', mutations: [] }, output: { schema: 'agent.todo.v1', revision: 1, items: [{ id: 'child', text: 'child', status: 'pending' }] } }],
      },
    ] as AgentUIMessage[]
    expect(buildAgentMessageViews(messages).filter((view) => view.toolName === 'todo')).toHaveLength(2)
  })

  it('todo 失败调用保留诊断且不覆盖同一 run 的已提交计划', () => {
    const views = buildAgentMessageViews([
      {
        id: 'todo-committed',
        role: 'assistant',
        metadata: { run_id: 'run-todo-error' },
        parts: [{
          type: 'dynamic-tool',
          toolName: 'todo',
          toolCallId: 'todo-committed',
          state: 'output-available',
          input: { action: 'update', mutations: [{ id: 'keep', text: '保留计划', status: 'in_progress' }] },
          output: { schema: 'agent.todo.v1', revision: 1, items: [{ id: 'keep', text: '保留计划', status: 'in_progress' }] },
        }],
      },
      {
        id: 'todo-rejected',
        role: 'assistant',
        metadata: { run_id: 'run-todo-error' },
        parts: [{
          type: 'dynamic-tool',
          toolName: 'todo',
          toolCallId: 'todo-rejected',
          state: 'output-error',
          input: { action: 'update', mutations: [{ id: 'rejected', text: '不应覆盖', status: 'pending' }] },
          errorText: 'todo plan must contain at most one in_progress item',
        }],
      },
    ] as AgentUIMessage[])

    expect(views.filter((view) => view.toolName === 'todo')).toEqual(expect.arrayContaining([
      expect.objectContaining({ partId: 'todo-committed', status: 'success' }),
      expect.objectContaining({ partId: 'todo-rejected', status: 'error' }),
    ]))
  })

  it('将工具审批关联到原工具 view，不生成独立时间线卡片', () => {
    const views = buildAgentMessageViews([
    {
      id: 'tool-message', role: 'assistant', metadata: { run_id: 'run-approval' },
      parts: [{
      type: 'dynamic-tool', toolName: 'bash', toolCallId: 'tool-execution-1',
      state: 'input-available', input: { command: 'go test ./...' },
      }],
    },
    {
      id: 'approval-message', role: 'assistant', metadata: { run_id: 'run-approval' },
      parts: [{
      type: 'data-agent-ask', id: 'approval-1', data: {
        schema: 'ask.pending.v1', id: 'approval-1', kind: 'tool_approval',
        tool_call_id: 'tool-execution-1', agent_kind: 'ide', status: 'pending',
        questions: [{ id: 'tool-approval', question: 'Approve?', options: [{ id: 'allow-once', label: 'Allow once' }, { id: 'deny', label: 'Deny' }] }],
        approval: { mode: 'ask', tool_name: 'bash', command: 'go test ./...', risk: 'high', rule_id: 'bash_unlisted_command', args_hash: 'a'.repeat(64) },
      },
      }],
    },
    ] as AgentUIMessage[])

    expect(views).toHaveLength(1)
    expect(views[0]).toMatchObject({ kind: 'tool', partId: 'tool-execution-1', approval: { id: 'approval-1' } })
    expect(agentViewToRenderMessage(views[0])).toMatchObject({ role: 'tool_call', ask: { id: 'approval-1' } })
  })

  it('将找不到工具调用的审批保留在 trace 中作为容错诊断', () => {
    const views = buildAgentMessageViews([{
    id: 'orphan-approval', role: 'assistant', parts: [{
      type: 'data-agent-ask', id: 'orphan-approval', data: {
      schema: 'ask.pending.v1', id: 'orphan-approval', kind: 'tool_approval',
      tool_call_id: 'missing-tool', agent_kind: 'ide', status: 'cancelled', questions: [],
      approval: { mode: 'ask', tool_name: 'bash', risk: 'high', rule_id: 'bash_unlisted_command', args_hash: 'a'.repeat(64) },
      },
    }],
    }] as AgentUIMessage[])

    expect(views).toHaveLength(1)
    expect(isAgentTraceView(views[0])).toBe(true)
  })

  it('提取 token usage records 供输入区统计使用', () => {
    const records = selectAgentTokenUsageRecords([
      {
        id: 'assistant-1',
        role: 'assistant',
        metadata: { run_id: 'run-1', agent_kind: 'chat' },
        parts: [{ type: 'data-agent-token-usage', id: 'usage-1', data: { model_calls: 2, total_tokens: 88 } }],
      },
    ] as AgentUIMessage[])

    expect(records).toEqual([
      expect.objectContaining({ id: 'usage-1', role: 'token_usage', run_id: 'run-1', agent_kind: 'chat', model_calls: 2, total_tokens: 88 }),
    ])
  })

  it('只在 Agent 已产生有效结果且不再流式输出时标记完成回合', () => {
    const messages = [{
      id: 'assistant-1',
      role: 'assistant',
      parts: [{ type: 'text', text: '已经完成的正文', state: 'done' }],
    }] as AgentUIMessage[]

    expect(countCompletedAgentTurnSignals(messages)).toBe(1)
    expect(hasCompletedAgentTurn(messages, true)).toBe(false)
    expect(hasCompletedAgentTurn(messages, false)).toBe(true)
    expect(hasCompletedAgentTurn([], false)).toBe(false)
  })

  it('忽略空内容的 system 和未知 activity data part', () => {
    const views = buildAgentMessageViews([
      {
        id: 'assistant-empty',
        role: 'assistant',
        parts: [
          { type: 'data-agent-system', id: 'system-empty', data: {} },
          { type: 'data-agent-activity', id: 'activity-empty', data: { status: 'running' } },
          { type: 'data-agent-activity', id: 'cycle-started', data: { event: 'agent_cycle_started', message: '继续下一章' } },
          { type: 'data-agent-activity', id: 'activity-visible', data: { content: '正在整理' } },
        ],
      },
    ] as AgentUIMessage[])

    expect(views).toHaveLength(1)
    expect(views[0]).toMatchObject({ kind: 'activity', partId: 'activity-visible', content: '正在整理' })
  })
})
