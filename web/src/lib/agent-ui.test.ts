import { describe, expect, it, vi } from 'vitest'
import {
  AgentChatTransport,
  buildAgentChatRequestBody,
  normalizeAgentUIMessages,
  type AgentUIMessage,
} from './agent-ui'
import { agentViewToRenderMessage, buildAgentMessageViews } from './agent-message-view'

describe('agent-ui', () => {
  it('保留单轮请求 extras，不回传完整 UI 历史', () => {
    expect(buildAgentChatRequestBody({
      references: ['chapters/a.md'],
      lore_references: ['lore-1'],
      style_scenes: ['battle'],
      selections: [{ file_name: 'a.md', start_line: 1, end_line: 2, content: 'text' }],
      ide_context: { current_file: 'a.md', open_files: ['a.md'] },
      plan_mode: true,
      writing_skill: 'draft',
      image_preset_id: 'preset-1',
      teller_id: 'teller-1',
      review_feedback: { review_thread_id: 'review-1', comment_ids: ['comment-1', 'comment-1', 'comment-2'] },
    })).toEqual({
      references: ['chapters/a.md'],
      lore_references: ['lore-1'],
      style_scenes: ['battle'],
      selections: [{ file_name: 'a.md', start_line: 1, end_line: 2, content: 'text' }],
      ide_context: { current_file: 'a.md', open_files: ['a.md'] },
      plan_mode: true,
      writing_skill: 'draft',
      image_preset_id: 'preset-1',
      teller_id: 'teller-1',
      review_feedback: { review_thread_id: 'review-1', comment_ids: ['comment-1', 'comment-2'] },
    })
  })

  it('保留正文审阅来源并去重评论 ID', () => {
    expect(buildAgentChatRequestBody({
      review_feedback: {
        source: 'document',
        review_thread_id: 'document-review-1',
        comment_ids: ['comment-1', 'comment-1', 'comment-2'],
      },
    }).review_feedback).toEqual({
      source: 'document',
      review_thread_id: 'document-review-1',
      comment_ids: ['comment-1', 'comment-2'],
    })
  })

  it('通过唯一 view 模块将 AgentUIMessage parts 转为展示模型', () => {
    const messages: AgentUIMessage[] = [
      {
        id: 'hidden-user',
        role: 'user',
        metadata: { display_hidden: true },
        parts: [{ type: 'text', text: 'protocol only' }],
      },
      {
        id: 'user-1',
        role: 'user',
        parts: [{ type: 'text', text: '写下一章' }],
      },
      {
        id: 'assistant-1',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          { type: 'reasoning', text: '先分析', state: 'streaming' },
          { type: 'text', text: '正文', state: 'done' },
          { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-1', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
          { type: 'data-agent-plan-question', id: 'question-1', data: { content: '选择方向', status: 'running' } },
          { type: 'data-agent-token-usage', id: 'usage-1', data: { total_tokens: 42, usage_calls: [{ index: 0, total_tokens: 42 }] } },
          { type: 'data-agent-rule-roll', id: 'roll-1', data: { rule_roll: { label: '检定', total: 18 } } },
          {
            type: 'data-agent-interactive-image',
            id: 'image-1',
            data: {
              name: 'generate_interactive_image',
              status: 'success',
              interactive_image: {
                schema: 'interactive_image.v1',
                story_id: 'story-1',
                branch_id: 'branch-1',
                turn_id: 'turn-1',
                image_path: 'assets/interactive/images/scene.png',
                meta_path: 'assets/interactive/images/scene.json',
              },
            },
          },
        ],
      },
    ] as AgentUIMessage[]

    const converted = buildAgentMessageViews(messages)
      .map(view => agentViewToRenderMessage(view))
      .filter((message): message is NonNullable<typeof message> => Boolean(message))
    expect(converted.map(message => message.role)).toEqual([
      'user',
      'thinking',
      'assistant',
      'tool_call',
      'plan_question',
      'token_usage',
      'rule_roll',
      'tool_result',
    ])
    expect(converted[0]).toMatchObject({ id: 'user-1:0', content: '写下一章' })
    expect(converted[1]).toMatchObject({ content: '先分析', streaming: true, run_id: 'run-1' })
    expect(converted[3]).toMatchObject({ id: 'tool-1', name: 'read_file', status: 'success', result: 'ok' })
    expect(converted[4]).toMatchObject({ id: 'question-1', status: 'running', streaming: true })
    expect(converted[5]).toMatchObject({ id: 'usage-1', total_tokens: 42, usage_calls: [{ index: 0, total_tokens: 42 }] })
    expect(converted[6].rule_roll).toMatchObject({ label: '检定', total: 18 })
    expect(converted[7]).toMatchObject({
      id: 'image-1',
      name: 'generate_interactive_image',
      interactive_image_status: 'success',
      interactive_image: { image_path: 'assets/interactive/images/scene.png' },
    })
  })

  it('AgentChatTransport 只发送本轮 body 并解析 UI message stream', async () => {
    let requestBody: Record<string, unknown> | undefined
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_input, init) => {
      requestBody = JSON.parse(String(init?.body || '{}')) as Record<string, unknown>
      return new Response(
        'data: {"type":"start","messageId":"assistant-1"}\n\n' +
        'data: {"type":"text-start","id":"text-1"}\n\n' +
        'data: {"type":"text-delta","id":"text-1","delta":"你好"}\n\n' +
        'data: {"type":"text-end","id":"text-1"}\n\n' +
        'data: {"type":"finish","finishReason":"stop"}\n\n' +
        'data: [DONE]\n\n',
        { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
      )
    })

    try {
      const transport = new AgentChatTransport()
      const stream = await transport.sendMessages({
        trigger: 'submit-message',
        chatId: 'chat-1',
        messageId: undefined,
        abortSignal: undefined,
        messages: [
          { id: 'user-1', role: 'user', parts: [{ type: 'text', text: '最新输入' }] },
        ] as AgentUIMessage[],
        body: {
          references: ['chapters/a.md'],
          plan_mode: true,
        },
      })
      const chunks = await readStream(stream)

      expect(requestBody).toEqual({
        references: ['chapters/a.md'],
        plan_mode: true,
        message: '最新输入',
      })
      expect(requestBody).not.toHaveProperty('messages')
      expect(chunks.map(chunk => chunk.type)).toEqual(['start', 'text-start', 'text-delta', 'text-end', 'finish'])
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('恢复活跃流时按 part 稳定身份合并历史和 replay，避免卡片在底部重复', () => {
    const messages = normalizeAgentUIMessages([
      {
        id: 'history-tool',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-1', state: 'output-available', input: { path: 'a.md' }, output: 'persisted' },
        ],
      },
      {
        id: 'history-thinking',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [{ type: 'reasoning', text: '先分析' }],
      },
      {
        id: 'history-usage',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [{ type: 'data-agent-token-usage', id: 'run-1', data: { run_id: 'run-1', total_tokens: 10 } }],
      },
      {
        id: 'replay-assistant',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          { type: 'reasoning', id: 'reasoning-1', text: '先分析', providerMetadata: { agent: { run_id: 'run-1' } } },
          { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-1', state: 'input-streaming', input: { path: 'a.md' } },
          { type: 'data-agent-token-usage', id: 'run-1', data: { run_id: 'run-1', total_tokens: 20 } },
          { type: 'text', id: 'text-1', text: '继续生成', providerMetadata: { agent: { run_id: 'run-1' } } },
        ],
      },
    ] as AgentUIMessage[])

    expect(messages).toHaveLength(4)
    expect(messages[0].parts).toEqual([
      expect.objectContaining({ type: 'dynamic-tool', toolCallId: 'tool-1', state: 'output-available', output: 'persisted' }),
    ])
    expect(messages[1].parts).toEqual([
      expect.objectContaining({ type: 'reasoning', text: '先分析' }),
    ])
    expect(messages[2].parts).toEqual([
      expect.objectContaining({ type: 'data-agent-token-usage', data: expect.objectContaining({ total_tokens: 20 }) }),
    ])
    expect(messages[3].parts).toEqual([
      expect.objectContaining({ type: 'text', text: '继续生成' }),
    ])
  })

  it('恢复流 replay 到完成态时用最新 tool part 更新历史卡片', () => {
    const messages = normalizeAgentUIMessages([
      {
        id: 'history-tool',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-1', state: 'input-available', input: { path: 'a.md' } },
        ],
      },
      {
        id: 'replay-tool',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-1', state: 'output-available', input: { path: 'a.md' }, output: 'fresh' },
        ],
      },
    ] as AgentUIMessage[])

    expect(messages).toHaveLength(1)
    expect(messages[0].parts).toEqual([
      expect.objectContaining({ type: 'dynamic-tool', toolCallId: 'tool-1', state: 'output-available', output: 'fresh' }),
    ])
  })

  it('恢复流中的同一段 reasoning 继续增长时仍更新历史 part 而不是追加新卡片', () => {
    const base = '这是一段已经持久化的思考内容，用来模拟刷新前已经落入历史的推理文本。'
    const messages = normalizeAgentUIMessages([
      {
        id: 'history-thinking',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [{ type: 'reasoning', text: base }],
      },
      {
        id: 'replay-thinking',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [{ type: 'reasoning', id: 'reasoning-1', text: `${base}继续补充。` }],
      },
    ] as AgentUIMessage[])

    expect(messages).toHaveLength(1)
    expect(messages[0].parts).toEqual([
      expect.objectContaining({ type: 'reasoning', text: `${base}继续补充。` }),
    ])
  })
})

async function readStream<T>(stream: ReadableStream<T>): Promise<T[]> {
  const reader = stream.getReader()
  const chunks: T[] = []
  while (true) {
    const { done, value } = await reader.read()
    if (done) return chunks
    chunks.push(value)
  }
}
