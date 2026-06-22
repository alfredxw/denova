import { describe, expect, it } from 'vitest'
import { buildContextCompactionMessage, upsertContextCompactionMessage } from './context-compaction-message'
import type { ChatMessage } from '@/lib/api'

describe('context compaction message helpers', () => {
  it('appends deltas to one compaction card and replaces content on retry', () => {
    let messages: ChatMessage[] = []
    const id = 'context-compaction:test'

    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'started', phase: 'pre_run', tokens_before: 1200 }, id))
    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'delta', attempt: 1, delta: '第一段' }, id))
    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'delta', attempt: 1, delta: '第二段' }, id))

    expect(messages).toHaveLength(1)
    expect(messages[0]).toMatchObject({ role: 'context_compaction', status: 'running', content: '第一段第二段' })

    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'delta', attempt: 2, delta: '重试摘要' }, id))
    expect(messages[0]).toMatchObject({ attempt: 2, content: '重试摘要' })

    messages = upsertContextCompactionMessage(messages, buildContextCompactionMessage({ status: 'completed', summary: '最终摘要', tokens_after: 240, epoch: 3 }, id))
    expect(messages[0]).toMatchObject({ status: 'success', content: '最终摘要', tokens_after: 240, epoch: 3 })
  })
})
