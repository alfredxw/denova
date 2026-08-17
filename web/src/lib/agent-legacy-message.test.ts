import { describe, expect, it } from 'vitest'
import type { ChatMessage } from './api-client/types'
import { chatMessagesToAgentUIMessages } from './agent-legacy-message'

describe('chatMessagesToAgentUIMessages', () => {
  it('preserves completed message identity while only the live tail changes', () => {
    const completed: ChatMessage = {
      id: 'turn-1-assistant',
      role: 'assistant',
      content: '已经完成的历史正文',
    }
    const first = chatMessagesToAgentUIMessages([
      completed,
      { id: 'live-assistant', role: 'assistant', content: '流式', streaming: true },
    ])

    const second = chatMessagesToAgentUIMessages([
      completed,
      { id: 'live-assistant', role: 'assistant', content: '流式更新', streaming: true },
    ])

    expect(second[0]).toBe(first[0])
    expect(second[1]).not.toBe(first[1])
  })

  it('preserves the paired context usage fields in token usage data', () => {
    const [message] = chatMessagesToAgentUIMessages([{
      id: 'usage-1',
      role: 'token_usage',
      context_window_tokens: 400000,
      context_prompt_tokens: 1200,
      prompt_tokens: 91000,
      model_calls: 2,
    }])

    expect(message.parts[0]).toMatchObject({
      type: 'data-agent-token-usage',
      data: {
        context_window_tokens: 400000,
        context_prompt_tokens: 1200,
        prompt_tokens: 91000,
      },
    })
  })
})
