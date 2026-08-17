import { describe, expect, it } from 'vitest'
import type { AgentMessageView } from '@/lib/agent-message-view'
import { buildToolCallTree } from './tool-call-tree'

function view(partId: string, parentCallID = ''): AgentMessageView {
  return {
    key: partId,
    kind: 'tool',
    messageId: 'message',
    partId,
    partIndex: 0,
    ref: { messageId: 'message', partId, partIndex: 0, type: 'dynamic-tool' },
    message: {} as AgentMessageView['message'],
    part: {} as AgentMessageView['part'],
    metadata: { parent_call_id: parentCallID || undefined },
    data: {},
    content: partId,
    streaming: false,
  }
}

describe('buildToolCallTree', () => {
  it('builds arbitrary nested Script Tool and task ownership edges', () => {
    const roots = buildToolCallTree([
      view('script'),
      view('saved-tool', 'script'),
      view('task', 'saved-tool'),
      view('child-read', 'task'),
    ])
    expect(roots.map(node => node.view.partId)).toEqual(['script'])
    expect(roots[0].children[0].view.partId).toBe('saved-tool')
    expect(roots[0].children[0].children[0].view.partId).toBe('task')
    expect(roots[0].children[0].children[0].children[0].view.partId).toBe('child-read')
  })

  it('keeps malformed forward references visible without creating a cycle', () => {
    const roots = buildToolCallTree([view('first', 'later'), view('later', 'first')])
    expect(roots.map(node => node.view.partId)).toEqual(['first'])
    expect(roots[0].children.map(node => node.view.partId)).toEqual(['later'])
  })
})
