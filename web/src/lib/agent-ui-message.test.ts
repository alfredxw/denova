import { describe, expect, it } from 'vitest'
import { agentViewToRenderMessage, buildAgentMessageViews } from './agent-message-view'
import {
  createAgentDataMessage,
  createAgentReasoningMessage,
  createAgentTextMessage,
  createAgentToolMessage,
  parseAgentToolInput,
} from './agent-ui-message'

describe('Agent UI message constructors', () => {
  it('preserves explicit display identity and provenance', () => {
    const message = createAgentReasoningMessage({
      id: 'thinking-segment',
      text: '正在判断门后的动静。',
      metadata: {
        run_id: 'run-game',
        display_segment_id: 'thinking-segment',
        display_phase: 'candidate',
      },
    })

    expect(message).toMatchObject({
      id: 'thinking-segment',
      metadata: {
        run_id: 'run-game',
        display_segment_id: 'thinking-segment',
        display_phase: 'candidate',
      },
    })
    expect(buildAgentMessageViews([message])[0]).toMatchObject({
      kind: 'reasoning',
      content: '正在判断门后的动静。',
      streaming: false,
    })
  })

  it('constructs text, tool and data parts through one UI protocol', () => {
    const messages = [
      createAgentTextMessage({ id: 'user-1', role: 'user', text: '推开石门' }),
      createAgentToolMessage({
        id: 'tool-1',
        name: 'read',
        state: 'output-available',
        input: parseAgentToolInput('{"path":"story.md"}'),
        output: 'ok',
      }),
      createAgentDataMessage({
        id: 'error-1',
        type: 'agent-error',
        data: { content: 'failed' },
      }),
    ]

    expect(buildAgentMessageViews(messages)).toEqual([
      expect.objectContaining({ kind: 'user', content: '推开石门' }),
      expect.objectContaining({ kind: 'tool', toolName: 'read', input: { path: 'story.md' }, output: 'ok' }),
      expect.objectContaining({ kind: 'error', content: 'failed' }),
    ])
  })

  it('keeps raw tool input authoritative for rendering while parsing remains derived', () => {
    const inputText = '{"path":"chapter.md","edits":[{"old_string":"partial'
    const message = createAgentToolMessage({
      id: 'tool-stream',
      name: 'edit',
      state: 'input-streaming',
      input: { path: 'chapter.md', edits: [{ old_string: 'partial' }] },
      inputText,
    })

    const view = buildAgentMessageViews([message])[0]
    expect(view).toMatchObject({ inputText, streaming: true })
    expect(agentViewToRenderMessage(view)).toMatchObject({ args: inputText })
  })
})
