import { describe, expect, it, vi } from 'vitest'
import {
  AgentChatTransport,
  agentUIMessagesToChatMessages,
  buildAgentChatRequestBody,
  type AgentUIMessage,
} from './agent-ui'

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
    })
  })

  it('将 AgentUIMessage parts 转为现有 ChatMessage 展示模型', () => {
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

    const converted = agentUIMessagesToChatMessages(messages)
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
    expect(converted[0]).toMatchObject({ id: 'user-1', content: '写下一章' })
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
