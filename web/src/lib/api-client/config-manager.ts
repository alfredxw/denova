import type { UIMessageChunk } from 'ai'
import { fetchAPI, jsonHeaders, parseUIMessageStream, readErrorMessage, requestJSON } from './client'
import type { AgentUIMessage } from '@/lib/agent-ui'

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

export async function runConfigManagerStream(req: ConfigManagerRunRequest): Promise<ReadableStream<UIMessageChunk>> {
  const res = await fetchAPI('/api/config-manager/stream', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  if (!res.body) throw new Error('No response body')
  return parseUIMessageStream(res.body)
}

export function getConfigManagerMessages(scope: ConfigManagerScope = {}): Promise<AgentUIMessage[]> {
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
