import { useEffect } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { SSEEvent } from '@/lib/api'
import { useAgentEventStream } from './useAgentEventStream'

describe('useAgentEventStream', () => {
  it('工具结果缺少 id 且同名 pending 唯一时按工具名回填', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['tool_call', { id: 'call-read', name: 'read_file', args: '{"path":"chapters/ch01.md"}' }],
        ['tool_call', { id: 'call-execute', name: 'execute', args: '{"command":"pwd"}' }],
        ['tool_result', { name: 'execute', content: 'command done' }],
      ]))
    })

    const messages = readMessages()
    const readFile = messages.find((message) => message.name === 'read_file')
    const execute = messages.find((message) => message.name === 'execute')
    expect(readFile?.result).toBeUndefined()
    expect(execute).toMatchObject({ status: 'success', result: 'command done', streaming: false })
  })

  it('工具结果 id 不匹配时不使用工具名误回填结果', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['tool_call', { id: 'call-execute', name: 'execute', args: '{"command":"pwd"}' }],
        ['tool_result', { id: 'stale-id', name: 'execute', content: 'stale result' }],
      ]))
    })

    expect(readMessages().find((message) => message.name === 'execute')?.result).toBeUndefined()
  })

  it('正常结束时将未收到 tool_result 的工具卡片收敛为成功态', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['tool_call', { id: 'call-execute', name: 'execute', args: '{"command":"pwd"}' }],
        ['done', { status: 'ok' }],
      ]))
    })

    expect(readMessages().find((message) => message.name === 'execute')).toMatchObject({
      status: 'success',
      streaming: false,
    })
  })

  it('done 事件到达但连接未关闭时不插入完成活动提示', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    let controller: ReadableStreamDefaultController<SSEEvent> | undefined
    const stream = new ReadableStream<SSEEvent>({
      start(nextController) {
        controller = nextController
      },
    })
    let consumePromise: Promise<void> | undefined
    let streamClosed = false

    try {
      await act(async () => {
        consumePromise = agent?.consumeAgentStream(stream)
        await Promise.resolve()
      })

      await act(async () => {
        controller?.enqueue(sseEvent('done', { status: 'ok' }))
        await Promise.resolve()
        await Promise.resolve()
      })

      expect(screen.getByTestId('activity').textContent).toBe('')
    } finally {
      if (!streamClosed) {
        await act(async () => {
          controller?.close()
          streamClosed = true
          await consumePromise
        })
      }
    }
  })

  it('保留 SSE 隐藏正文展示元信息并追加路径参数', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['tool_call', { id: 'call-write', name: 'write_file', args: '' }],
        ['tool_args_delta', {
          id: 'call-write',
          name: 'write_file',
          delta: '{"file_path":"chapters/ch01.md"}',
          sse_hidden_fields: ['content'],
          sse_hidden_reason: 'novel_chapter_body',
          sse_display_notice: 'chapter_body_hidden',
          sse_generated_chars: 3,
        }],
      ]))
    })

    await waitFor(() => expect(readMessages().find((message) => message.name === 'write_file')?.args).toContain('chapters/ch01.md'))
    expect(readMessages().find((message) => message.name === 'write_file')).toMatchObject({
      sse_hidden_fields: ['content'],
      sse_hidden_reason: 'novel_chapter_body',
      sse_display_notice: 'chapter_body_hidden',
      sse_generated_chars: 3,
    })
  })

  it('多个同名 pending 工具时不使用工具名误回填结果', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['tool_call', { id: 'execute-1', name: 'execute', args: '{"command":"pwd"}' }],
        ['tool_call', { id: 'execute-2', name: 'execute', args: '{"command":"ls"}' }],
        ['tool_result', { id: 'stale-id', name: 'execute', content: 'ambiguous result' }],
      ]))
    })

    const executeMessages = readMessages().filter((message) => message.name === 'execute')
    expect(executeMessages).toHaveLength(2)
    expect(executeMessages.some((message) => message.result === 'ambiguous result')).toBe(false)
  })

  it('章节插画工具结果保留结构化 illustration 数据', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['tool_call', { id: 'call-image', name: 'generate_image', args: '{"purpose":"chapter_illustration","target_path":"chapters/ch01.md"}' }],
        ['tool_result', {
          id: 'call-image',
          name: 'generate_image',
          content: '{"schema":"chapter_illustration.v1"}',
          illustration: {
            schema: 'chapter_illustration.v1',
            chapter_path: 'chapters/ch01.md',
            image_path: 'assets/illustrations/ch01/run/image.png',
            meta_path: 'assets/illustrations/ch01/run/meta.json',
            markdown: '![图](assets/illustrations/ch01/run/image.png)',
            alt_text: '图',
            profile_id: 'default',
            provider: 'openai',
            model: 'gpt-image-1',
          },
        }],
      ]))
    })

    const message = readMessages().find((item) => item.name === 'generate_image')
    expect(message).toMatchObject({
      status: 'success',
      illustration: {
        schema: 'chapter_illustration.v1',
        image_path: 'assets/illustrations/ch01/run/image.png',
      },
    })
  })

  it('subagent chunk 单独成段，不合并进 root assistant 输出', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['chunk', { content: 'root-a', agent_name: 'NovaAgent', root_agent_name: 'NovaAgent', run_path: ['NovaAgent'], subagent: false }],
        ['chunk', { content: 'sub-draft', agent_name: 'researcher', root_agent_name: 'NovaAgent', run_path: ['NovaAgent', 'researcher'], subagent: true }],
        ['chunk', { content: 'root-b', agent_name: 'NovaAgent', root_agent_name: 'NovaAgent', run_path: ['NovaAgent'], subagent: false }],
      ]))
    })

    const messages = readMessages().filter((message) => message.role === 'assistant')
    expect(messages).toHaveLength(3)
    expect(messages[0]).toMatchObject({ content: 'root-a', subagent: false })
    expect(messages[1]).toMatchObject({ content: 'sub-draft', subagent: true, agent_name: 'researcher' })
    expect(messages[2]).toMatchObject({ content: 'root-b', subagent: false })
  })

  it('同名 subagent 使用不同 subagent_session_id 时不合并输出', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['chunk', { content: 'first', agent_name: 'researcher', root_agent_name: 'NovaAgent', run_path: ['NovaAgent', 'researcher'], subagent: true, subagent_session_id: 'run-1-subagent-01-researcher' }],
        ['chunk', { content: 'second', agent_name: 'researcher', root_agent_name: 'NovaAgent', run_path: ['NovaAgent', 'researcher'], subagent: true, subagent_session_id: 'run-1-subagent-02-researcher' }],
      ]))
    })

    const messages = readMessages().filter((message) => message.role === 'assistant')
    expect(messages).toHaveLength(2)
    expect(messages[0]).toMatchObject({ content: 'first', subagent_session_id: 'run-1-subagent-01-researcher' })
    expect(messages[1]).toMatchObject({ content: 'second', subagent_session_id: 'run-1-subagent-02-researcher' })
  })

  it('每帧刷新全部已收到的 assistant 增量，不按固定字符数限速播放', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    const raf = installManualAnimationFrame()
    let controller: ReadableStreamDefaultController<SSEEvent> | undefined
    const stream = new ReadableStream<SSEEvent>({
      start(nextController) {
        controller = nextController
      },
    })
    let consumePromise: Promise<void> | undefined
    let streamClosed = false

    try {
      await act(async () => {
        consumePromise = agent?.consumeAgentStream(stream)
        await Promise.resolve()
      })

      const firstChunk = '首段'
      const fastChunk = '后续内容一次性到达，应在下一帧完整显示。'
      await act(async () => {
        controller?.enqueue(sseEvent('chunk', { content: firstChunk }))
        controller?.enqueue(sseEvent('chunk', { content: fastChunk }))
        await Promise.resolve()
        await Promise.resolve()
      })

      expect(readMessages().find((message) => message.role === 'assistant')).toMatchObject({
        content: '',
        streaming_target_content: firstChunk,
      })

      await act(async () => {
        raf.flush()
      })

      const stagedAssistant = readMessages().find((message) => message.role === 'assistant')
      expect(stagedAssistant?.content).toBe(firstChunk)
      expect(stagedAssistant?.streaming_target_content).toBe(firstChunk + fastChunk)

      await act(async () => {
        raf.flush()
      })

      const assistant = readMessages().find((message) => message.role === 'assistant')
      expect(assistant?.content).toBe(firstChunk + fastChunk)
      expect(assistant?.streaming_target_content).toBeUndefined()

      await act(async () => {
        controller?.close()
        streamClosed = true
        await consumePromise
      })
    } finally {
      if (!streamClosed) {
        await act(async () => {
          controller?.close()
          await consumePromise
        })
      }
      raf.restore()
    }
  })

  it('Plan 卡片 running 事件先占位，success 事件原地更新内容', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['plan_question', { id: 'plan_question-1', status: 'running' }],
        ['plan_question', { id: 'plan_question-1', status: 'success', content: '{"questions":[{"id":"scope","type":"single","question":"确认范围？","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}]}' }],
      ]))
    })

    const messages = readMessages().filter((message) => message.role === 'plan_question')
    expect(messages).toHaveLength(1)
    expect(messages[0]).toMatchObject({
      status: 'success',
      streaming: false,
      content: expect.stringContaining('"questions"'),
    })
  })

  it('Plan running 卡片不展示卡片出现前的 stale thinking', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['thinking', { content: '我先检查上下文\n准备输出问题卡', subagent: false }],
        ['plan_question', { id: 'plan_question-1', status: 'running' }],
      ]))
    })

    const message = readMessages().find((item) => item.role === 'plan_question')
    expect(message).toMatchObject({
      status: 'running',
      streaming: true,
    })
    expect(message?.thinking_preview).toBeUndefined()
  })

  it('Plan running 卡片展示 running 后变化的 root thinking 预览', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['plan_question', { id: 'plan_question-1', status: 'running' }],
        ['thinking', { content: '正在整理\n新的计划摘要', subagent: false }],
      ]))
    })

    const message = readMessages().find((item) => item.role === 'plan_question')
    expect(message).toMatchObject({
      status: 'running',
      streaming: true,
      thinking_preview: '新的计划摘要',
    })
  })

  it('SubAgent thinking 不会进入 Plan running 卡片预览', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['plan_question', { id: 'plan_question-1', status: 'running', subagent: false }],
        ['thinking', { content: '子任务内部分析', subagent: true, agent_name: 'researcher' }],
      ]))
    })

    const message = readMessages().find((item) => item.role === 'plan_question')
    expect(message?.thinking_preview).toBeUndefined()
  })

  it('不同 run 的同 raw id Plan 卡片不会互相覆盖', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['plan_question', { id: 'plan_question-1', status: 'success', run_id: 'run-1', content: '{"questions":[{"id":"first","type":"single","question":"第一轮？","options":[{"id":"a","label":"A"}]}]}' }],
        ['plan_question', { id: 'plan_question-1', status: 'running', run_id: 'run-2' }],
        ['plan_question', { id: 'plan_question-1', status: 'success', run_id: 'run-2', content: '{"questions":[{"id":"second","type":"single","question":"第二轮？","options":[{"id":"b","label":"B"}]}]}' }],
      ]))
    })

    const messages = readMessages().filter((message) => message.role === 'plan_question')
    expect(messages).toHaveLength(2)
    expect(messages[0]).toMatchObject({
      run_id: 'run-1',
      status: 'success',
      content: expect.stringContaining('"first"'),
    })
    expect(messages[1]).toMatchObject({
      run_id: 'run-2',
      status: 'success',
      content: expect.stringContaining('"second"'),
    })
    expect(messages[0].id).not.toBe(messages[1].id)
  })

  it('Plan 卡片到达时丢弃紧邻的 assistant 前言', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['chunk', { content: '在输出最终方案前，还有几个关键问题需要确认。' }],
        ['plan_question', { id: 'plan_question-1', status: 'success', content: '{"questions":[{"id":"scope","type":"single","question":"确认范围？","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}]}' }],
      ]))
    })

    const messages = readMessages()
    expect(messages.some((message) => message.role === 'assistant')).toBe(false)
    expect(messages.filter((message) => message.role === 'plan_question')).toHaveLength(1)
  })

  it('Plan 卡片到达后丢弃紧邻的 assistant 说明总结', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['proposed_plan', { id: 'proposed_plan-1', status: 'success', content: '# Summary\n\n- A' }],
        ['chunk', { content: '计划已经生成，请确认后执行。' }],
      ]))
    })

    const messages = readMessages()
    expect(messages.some((message) => message.role === 'assistant')).toBe(false)
    expect(messages.filter((message) => message.role === 'proposed_plan')).toHaveLength(1)
  })

  it('plan_questions 工具形态不会展示为工具卡，并在同一 run 内合并成一张问题卡', async () => {
    let agent: ReturnType<typeof useAgentEventStream> | undefined
    render(<AgentStreamHarness onChange={(value) => { agent = value }} />)
    await waitFor(() => expect(agent).toBeDefined())

    await act(async () => {
      await agent?.consumeAgentStream(sseStream([
        ['tool_call', {
          id: 'call-plan-1',
          name: 'plan_questions',
          args: '{"questions":[{"id":"first","question":"第一题？"}]}',
          run_id: 'run-plan-tool',
        }],
        ['tool_call', {
          id: 'call-plan-2',
          name: 'plan_questions',
          args: '{"questions":[{"id":"second","question":"第二题？"}]}',
          run_id: 'run-plan-tool',
        }],
        ['tool_result', { id: 'call-plan-2', name: 'plan_questions', content: 'ignored' }],
      ]))
    })

    const messages = readMessages()
    expect(messages.some((message) => message.role === 'tool_call' || message.role === 'tool_result')).toBe(false)
    const planQuestions = messages.filter((message) => message.role === 'plan_question')
    expect(planQuestions).toHaveLength(1)
    expect(planQuestions[0]).toMatchObject({
      run_id: 'run-plan-tool',
      status: 'success',
      content: expect.stringContaining('"second"'),
    })
  })
})

function AgentStreamHarness({ onChange }: { onChange: (value: ReturnType<typeof useAgentEventStream>) => void }) {
  const agent = useAgentEventStream()
  useEffect(() => onChange(agent), [agent, onChange])
  return (
    <>
      <pre data-testid="messages">{JSON.stringify(agent.messages)}</pre>
      <span data-testid="activity">{agent.activityContent}</span>
    </>
  )
}

function sseStream(events: Array<[string, unknown]>) {
  return new ReadableStream<SSEEvent>({
    start(controller) {
      for (const [event, data] of events) {
        controller.enqueue(sseEvent(event, data))
      }
      controller.close()
    },
  })
}

function sseEvent(event: string, data: unknown): SSEEvent {
  return { event, data: JSON.stringify(data) }
}

function installManualAnimationFrame() {
  const originalRequestAnimationFrame = window.requestAnimationFrame
  const originalCancelAnimationFrame = window.cancelAnimationFrame
  let nextHandle = 1
  let callbacks: Array<{ handle: number; callback: FrameRequestCallback }> = []
  window.requestAnimationFrame = ((callback: FrameRequestCallback) => {
    const handle = nextHandle++
    callbacks.push({ handle, callback })
    return handle
  }) as typeof window.requestAnimationFrame
  window.cancelAnimationFrame = ((handle: number) => {
    callbacks = callbacks.filter((item) => item.handle !== handle)
  }) as typeof window.cancelAnimationFrame
  return {
    flush() {
      const pending = callbacks
      callbacks = []
      for (const item of pending) item.callback(performance.now())
    },
    restore() {
      callbacks = []
      window.requestAnimationFrame = originalRequestAnimationFrame
      window.cancelAnimationFrame = originalCancelAnimationFrame
    },
  }
}

function readMessages() {
  return JSON.parse(screen.getByTestId('messages').textContent || '[]') as Array<{
    id?: string
    role?: string
    content?: string
    streaming_target_content?: string
    name?: string
    args?: string
    status?: string
    result?: string
    streaming?: boolean
    run_id?: string
    subagent?: boolean
    agent_name?: string
    subagent_session_id?: string
    sse_hidden_fields?: string[]
    sse_hidden_reason?: string
    sse_display_notice?: string
    thinking_preview?: string
    illustration?: Record<string, unknown>
  }>
}
