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
})

function AgentStreamHarness({ onChange }: { onChange: (value: ReturnType<typeof useAgentEventStream>) => void }) {
  const agent = useAgentEventStream()
  useEffect(() => onChange(agent), [agent, onChange])
  return <pre data-testid="messages">{JSON.stringify(agent.messages)}</pre>
}

function sseStream(events: Array<[string, unknown]>) {
  return new ReadableStream<SSEEvent>({
    start(controller) {
      for (const [event, data] of events) {
        controller.enqueue({ event, data: JSON.stringify(data) })
      }
      controller.close()
    },
  })
}

function readMessages() {
  return JSON.parse(screen.getByTestId('messages').textContent || '[]') as Array<{
    role?: string
    content?: string
    name?: string
    args?: string
    status?: string
    result?: string
    streaming?: boolean
    subagent?: boolean
    agent_name?: string
    subagent_session_id?: string
    sse_hidden_fields?: string[]
    sse_hidden_reason?: string
    sse_display_notice?: string
    illustration?: Record<string, unknown>
  }>
}
