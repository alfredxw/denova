import { describe, expect, it } from 'vitest'
import type { UIMessageChunk } from 'ai'
import { agentToolInputRenderChunks, toolInputTextRenderChunks } from './agent-tool-input-stream'

describe('tool input render framing', () => {
  it('turns one oversized edit delta into visible append-only raw-text frames', async () => {
    const raw = `{"edits":[{"new_string":"${'内容😀'.repeat(800)}","old_string":"marker"}],"path":"chapter.md"}`
    const chunks = await collect(toolInputTextRenderChunks(raw))

    expect(chunks.length).toBeGreaterThan(1)
    expect(chunks.length).toBeLessThanOrEqual(12)
    expect(chunks.join('')).toBe(raw)

    let accumulated = ''
    for (const chunk of chunks) {
      accumulated += chunk
      expect(raw.startsWith(accumulated)).toBe(true)
      const finalCodeUnit = chunk.charCodeAt(chunk.length - 1)
      expect(finalCodeUnit < 0xD800 || finalCodeUnit > 0xDBFF).toBe(true)
    }
  })

  it('uses the same framing for every tool because tool name and schema are absent', async () => {
    const raw = `{"value":"${'x'.repeat(2000)}"}`
    const source: UIMessageChunk = {
      type: 'tool-input-delta',
      toolCallId: 'call-1',
      inputTextDelta: raw,
    }
    const chunks = await collect(agentToolInputRenderChunks(source))

    expect(chunks.length).toBeGreaterThan(1)
    expect(chunks.every((chunk) => chunk.type === 'tool-input-delta')).toBe(true)
    expect(chunks.map((chunk) => chunk.type === 'tool-input-delta' ? chunk.inputTextDelta : '').join('')).toBe(raw)
  })

  it('leaves already fine-grained provider deltas unchanged', async () => {
    const raw = '{"path":"chapter.md"'
    expect(await collect(toolInputTextRenderChunks(raw))).toEqual([raw])
  })
})

async function collect<T>(stream: AsyncIterable<T>) {
  const values: T[] = []
  for await (const value of stream) values.push(value)
  return values
}
