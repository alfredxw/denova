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

export type ConfigManagerScope = Omit<ConfigManagerRunRequest, 'instruction' | 'references' | 'context'>

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

export function getConfigManagerMessages(scope: ConfigManagerScope = {}): Promise<ChatMessage[]> {
  return requestJSON(`/api/config-manager/messages${configManagerScopeQuery(scope)}`)
}

export async function clearConfigManagerSession(scope: ConfigManagerScope = {}): Promise<void> {
  await requestJSON(`/api/config-manager/clear${configManagerScopeQuery(scope)}`, { method: 'POST' })
}

function configManagerScopeQuery(scope: ConfigManagerScope): string {
  const params = new URLSearchParams()
  appendParam(params, 'origin', scope.origin)
  appendParam(params, 'resource_id', scope.resource_id)
  appendParam(params, 'story_id', scope.story_id)
  appendParam(params, 'branch_id', scope.branch_id)
  const query = params.toString()
  return query ? `?${query}` : ''
}

function appendParam(params: URLSearchParams, key: string, value?: string) {
  const trimmed = value?.trim()
  if (trimmed) params.set(key, trimmed)
}
