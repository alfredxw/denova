import { describe, expect, it } from 'vitest'
import type { AgentUIMessage } from './agent-ui'
import { buildAgentMessageViews, selectAgentTokenUsageRecords } from './agent-message-view'

describe('agent-message-view', () => {
  it('从 AgentUIMessage parts 生成稳定渲染 view', () => {
    const messages: AgentUIMessage[] = [
      { id: 'hidden-user', role: 'user', metadata: { display_hidden: true }, parts: [{ type: 'text', text: 'hidden' }] },
      { id: 'user-1', role: 'user', metadata: { turn_id: 'turn-1' }, parts: [{ type: 'text', text: '写下一章' }] },
      {
        id: 'assistant-1',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          { type: 'reasoning', id: 'reason-1', text: '先分析', state: 'streaming' },
          { type: 'text', id: 'text-1', text: '正文', state: 'done' },
          { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-1', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
          { type: 'data-agent-context-compaction', id: 'compact-1', data: { content: '压缩上下文', status: 'running', tokens_before: 100 } },
          { type: 'data-agent-plan-question', id: 'question-1', data: { content: '选择方向', status: 'running' } },
          { type: 'data-agent-proposed-plan', id: 'plan-1', data: { content: '# Plan', status: 'success' } },
          { type: 'data-agent-token-usage', id: 'usage-1', data: { run_id: 'run-1', model_calls: 1, total_tokens: 42, usage_calls: [{ index: 1, total_tokens: 42 }] } },
          { type: 'data-agent-rule-roll', id: 'roll-1', data: { rule_roll: { label: '检定', total: 18 } } },
          {
            type: 'data-agent-interactive-image',
            id: 'image-1',
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
      'plan-question',
      'proposed-plan',
      'token-usage',
      'rule-roll',
      'interactive-image',
      'error',
      'clear',
    ])
    expect(views[0]).toMatchObject({ messageId: 'user-1', content: '写下一章', metadata: { turn_id: 'turn-1' } })
    expect(views[1]).toMatchObject({ partId: 'reason-1', streaming: true, metadata: { run_id: 'run-1' } })
    expect(views[3]).toMatchObject({ partId: 'tool-1', toolName: 'read_file', status: 'success' })
    expect(views[5].ref).toEqual({ messageId: 'assistant-1', partId: 'question-1', partIndex: 4, type: 'data-agent-plan-question' })
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

  it('忽略空内容的 system 和未知 activity data part', () => {
    const views = buildAgentMessageViews([
      {
        id: 'assistant-empty',
        role: 'assistant',
        parts: [
          { type: 'data-agent-system', id: 'system-empty', data: {} },
          { type: 'data-agent-activity', id: 'activity-empty', data: { status: 'running' } },
          { type: 'data-agent-activity', id: 'activity-visible', data: { content: '正在整理' } },
        ],
      },
    ] as AgentUIMessage[])

    expect(views).toHaveLength(1)
    expect(views[0]).toMatchObject({ kind: 'activity', partId: 'activity-visible', content: '正在整理' })
  })
})
