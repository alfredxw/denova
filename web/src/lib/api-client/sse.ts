import type { SSEEvent } from './types'

/** Parses an SSE byte stream without depending on Window-only APIs. */
export function parseSSEStream<T extends SSEEvent = SSEEvent>(body: ReadableStream<Uint8Array>): ReadableStream<T> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let scanOffset = 0

  return new ReadableStream<T>({
    async pull(controller) {
      while (true) {
        const { done, value } = await reader.read()
        if (done) {
          buffer += decoder.decode()
          enqueueSSEBlocks(controller, true)
          controller.close()
          reader.releaseLock()
          return
        }
        buffer += decoder.decode(value, { stream: true })
        if (enqueueSSEBlocks(controller, false) > 0) return
      }
    },
    async cancel(reason) {
      await reader.cancel(reason)
      reader.releaseLock()
    },
  })

  function enqueueSSEBlocks(controller: ReadableStreamDefaultController<T>, flushRemainder: boolean) {
    let enqueued = 0
    while (true) {
      const boundary = findSSEBoundary(buffer, scanOffset)
      if (!boundary) break
      const block = buffer.slice(0, boundary.index)
      buffer = buffer.slice(boundary.index + boundary.length)
      scanOffset = 0
      if (enqueueSSEBlock(controller, block)) enqueued += 1
    }
    if (flushRemainder) {
      if (enqueueSSEBlock(controller, buffer)) enqueued += 1
      buffer = ''
      scanOffset = 0
    } else {
      // Only rescan the suffix that can contain a separator split across chunks.
      scanOffset = Math.max(0, buffer.length - 3)
    }
    return enqueued
  }
}

function findSSEBoundary(value: string, fromIndex: number): { index: number; length: number } | null {
  const lineFeed = value.indexOf('\n\n', fromIndex)
  const carriageReturn = value.indexOf('\r\r', fromIndex)
  const crlf = value.indexOf('\r\n\r\n', fromIndex)
  const candidates = [
    lineFeed >= 0 ? { index: lineFeed, length: 2 } : null,
    carriageReturn >= 0 ? { index: carriageReturn, length: 2 } : null,
    crlf >= 0 ? { index: crlf, length: 4 } : null,
  ].filter((candidate): candidate is { index: number; length: number } => candidate !== null)
  if (candidates.length === 0) return null
  return candidates.reduce((earliest, candidate) => candidate.index < earliest.index ? candidate : earliest)
}

function enqueueSSEBlock<T extends SSEEvent>(controller: ReadableStreamDefaultController<T>, block: string) {
  if (!block.trim()) return false
  let id: string | undefined
  let event = ''
  const data: string[] = []
  for (const line of block.split(/\r\n|\r|\n/)) {
    if (!line || line.startsWith(':')) continue
    const separator = line.indexOf(':')
    const field = separator >= 0 ? line.slice(0, separator) : line
    const rawValue = separator >= 0 ? line.slice(separator + 1) : ''
    const value = rawValue.startsWith(' ') ? rawValue.slice(1) : rawValue
    if (field === 'id') id = value
    else if (field === 'event') event = value
    else if (field === 'data') data.push(value)
  }
  if (!event) return false
  controller.enqueue({ ...(id === undefined ? {} : { id }), event, data: data.join('\n') } as T)
  return true
}
