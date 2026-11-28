import { expect, it } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { appendDataMessage, markPlanUIMessageAction } from './agent-chat-state'

it('keeps parallel questions ordered without duplicating rediscovered cards', () => {
  let messages: AgentUIMessage[] = []
  const update = (fn: (value: AgentUIMessage[]) => AgentUIMessage[]) => { messages = fn(messages) }
  for (const id of ['first', 'second', 'first']) appendDataMessage(update, 'data-agent-ask', { id, status: 'pending' })
  expect(messages).toHaveLength(2)
  appendDataMessage(update, 'data-agent-ask', { id: 'first', status: 'answered' })
  expect(messages.flatMap(message => message.parts).map(part => 'data' in part ? part.data : null)).toEqual([{ id: 'first', status: 'answered' }, { id: 'second', status: 'pending' }])
})

it('marks action state on data-agent-proposed-plan parts', () => {
  const messages: AgentUIMessage[] = [{
    id: 'msg-plan',
    role: 'assistant',
    parts: [{
      type: 'data-agent-proposed-plan',
      id: 'plan-1',
      data: { id: 'plan-1', content: 'Outline', status: 'pending' },
    }],
  }]
  const updated = markPlanUIMessageAction(
    messages,
    { messageId: 'msg-plan', partId: 'plan-1', partIndex: 0, type: 'data-agent-proposed-plan' },
    'approved',
  )
  expect(updated[0].parts[0]).toMatchObject({
    type: 'data-agent-proposed-plan',
    data: { id: 'plan-1', plan_action: 'approved', status: 'success' },
  })
})
