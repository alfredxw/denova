import { describe, expect, it } from 'vitest'
import { agentViewToRenderMessage, buildAgentMessageViews } from './agent-message-view'
import type { AgentUIMessage } from './agent-ui'
import { createAgentToolMessage } from './agent-ui-message'
import { completeToolMessage, updateToolMessageInput } from '@/features/interactive/components/story-stage/live-stream-messages'

describe('tool inspection projection', () => {
  it('preserves Game history and streaming arguments through tool completion', () => {
    const input = ' {"id":9223372036854775807,"id":1} '
    const history = createAgentToolMessage({ name: 'custom', state: 'output-available', input, output: 'done' })
    const started = createAgentToolMessage({ name: 'custom', state: 'input-streaming', input: input.slice(0, 10) })
    const completed = completeToolMessage(updateToolMessageInput(started, input), 'done')
    for (const message of [history, completed]) {
      expect(agentViewToRenderMessage(buildAgentMessageViews([message])[0])).toMatchObject({ args: input, args_preview: false, result: 'done' })
    }
  })
  it('prefers recorded text over the SDK parsed input and preserves output metadata', () => {
    const input = ' {"id":9223372036854775807,"id":1} '
    const messages = [{ id: 'm1', role: 'assistant', parts: [{ type: 'dynamic-tool', toolCallId: 'c1', toolName: 'custom', state: 'output-available', input: { id: 1 }, output: 'raw\n', toolMetadata: { input_text: input, display_truncated: true } }] }] as AgentUIMessage[]
    expect(agentViewToRenderMessage(buildAgentMessageViews(messages)[0])).toMatchObject({ args: input, args_preview: false, result: 'raw\n', result_truncated: true })
  })

  it('identifies partial SDK input as a preview', () => {
    const messages = [{ id: 'm1', role: 'assistant', parts: [{ type: 'dynamic-tool', toolCallId: 'c1', toolName: 'custom', state: 'input-streaming', input: { text: 'partial' } }] }] as AgentUIMessage[]
    expect(agentViewToRenderMessage(buildAgentMessageViews(messages)[0])).toMatchObject({ args: '{"text":"partial"}', args_preview: true, streaming: true })
  })
})
