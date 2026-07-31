import { describe, expect, it } from 'vitest'
import type { ChatMessage } from './api-client/types'
import { chatMessagesToAgentUIMessages } from './agent-legacy-message'

describe('chatMessagesToAgentUIMessages', () => {
  it('preserves display identity from the interactive render adapter', () => {
    const [message] = chatMessagesToAgentUIMessages([{
      id: 'live-thinking',
      role: 'thinking',
      content: '正在判断门后的动静。',
      run_id: 'run-game',
      display_segment_id: 'thinking-segment',
      display_phase: 'candidate',
    } as ChatMessage])

    expect(message.metadata).toMatchObject({
      run_id: 'run-game',
      display_segment_id: 'thinking-segment',
      display_phase: 'candidate',
    })
  })

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
})
