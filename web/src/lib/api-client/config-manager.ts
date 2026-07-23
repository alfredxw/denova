import type { UIMessageChunk } from 'ai'
import { fetchAPI, jsonHeaders, parseUIMessageStream, requestJSON, responseAPIError } from './client'
import type { ActiveChatTask, AgentRuntimeRecoveryAction, AgentRuntimeRecoveryReceipt } from './chat'
import type { AgentUIMessage } from '@/lib/agent-ui'

export interface ConfigManagerRunRequest {
  command_id: string
  instruction: string
  origin?: string
  resource_id?: string
  story_id?: string
  branch_id?: string
  references?: string[]
  context?: Record<string, string>
}

export type ConfigManagerScope = Omit<ConfigManagerRunRequest, 'command_id' | 'instruction' | 'references' | 'context'>

export async function runConfigManagerStream(req: ConfigManagerRunRequest): Promise<ReadableStream<UIMessageChunk>> {
  const res = await fetchAPI('/api/config-manager/stream', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw await responseAPIError(res)
  }
  if (!res.body) throw new Error('No response body')
  return parseUIMessageStream(res.body)
}

export function getConfigManagerMessages(scope: ConfigManagerScope = {}): Promise<AgentUIMessage[]> {
  return requestJSON(`/api/config-manager/messages${configManagerScopeQuery(scope)}`)
}

/** Inspect one exact Config Manager scope without exposing durable input. */
export function getActiveConfigManagerTask(scope: ConfigManagerScope = {}): Promise<ActiveChatTask> {
  return requestJSON(`/api/config-manager/active${configManagerScopeQuery(scope)}`)
}

/** Attach to the server-selected display Task for this exact scope. */
export async function reconnectConfigManagerStream(
  scope: ConfigManagerScope,
  taskId: string,
  after = 0,
): Promise<ReadableStream<UIMessageChunk>> {
  const taskID = taskId.trim()
  if (!taskID) throw new Error('Cannot reconnect Config Manager without an exact task ID')
  const params = configManagerScopeParams(scope)
  params.set('task_id', taskID)
  if (after > 0) params.set('after', String(after))
  const res = await fetchAPI(`/api/config-manager/stream?${params.toString()}`)
  if (!res.ok) throw await responseAPIError(res)
  if (!res.body) throw new Error('No response body')
  return parseUIMessageStream(res.body)
}

/** Execute only a payload-free recovery identity projected by the server. */
export function recoverConfigManagerRuntime(
  scope: ConfigManagerScope,
  action: AgentRuntimeRecoveryAction,
): Promise<AgentRuntimeRecoveryReceipt> {
  return requestJSON(`/api/config-manager/recovery${configManagerScopeQuery(scope)}`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ action }),
  })
}

export async function clearConfigManagerSession(scope: ConfigManagerScope = {}): Promise<void> {
  await requestJSON(`/api/config-manager/clear${configManagerScopeQuery(scope)}`, { method: 'POST' })
}

function configManagerScopeQuery(scope: ConfigManagerScope): string {
  const params = configManagerScopeParams(scope)
  const query = params.toString()
  return query ? `?${query}` : ''
}

function configManagerScopeParams(scope: ConfigManagerScope): URLSearchParams {
  const params = new URLSearchParams()
  appendParam(params, 'origin', scope.origin)
  appendParam(params, 'resource_id', scope.resource_id)
  appendParam(params, 'story_id', scope.story_id)
  appendParam(params, 'branch_id', scope.branch_id)
  return params
}

function appendParam(params: URLSearchParams, key: string, value?: string) {
  const trimmed = value?.trim()
  if (trimmed) params.set(key, trimmed)
}
