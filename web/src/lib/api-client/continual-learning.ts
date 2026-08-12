import type { UIMessageChunk } from 'ai'
import type { AgentUIMessage } from '@/lib/agent-ui'
import type { ActiveChatTask } from './chat'
import type { AgentAskAnswer, AgentAskResolution } from './types'
import { fetchAPI, jsonHeaders, parseUIMessageStream, requestJSON, responseAPIError } from './client'

const ROOT = '/api/continual-learning'

export interface HarnessStateFile {
  path: string
  content: string
}

export interface HarnessStateSnapshot {
  revision: string
  files: HarnessStateFile[]
}

export interface HarnessStateChange {
  path: string
  content?: string
  delete?: boolean
}

export interface HarnessStateVersion {
  id: string
  revision: string
  summary: string
  created_at: string
}

export interface HarnessStateVersionDiff {
  from: string
  to: string
  patch: string
}

export interface HarnessStateUpdateResult {
  version?: HarnessStateVersion
  changed: boolean
}

export interface HarnessOptimizerMessagesPage {
  messages: AgentUIMessage[]
  page: { next_before: string; has_more: boolean; total: number }
}

export interface ContinualLearningScheduleStatus {
  enabled: boolean
  interval_hours: number
  last_attempt?: string
  last_success?: string
  last_task_id?: string
}

export function getHarnessState(): Promise<HarnessStateSnapshot> {
  return requestJSON(`${ROOT}/state`)
}

export function updateHarnessState(request: {
  base_revision: string
  summary: string
  changes: HarnessStateChange[]
}): Promise<HarnessStateUpdateResult> {
  return requestJSON(`${ROOT}/state`, {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(request),
  })
}

export function getHarnessStateVersions(limit = 100): Promise<HarnessStateVersion[]> {
  return requestJSON(`${ROOT}/versions?limit=${encodeURIComponent(String(limit))}`)
}

export function getHarnessStateVersionDiff(from: string, to: string): Promise<HarnessStateVersionDiff> {
  const params = new URLSearchParams({ from, to })
  return requestJSON(`${ROOT}/versions/diff?${params.toString()}`)
}

export function restoreHarnessStateVersion(id: string): Promise<HarnessStateUpdateResult> {
  return requestJSON(`${ROOT}/versions/${encodeURIComponent(id)}/restore`, { method: 'POST' })
}

export function getContinualLearningSchedule(): Promise<ContinualLearningScheduleStatus> {
  return requestJSON(`${ROOT}/schedule`)
}

export async function runHarnessOptimizer(commandId: string, instruction = ''): Promise<ReadableStream<UIMessageChunk>> {
  const response = await fetchAPI(`${ROOT}/optimize/stream`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ command_id: commandId, instruction }),
  })
  if (!response.ok) throw await responseAPIError(response)
  if (!response.body) throw new Error('Harness Optimizer returned no response body')
  return parseUIMessageStream(response.body)
}

export async function reconnectHarnessOptimizer(taskId: string): Promise<ReadableStream<UIMessageChunk>> {
  const params = new URLSearchParams({ task_id: taskId })
  const response = await fetchAPI(`${ROOT}/optimize/stream?${params.toString()}`)
  if (!response.ok) throw await responseAPIError(response)
  if (!response.body) throw new Error('Harness Optimizer returned no response body')
  return parseUIMessageStream(response.body)
}

export function getActiveHarnessOptimizer(): Promise<ActiveChatTask> {
  return requestJSON(`${ROOT}/optimize/active`)
}

export function getHarnessOptimizerMessages(before?: string, limit = 100): Promise<HarnessOptimizerMessagesPage> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (before) params.set('before', before)
  return requestJSON(`${ROOT}/optimize/messages?${params.toString()}`)
}

export async function clearHarnessOptimizer(): Promise<void> {
  await requestJSON(`${ROOT}/optimize/clear`, { method: 'POST' })
}

export function answerHarnessOptimizerAsk(askId: string, answers: AgentAskAnswer[]): Promise<AgentAskResolution> {
  return requestJSON(`${ROOT}/optimize/asks/${encodeURIComponent(askId)}/answer`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ answers }),
  })
}

export function cancelHarnessOptimizerAsk(askId: string): Promise<AgentAskResolution> {
  return requestJSON(`${ROOT}/optimize/asks/${encodeURIComponent(askId)}/cancel`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ reason: 'user_cancelled' }),
  })
}
