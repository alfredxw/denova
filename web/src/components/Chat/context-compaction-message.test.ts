import { describe, expect, it } from 'vitest'
import { buildContextCompactionMessage, upsertContextCompactionMessage } from './context-compaction-message'
import { buildAgentMessageViews } from '@/lib/agent-message-view'
import type { AgentUIMessage } from '@/lib/agent-ui'

describe('context compaction message helpers', () => {
  it('appends deltas to one compaction card and replaces content on retry', () => {
    let messages: AgentUIMessage[] = []
    const id = 'context-compaction:test'

    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'started', phase: 'pre_run', tokens_before: 1200 }, id))
    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'delta', attempt: 1, delta: '第一段' }, id))
    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'delta', attempt: 1, delta: '第二段' }, id))

    expect(messages).toHaveLength(1)
    expect(buildAgentMessageViews(messages)[0]).toMatchObject({ kind: 'context-compaction', status: 'running', content: '第一段第二段' })

    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'delta', attempt: 2, delta: '重试摘要' }, id))
    expect(buildAgentMessageViews(messages)[0]).toMatchObject({ content: '重试摘要', data: { attempt: 2 } })

    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'completed', summary: '最终摘要', tokens_after: 240, epoch: 3 }, id))
    expect(buildAgentMessageViews(messages)[0]).toMatchObject({ status: 'success', content: '最终摘要', data: { tokens_after: 240, epoch: 3 } })
  })
})
