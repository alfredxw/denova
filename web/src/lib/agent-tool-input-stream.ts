import type { UIMessageChunk } from 'ai'

// Providers are allowed to coalesce an entire tool argument into one delta.
// Bound the number of presentation frames so large inputs remain responsive
// while still making every oversized raw delta visibly progressive.
const toolInputSplitThreshold = 512
const maxToolInputRenderChunks = 12

/** Splits only transport framing; the concatenated raw text is unchanged. */
export async function* toolInputTextRenderChunks(delta: string): AsyncGenerator<string> {
  if (delta.length <= toolInputSplitThreshold) {
    yield delta
    return
  }

  const chunkLength = Math.ceil(delta.length / maxToolInputRenderChunks)
  for (let start = 0; start < delta.length;) {
    let end = Math.min(delta.length, start + chunkLength)
    // Do not leave a UTF-16 surrogate pair half-rendered for one frame.
    if (end < delta.length && isHighSurrogate(delta.charCodeAt(end - 1)) && isLowSurrogate(delta.charCodeAt(end))) {
      end += 1
    }
    yield delta.slice(start, end)
    start = end
    if (start < delta.length) await nextRenderFrame()
  }
}

/** Applies the same schema-agnostic framing to every AI SDK tool input. */
export async function* agentToolInputRenderChunks(chunk: UIMessageChunk): AsyncGenerator<UIMessageChunk> {
  if (chunk.type !== 'tool-input-delta') {
    yield chunk
    return
  }
  for await (const inputTextDelta of toolInputTextRenderChunks(chunk.inputTextDelta)) {
    yield { ...chunk, inputTextDelta }
  }
}

function nextRenderFrame() {
  if (typeof requestAnimationFrame !== 'function') return Promise.resolve()
  return new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
}

function isHighSurrogate(value: number) {
  return value >= 0xD800 && value <= 0xDBFF
}

function isLowSurrogate(value: number) {
  return value >= 0xDC00 && value <= 0xDFFF
}
