import type { UIMessageChunk } from 'ai'
import { fetchAPI, jsonHeaders, parseUIMessageStream, requestJSON, responseAPIError } from './client'
import type { ActiveChatTask, AgentRuntimeRecoveryAction, AgentRuntimeRecoveryReceipt } from './chat'
import type { AgentAskAnswer, AgentAskResolution } from './types'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { projectAPIPath } from './project-scope'

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

function configManagerPath(projectId: string, suffix: string): string {
  return projectAPIPath(projectId, `config-manager/${suffix.replace(/^\/+/, '')}`)
}

export async function runConfigManagerStream(projectId: string, req: ConfigManagerRunRequest): Promise<ReadableStream<UIMessageChunk>> {
  const res = await fetchAPI(configManagerPath(projectId, 'stream'), {
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

export function getConfigManagerMessages(projectId: string, scope: ConfigManagerScope = {}): Promise<AgentUIMessage[]> {
  return requestJSON(`${configManagerPath(projectId, 'messages')}${configManagerScopeQuery(scope)}`)
}

export interface ConfigManagerMessagesPage {
  messages: AgentUIMessage[]
  nextBefore: string
  hasMore: boolean
  total: number
}

export async function getConfigManagerMessagesPage(
  projectId: string,
  scope: ConfigManagerScope = {},
  options: { before?: string; limit?: number } = {},
): Promise<ConfigManagerMessagesPage> {
  const params = configManagerScopeParams(scope)
  params.set('limit', String(options.limit || 100))
  if (options.before) params.set('before', options.before)
  const data = await requestJSON<
    | AgentUIMessage[]
    | { messages?: AgentUIMessage[]; page?: { next_before?: string; has_more?: boolean; total?: number } }
  >(`${configManagerPath(projectId, 'messages')}?${params.toString()}`)
  if (Array.isArray(data)) {
    return { messages: data, nextBefore: '0', hasMore: false, total: data.length }
  }
  return {
    messages: data.messages || [],
    nextBefore: data.page?.next_before || '0',
    hasMore: data.page?.has_more === true,
    total: data.page?.total || 0,
  }
}

/** Inspect one exact Config Manager scope without exposing durable input. */
export function getActiveConfigManagerTask(projectId: string, scope: ConfigManagerScope = {}): Promise<ActiveChatTask> {
  return requestJSON(`${configManagerPath(projectId, 'active')}${configManagerScopeQuery(scope)}`)
}

/** Attach to the server-selected display Task for this exact scope. */
export async function reconnectConfigManagerStream(
  projectId: string,
  scope: ConfigManagerScope,
  taskId: string,
  after = 0,
): Promise<ReadableStream<UIMessageChunk>> {
  const taskID = taskId.trim()
  if (!taskID) throw new Error('Cannot reconnect Config Manager without an exact task ID')
  const params = configManagerScopeParams(scope)
  params.set('task_id', taskID)
  if (after > 0) params.set('after', String(after))
  const res = await fetchAPI(`${configManagerPath(projectId, 'stream')}?${params.toString()}`)
  if (!res.ok) throw await responseAPIError(res)
  if (!res.body) throw new Error('No response body')
  return parseUIMessageStream(res.body)
}

/** Execute only a payload-free recovery identity projected by the server. */
export function recoverConfigManagerRuntime(
  projectId: string,
  scope: ConfigManagerScope,
  action: AgentRuntimeRecoveryAction,
): Promise<AgentRuntimeRecoveryReceipt> {
  return requestJSON(`${configManagerPath(projectId, 'recovery')}${configManagerScopeQuery(scope)}`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ action }),
  })
}

export async function clearConfigManagerSession(projectId: string, scope: ConfigManagerScope = {}): Promise<void> {
  await requestJSON(`${configManagerPath(projectId, 'clear')}${configManagerScopeQuery(scope)}`, { method: 'POST' })
}

export function answerConfigManagerAsk(projectId: string, scope: ConfigManagerScope, askId: string, answers: AgentAskAnswer[]): Promise<AgentAskResolution> {
  return requestJSON(`${configManagerPath(projectId, `asks/${encodeURIComponent(askId)}/answer`)}${configManagerScopeQuery(scope)}`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ answers }),
  })
}

export function cancelConfigManagerAsk(projectId: string, scope: ConfigManagerScope, askId: string): Promise<AgentAskResolution> {
  return requestJSON(`${configManagerPath(projectId, `asks/${encodeURIComponent(askId)}/cancel`)}${configManagerScopeQuery(scope)}`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ reason: 'user_cancelled' }),
  })
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
