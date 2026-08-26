import { describe, expect, it } from 'vitest'
import { createAgentToolMessage } from '@/lib/agent-ui-message'
import { completeToolMessage, toolMessageInput, updateToolMessageInput } from './live-stream-messages'

describe('live tool input', () => {
  it('uses the canonical input field for streaming and completed tool state', () => {
    const partialInput = '{"path":"chapters/ch01.md"'
    const completeInput = '{"path":"chapters/ch01.md","content":"draft"}'
    const streaming = createAgentToolMessage({
      id: 'tool-write',
      name: 'write',
      state: 'input-streaming',
      input: partialInput,
    })

    expect(toolMessageInput(streaming)).toBe(partialInput)

    const updated = updateToolMessageInput(streaming, completeInput)
    const completed = completeToolMessage(updated, 'done')

    expect(completed.parts[0]).toMatchObject({
      state: 'output-available',
      input: { path: 'chapters/ch01.md', content: 'draft' },
      output: 'done',
    })
    expect(completed.parts[0]).not.toHaveProperty('inputText')
  })
})
