import { jsonHeaders, parseSSEStream, requestJSON } from './client'
import type { AutomationActiveRun, AutomationRunResult, AutomationTask, ChatMessage, SSEEvent } from './types'

export async function getAutomations(): Promise<AutomationTask[]> {
  const data = await requestJSON<{ tasks: AutomationTask[] }>('/api/automations')
  return data.tasks || []
}

export async function createAutomation(task: AutomationTask): Promise<AutomationTask> {
  return requestJSON('/api/automations', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(task),
  })
}

export async function updateAutomation(id: string, task: AutomationTask): Promise<AutomationTask> {
  return requestJSON(`/api/automations/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(task),
  })
}

export async function deleteAutomation(id: string): Promise<void> {
  await requestJSON(`/api/automations/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function runAutomation(id: string): Promise<AutomationRunResult> {
  return requestJSON(`/api/automations/${encodeURIComponent(id)}/run`, { method: 'POST' })
}

export async function streamAutomationRun(id: string, signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetch(`/api/automations/${encodeURIComponent(id)}/run/stream`, { method: 'POST', signal })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function getActiveAutomationRuns(): Promise<AutomationActiveRun[]> {
  const data = await requestJSON<{ runs: AutomationActiveRun[] }>('/api/automations/runs/active')
  return data.runs || []
}

export async function streamAutomationRunByID(runId: string, signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetch(`/api/automations/runs/${encodeURIComponent(runId)}/stream`, { signal })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function streamAutomationRunMessage(runId: string, message: string, signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetch(`/api/automations/runs/${encodeURIComponent(runId)}/chat/stream`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ message }),
    signal,
  })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function abortAutomationRun(runId: string): Promise<void> {
  await requestJSON(`/api/automations/runs/${encodeURIComponent(runId)}/abort`, { method: 'POST' })
}

export async function getAutomationRunMessages(runId: string): Promise<ChatMessage[]> {
  return requestJSON(`/api/automations/runs/${encodeURIComponent(runId)}/messages`)
}
