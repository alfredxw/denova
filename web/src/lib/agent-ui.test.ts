import { describe, expect, it, vi } from 'vitest'
import {
  AgentChatTransport,
  buildAgentChatRequestBody,
  initialSubmissionOutcomeForStatus,
  normalizeAgentUIMessages,
  type AgentUIMessage,
} from './agent-ui'
import { agentViewToRenderMessage, buildAgentMessageViews } from './agent-message-view'

describe('agent-ui', () => {
  it('classifies initial start acceptance without treating 5xx as a definite rejection', () => {
    expect(initialSubmissionOutcomeForStatus(200)).toBe('accepted')
    expect(initialSubmissionOutcomeForStatus(409)).toBe('rejected')
    expect(initialSubmissionOutcomeForStatus(500)).toBe('uncertain')
  })

  it('preserves the caller-owned initial command id in the request body', () => {
    expect(buildAgentChatRequestBody({ command_id: '  initial-command  ' })).toMatchObject({
      command_id: 'initial-command',
    })
  })

  it('没有重复 part 时保留消息和 parts 引用，避免流式更新重渲染历史消息', () => {
    const historyPart = {
      type: 'text',
      id: 'history-text',
      text: '已经渲染的历史正文',
    } as const
    const streamingPart = {
      type: 'reasoning',
      id: 'active-reasoning',
      text: '正在继续分析',
      state: 'streaming',
    } as const
    const historyMessage = {
      id: 'history-assistant',
      role: 'assistant',
      parts: [historyPart],
    } as AgentUIMessage
    const streamingMessage = {
      id: 'active-assistant',
      role: 'assistant',
      parts: [streamingPart],
    } as AgentUIMessage

    const normalized = normalizeAgentUIMessages([historyMessage, streamingMessage])

    expect(normalized[0]).toBe(historyMessage)
    expect(normalized[0].parts).toBe(historyMessage.parts)
    expect(normalized[0].parts[0]).toBe(historyPart)
    expect(normalized[1]).toBe(streamingMessage)
    expect(normalized[1].parts).toBe(streamingMessage.parts)
    expect(normalized[1].parts[0]).toBe(streamingPart)
  })

  it('不重复读取未变化历史消息的正文来生成去重指纹', () => {
    let textReads = 0
    const part = { type: 'text' } as Record<string, unknown>
    Object.defineProperty(part, 'text', {
      enumerable: true,
      get: () => {
        textReads += 1
        return '稳定的历史正文'
      },
    })
    const message = {
      id: 'history-assistant',
      role: 'assistant',
      metadata: { run_id: 'run-history' },
      parts: [part],
    } as unknown as AgentUIMessage

    normalizeAgentUIMessages([message])
    const readsAfterFirstNormalization = textReads
    normalizeAgentUIMessages([message])

    expect(textReads).toBe(readsAfterFirstNormalization)
  })

  it('保留单轮请求 extras，不回传完整 UI 历史', () => {
    expect(
      buildAgentChatRequestBody({
        references: ['chapters/a.md'],
        lore_references: ['lore-1'],
        style_scenes: ['battle'],
        selections: [{ file_name: 'a.md', start_line: 1, end_line: 2, content: 'text' }],
        ide_context: { current_file: 'a.md', open_files: ['a.md'] },
        plan_mode: true,
        writing_skill: 'draft',
        image_preset_id: 'preset-1',
        teller_id: 'teller-1',
        review_feedback: [
          {
            review_thread_id: 'review-1',
            comment_ids: ['comment-1', 'comment-1', 'comment-2'],
          },
        ],
      }),
    ).toEqual({
      references: ['chapters/a.md'],
      lore_references: ['lore-1'],
      style_scenes: ['battle'],
      selections: [{ file_name: 'a.md', start_line: 1, end_line: 2, content: 'text' }],
      ide_context: { current_file: 'a.md', open_files: ['a.md'] },
      plan_mode: true,
      writing_skill: 'draft',
      image_preset_id: 'preset-1',
      teller_id: 'teller-1',
      review_feedback: [
        {
          review_thread_id: 'review-1',
          comment_ids: ['comment-1', 'comment-2'],
        },
      ],
    })
  })

  it('保留正文审阅来源并去重评论 ID', () => {
    expect(
      buildAgentChatRequestBody({
        review_feedback: [
          {
            source: 'document',
            review_thread_id: 'document-review-1',
            comment_ids: ['comment-1', 'comment-1', 'comment-2'],
          },
        ],
      }).review_feedback,
    ).toEqual([
      {
        source: 'document',
        review_thread_id: 'document-review-1',
        comment_ids: ['comment-1', 'comment-2'],
      },
    ])
  })

  it('同时保留正文与 Diff 审阅来源', () => {
    expect(
      buildAgentChatRequestBody({
        review_feedback: [
          { review_thread_id: 'diff-review-1', comment_ids: ['diff-comment'] },
          {
            source: 'document',
            review_thread_id: 'document-review-1',
            comment_ids: ['document-comment'],
          },
        ],
      }).review_feedback,
    ).toEqual([
      { review_thread_id: 'diff-review-1', comment_ids: ['diff-comment'] },
      {
        source: 'document',
        review_thread_id: 'document-review-1',
        comment_ids: ['document-comment'],
      },
    ])
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
          {
            type: 'dynamic-tool',
            toolName: 'read_file',
            toolCallId: 'tool-1',
            state: 'output-available',
            input: { path: 'a.md' },
            output: 'ok',
          },
          {
            type: 'data-agent-plan-question',
            id: 'question-1',
            data: { content: '选择方向', status: 'running' },
          },
          {
            type: 'data-agent-token-usage',
            id: 'usage-1',
            data: {
              total_tokens: 42,
              usage_calls: [{ index: 0, total_tokens: 42 }],
            },
          },
          {
            type: 'data-agent-rule-roll',
            id: 'roll-1',
            data: { rule_roll: { label: '检定', total: 18 } },
          },
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
      .map((view) => agentViewToRenderMessage(view))
      .filter((message): message is NonNullable<typeof message> => Boolean(message))
    expect(converted.map((message) => message.role)).toEqual([
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
    expect(converted[1]).toMatchObject({
      content: '先分析',
      streaming: true,
      run_id: 'run-1',
    })
    expect(converted[3]).toMatchObject({
      id: 'tool-1',
      name: 'read_file',
      status: 'success',
      result: 'ok',
    })
    expect(converted[4]).toMatchObject({
      id: 'question-1',
      status: 'running',
      streaming: true,
    })
    expect(converted[5]).toMatchObject({
      id: 'usage-1',
      total_tokens: 42,
      usage_calls: [{ index: 0, total_tokens: 42 }],
    })
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
          {
            id: 'user-1',
            role: 'user',
            parts: [{ type: 'text', text: '最新输入' }],
          },
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
      expect(chunks.map((chunk) => chunk.type)).toEqual(['start', 'text-start', 'text-delta', 'text-end', 'finish'])
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('只绑定 exact task 并从 0 完整 replay，不把 SSE id 或 runtime cursor 当作 after', async () => {
    const requests: string[] = []
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      requests.push(`${init?.method || 'GET'} ${String(input)}`)
      if (init?.method === 'GET') return new Response(null, { status: 204 })
      return fragmentedEventStreamResponse([
        'id: 4\r\nda',
        'ta: {"type":"start","messageId":"assistant-1"}\r\n\r\n',
        'id: 9\ndata: {"type":"finish","finishReason":"stop"}\n\n',
        'data: [DONE]\n\n',
      ])
    })

    try {
      const transport = new AgentChatTransport()
      const stream = await transport.sendMessages(agentSendOptions())
      await readStream(stream)
      transport.setActiveStreamTarget('task/exact 1')

      await expect(transport.reconnectToStream({ chatId: 'chat-1' })).resolves.toBeNull()

      expect(requests.at(-1)).toBe('GET /api/chat/stream?task_id=task%2Fexact+1')
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('新发送会清空旧 task 绑定，必须等 /active 返回新的 exact task', async () => {
    const requests: string[] = []
    let sendCount = 0
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      requests.push(`${init?.method || 'GET'} ${String(input)}`)
      if (init?.method === 'GET') return new Response(null, { status: 204 })
      sendCount += 1
      return sendCount === 1
        ? fragmentedEventStreamResponse(['id: 12\ndata: {"type":"start","messageId":"assistant-1"}\n\n', 'data: [DONE]\n\n'])
        : fragmentedEventStreamResponse(['data: [DONE]\n\n'])
    })

    try {
      const transport = new AgentChatTransport()
      await readStream(await transport.sendMessages(agentSendOptions()))
      transport.setActiveStreamTarget('task-old')
      await readStream(await transport.sendMessages(agentSendOptions('chat-2')))

      await expect(transport.reconnectToStream({ chatId: 'chat-2' })).rejects.toThrow('exact Agent stream task')

      transport.setActiveStreamTarget('task-new')

      await transport.reconnectToStream({ chatId: 'chat-2' })

      expect(requests.at(-1)).toBe('GET /api/chat/stream?task_id=task-new')
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('切换 active task 时重连 URL 只包含新的 exact task 身份', async () => {
    const requests: string[] = []
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      requests.push(`${init?.method || 'GET'} ${String(input)}`)
      if (init?.method === 'GET') return new Response(null, { status: 204 })
      return fragmentedEventStreamResponse(['id: 7\ndata: {"type":"start","messageId":"assistant-1"}\n\n', 'data: [DONE]\n\n'])
    })

    try {
      const transport = new AgentChatTransport()
      await readStream(await transport.sendMessages(agentSendOptions()))
      transport.setActiveStreamTarget('task-one')
      transport.setActiveStreamTarget('task-two')

      await transport.reconnectToStream({ chatId: 'chat-1' })

      expect(requests.at(-1)).toBe('GET /api/chat/stream?task_id=task-two')
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('只在规范历史恢复后的首个连接携带服务端检查点游标', async () => {
    const requests: string[] = []
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      requests.push(`${init?.method || 'GET'} ${String(input)}`)
      return new Response(null, { status: 204 })
    })

    try {
      const transport = new AgentChatTransport()
      transport.setActiveStreamTarget('task-rehydrate', 41)

      await transport.reconnectToStream({ chatId: 'chat-1' })
      await transport.reconnectToStream({ chatId: 'chat-1' })

      expect(requests).toEqual(['GET /api/chat/stream?task_id=task-rehydrate&after=41', 'GET /api/chat/stream?task_id=task-rehydrate'])
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
          {
            type: 'dynamic-tool',
            toolName: 'read_file',
            toolCallId: 'tool-1',
            state: 'output-available',
            input: { path: 'a.md' },
            output: 'persisted',
          },
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
        parts: [
          {
            type: 'data-agent-token-usage',
            id: 'run-1',
            data: { run_id: 'run-1', total_tokens: 10 },
          },
        ],
      },
      {
        id: 'replay-assistant',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          {
            type: 'reasoning',
            text: '先分析',
            providerMetadata: { agent: { run_id: 'run-1', display_segment_id: 'reasoning-1' } },
          },
          {
            type: 'dynamic-tool',
            toolName: 'read_file',
            toolCallId: 'tool-1',
            state: 'input-streaming',
            input: { path: 'a.md' },
          },
          {
            type: 'data-agent-token-usage',
            id: 'run-1',
            data: { run_id: 'run-1', total_tokens: 20 },
          },
          {
            type: 'text',
            text: '继续生成',
            providerMetadata: { agent: { run_id: 'run-1', display_segment_id: 'text-1' } },
          },
        ],
      },
    ] as AgentUIMessage[])

    expect(messages).toHaveLength(4)
    expect(messages[0].parts).toEqual([
      expect.objectContaining({
        type: 'dynamic-tool',
        toolCallId: 'tool-1',
        state: 'output-available',
        output: 'persisted',
      }),
    ])
    expect(messages[1].parts).toEqual([expect.objectContaining({ type: 'reasoning', text: '先分析' })])
    expect(messages[2].parts).toEqual([
      expect.objectContaining({
        type: 'data-agent-token-usage',
        data: expect.objectContaining({ total_tokens: 20 }),
      }),
    ])
    expect(messages[3].parts).toEqual([expect.objectContaining({ type: 'text', text: '继续生成' })])
  })

  it('恢复流 replay 到完成态时用最新 tool part 更新历史卡片', () => {
    const messages = normalizeAgentUIMessages([
      {
        id: 'history-tool',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          {
            type: 'dynamic-tool',
            toolName: 'read_file',
            toolCallId: 'tool-1',
            state: 'input-available',
            input: { path: 'a.md' },
          },
        ],
      },
      {
        id: 'replay-tool',
        role: 'assistant',
        metadata: { run_id: 'run-1' },
        parts: [
          {
            type: 'dynamic-tool',
            toolName: 'read_file',
            toolCallId: 'tool-1',
            state: 'output-available',
            input: { path: 'a.md' },
            output: 'fresh',
          },
        ],
      },
    ] as AgentUIMessage[])

    expect(messages).toHaveLength(1)
    expect(messages[0].parts).toEqual([
      expect.objectContaining({
        type: 'dynamic-tool',
        toolCallId: 'tool-1',
        state: 'output-available',
        output: 'fresh',
      }),
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
        parts: [
          {
            type: 'reasoning',
            text: `${base}继续补充。`,
            providerMetadata: { agent: { run_id: 'run-1', display_segment_id: 'reasoning-1' } },
          },
        ],
      },
    ] as AgentUIMessage[])

    expect(messages).toHaveLength(1)
    expect(messages[0].parts).toEqual([expect.objectContaining({ type: 'reasoning', text: `${base}继续补充。` })])
  })

  it('同一 run 中内容相同但稳定 ID 不同的正文和 reasoning 保持为独立分段', () => {
    const messages = normalizeAgentUIMessages([
      {
        id: 'thinking-1',
        role: 'assistant',
        metadata: { run_id: 'run-1', display_segment_id: 'segment-thinking-1' },
        parts: [{ type: 'reasoning', text: '再次检查。' }],
      },
      {
        id: 'text-1',
        role: 'assistant',
        metadata: { run_id: 'run-1', display_segment_id: 'segment-text-1' },
        parts: [{ type: 'text', text: '继续。' }],
      },
      {
        id: 'thinking-2',
        role: 'assistant',
        metadata: { run_id: 'run-1', display_segment_id: 'segment-thinking-2' },
        parts: [{ type: 'reasoning', text: '再次检查。' }],
      },
      {
        id: 'text-2',
        role: 'assistant',
        metadata: { run_id: 'run-1', display_segment_id: 'segment-text-2' },
        parts: [{ type: 'text', text: '继续。' }],
      },
    ] as AgentUIMessage[])

    expect(messages.map((message) => message.metadata?.display_segment_id)).toEqual([
      'segment-thinking-1',
      'segment-text-1',
      'segment-thinking-2',
      'segment-text-2',
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

function agentSendOptions(chatId = 'chat-1') {
  return {
    trigger: 'submit-message' as const,
    chatId,
    messageId: undefined,
    abortSignal: undefined,
    messages: [
      {
        id: 'user-1',
        role: 'user' as const,
        parts: [{ type: 'text' as const, text: '继续' }],
      },
    ] as AgentUIMessage[],
  }
}

function fragmentedEventStreamResponse(parts: string[]) {
  const encoder = new TextEncoder()
  return new Response(
    new ReadableStream<Uint8Array>({
      start(controller) {
        for (const part of parts) controller.enqueue(encoder.encode(part))
        controller.close()
      },
    }),
    { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
  )
}
