import { expect, it } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { appendDataMessage } from './agent-chat-state'

it('keeps parallel questions ordered without duplicating rediscovered cards', () => {
  let messages: AgentUIMessage[] = []
  const update = (fn: (value: AgentUIMessage[]) => AgentUIMessage[]) => { messages = fn(messages) }
  for (const id of ['first', 'second', 'first']) appendDataMessage(update, 'data-agent-ask', { id, status: 'pending' })
  expect(messages).toHaveLength(2)
  appendDataMessage(update, 'data-agent-ask', { id: 'first', status: 'answered' })
  expect(messages.flatMap(message => message.parts).map(part => 'data' in part ? part.data : null)).toEqual([{ id: 'first', status: 'answered' }, { id: 'second', status: 'pending' }])
})
