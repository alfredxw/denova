import type { SSEEvent } from './types'

export const jsonHeaders = { 'Content-Type': 'application/json' }

export async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  const text = await res.text()
  let data: Record<string, any> = {}
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { error: text }
    }
  }
  if (!res.ok) {
    throw new Error(data.error || `HTTP ${res.status}`)
  }
  return data as T
}

export async function readErrorMessage(res: Response): Promise<string> {
  let message = `HTTP ${res.status}`
  try {
    const data = await res.json()
    message = data.error || message
  } catch {
    // keep HTTP fallback
  }
  return message
}

export function parseSSEStream<T extends SSEEvent = SSEEvent>(body: ReadableStream<Uint8Array>): ReadableStream<T> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  return new ReadableStream<T>({
    async pull(controller) {
      while (true) {
        const { done, value } = await reader.read()
        if (done) {
          controller.close()
          return
        }
        buffer += decoder.decode(value, { stream: true })

        const events = buffer.split('\n\n')
        buffer = events.pop() || ''

        for (const eventStr of events) {
          if (!eventStr.trim()) continue
          const lines = eventStr.split('\n')
          let event = ''
          let data = ''
          for (const line of lines) {
            if (line.startsWith('event: ')) event = line.slice(7)
            else if (line.startsWith('data: ')) data = line.slice(6)
          }
          if (event) {
            controller.enqueue({ event, data } as T)
          }
        }
      }
    },
  })
}
