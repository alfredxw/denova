import { describe, expect, it } from 'vitest'
import { appendBufferedLiveMessage, promoteMessageTargets } from './live-stream-messages'

describe('appendBufferedLiveMessage', () => {
  it('preserves reasoning across separate render batches', () => {
    const first = appendBufferedLiveMessage([], {
      role: 'reasoning',
      content: '正在检查',
      metadata: { run_id: 'run-game', display_segment_id: 'thinking-game' },
    })
    const rendered = promoteMessageTargets(first)
    const queued = appendBufferedLiveMessage(rendered, {
      role: 'reasoning',
      content: '门后的动静。',
      metadata: { run_id: 'run-game', display_segment_id: 'thinking-game' },
    })

    expect(promoteMessageTargets(queued)[0].parts).toEqual([
      expect.objectContaining({ type: 'reasoning', text: '正在检查门后的动静。' }),
    ])
  })

  it('preserves prose across separate render batches', () => {
    const first = appendBufferedLiveMessage([], {
      role: 'assistant',
      content: '门后',
      metadata: { run_id: 'run-game' },
    })
    const rendered = promoteMessageTargets(first)
    const queued = appendBufferedLiveMessage(rendered, {
      role: 'assistant',
      content: '传来脚步声。',
      metadata: { run_id: 'run-game' },
    })

    expect(promoteMessageTargets(queued)[0].parts).toEqual([
      expect.objectContaining({ type: 'text', text: '门后传来脚步声。' }),
    ])
  })
})
