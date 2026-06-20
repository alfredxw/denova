import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from './client'
import type { ChatMessage, SSEEvent } from './types'

export interface ConfigManagerRunRequest {
  instruction: string
  origin?: string
  resource_id?: string
  story_id?: string
  branch_id?: string
  references?: string[]
  context?: Record<string, string>
}

export async function runConfigManagerStream(req: ConfigManagerRunRequest): Promise<ReadableStream<SSEEvent>> {
  const res = await fetchAPI('/api/config-manager/stream', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export function getConfigManagerMessages(): Promise<ChatMessage[]> {
  return requestJSON('/api/config-manager/messages')
}

export async function clearConfigManagerSession(): Promise<void> {
  await requestJSON('/api/config-manager/clear', { method: 'POST' })
}
