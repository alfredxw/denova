import { describe, expect, it } from 'vitest'
import { AgentStreamProjection, applyAgentStreamUpdates } from './agent-stream-projection'
import type { AgentUIMessage } from './agent-ui'
import { buildAgentMessageViews } from './agent-message-view'

describe('Agent display stream projection', () => {
  it('retains immutable completed tools while streaming subsequent large results', () => {
    const projection = new AgentStreamProjection()
    let messages: AgentUIMessage[] = []
    const body = 'chapter content '.repeat(4096)
    projection.consume({ type: 'start', messageId: 'run-1' })
    for (let index = 0; index < 100; index++) {
      const previous = messages
      const toolCallId = `call-${index}`
      projection.consume({ type: 'tool-input-start', toolCallId, toolName: 'read' })
      projection.consume({ type: 'tool-input-available', toolCallId, toolName: 'read', input: { path: 'chapter.md' }, toolMetadata: { input_text: '{ "path": "chapter.md" }' } })
      projection.consume({ type: 'tool-output-available', toolCallId, output: body })
      messages = applyAgentStreamUpdates(messages, projection.takeUpdates())
      expect(messages).toHaveLength(index + 1)
      for (let old = 0; old < previous.length; old++) expect(messages[old]).toBe(previous[old])
      expect(messages[index].parts).toHaveLength(1)
      expect(messages[index].parts[0]).toMatchObject({ output: body, toolMetadata: { input_text: '{ "path": "chapter.md" }' } })
    }
    projection.consume({ type: 'text-start', id: 'answer' })
    projection.consume({ type: 'text-delta', id: 'answer', delta: 'hel' })
    messages = applyAgentStreamUpdates(messages, projection.takeUpdates())
    const partial = messages.at(-1)
    projection.consume({ type: 'text-delta', id: 'answer', delta: 'lo' })
    projection.consume({ type: 'text-end', id: 'answer' })
    messages = applyAgentStreamUpdates(messages, projection.takeUpdates())
    expect(partial?.parts[0]).toMatchObject({ text: 'hel', state: 'streaming' })
    expect(messages.at(-1)?.parts[0]).toMatchObject({ text: 'hello', state: 'done' })
  })

  it('keeps interleaved root and delegated content and updates data by identity', () => {
    const projection = new AgentStreamProjection()
    projection.consume({ type: 'start', messageId: 'parent', messageMetadata: { run_id: 'root' } })
    projection.consume({ type: 'text-start', id: 'root-text', providerMetadata: { agent: { run_id: 'root', display_segment_id: 'root-text' } } })
    projection.consume({ type: 'text-start', id: 'child-text', providerMetadata: { agent: { run_id: 'child', display_segment_id: 'child-text', subagent: true, subagent_session_id: 'child-session' } } })
    projection.consume({ type: 'text-delta', id: 'root-text', delta: 'root' })
    projection.consume({ type: 'text-delta', id: 'child-text', delta: 'child' })
    projection.consume({ type: 'data-agent-ask', id: 'ask', data: { status: 'pending' } })
    let messages = applyAgentStreamUpdates([], projection.takeUpdates())
    projection.consume({ type: 'data-agent-ask', id: 'ask', data: { status: 'resolved' } })
    projection.consume({ type: 'finish' })
    messages = applyAgentStreamUpdates(messages, projection.takeUpdates())
    const views = buildAgentMessageViews(messages)
    expect(views.map(view => [view.content, view.metadata.run_id, view.streaming])).toEqual([
      ['root', 'root', false], ['child', 'child', false], ['', 'root', false],
    ])
    expect(messages).toHaveLength(3)
    expect(messages[2].parts[0]).toMatchObject({ data: { status: 'resolved' } })
  })
})
