import type { UIMessageChunk } from 'ai'
import { fetchAPI, jsonHeaders, parseUIMessageStream, requestJSON } from './client'
import type { AutomationActiveRun, AutomationInboxActionResult, AutomationInboxItem, AutomationTask, AutomationTaskTemplate, AutomationTriggerEvidence } from './types'
import type { AgentUIMessage } from '@/lib/agent-ui'

export async function getAutomations(): Promise<AutomationTask[]> {
  const data = await requestJSON<{ tasks: AutomationTask[] }>('/api/automations')
  return data.tasks || []
}

export async function getAutomationTemplates(locale: string): Promise<AutomationTaskTemplate[]> {
  const data = await requestJSON<{ templates: AutomationTaskTemplate[] }>(`/api/automations/templates?locale=${encodeURIComponent(locale)}`)
  return data.templates || []
}

export async function createAutomation(task: AutomationTask): Promise<AutomationTask> {
  return requestJSON('/api/automations', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(task),
  })
}

export async function getAutomationInbox(): Promise<AutomationInboxItem[]> {
  const data = await requestJSON<{ items: AutomationInboxItem[] }>('/api/automations/inbox')
  return data.items || []
}

export async function checkAutomation(id: string): Promise<AutomationInboxItem[]> {
  const data = await requestJSON<{ items: AutomationInboxItem[] }>(`/api/automations/${encodeURIComponent(id)}/check`, { method: 'POST' })
  return data.items || []
}

export async function confirmAutomationInboxItem(id: string): Promise<AutomationInboxActionResult> {
  return requestJSON(`/api/automations/inbox/${encodeURIComponent(id)}/confirm`, { method: 'POST' })
}

export async function dismissAutomationInboxItem(id: string): Promise<AutomationInboxItem> {
  return requestJSON(`/api/automations/inbox/${encodeURIComponent(id)}/dismiss`, { method: 'POST' })
}

export async function markAutomationInboxItemRead(id: string): Promise<AutomationInboxItem> {
  return requestJSON(`/api/automations/inbox/${encodeURIComponent(id)}/read`, { method: 'POST' })
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

export async function streamAutomationRun(id: string, signal?: AbortSignal, triggerEvidence: AutomationTriggerEvidence[] = []): Promise<ReadableStream<UIMessageChunk>> {
  const init: RequestInit = { method: 'POST', signal }
  if (triggerEvidence.length > 0) {
    init.headers = jsonHeaders
    init.body = JSON.stringify({ trigger_evidence: triggerEvidence })
  }
  const res = await fetchAPI(`/api/automations/${encodeURIComponent(id)}/run/stream`, init)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseUIMessageStream(res.body)
}

export async function getActiveAutomationRuns(): Promise<AutomationActiveRun[]> {
  const data = await requestJSON<{ runs: AutomationActiveRun[] }>('/api/automations/runs/active')
  return data.runs || []
}

export async function streamAutomationRunByID(runId: string, signal?: AbortSignal): Promise<ReadableStream<UIMessageChunk>> {
  const res = await fetchAPI(`/api/automations/runs/${encodeURIComponent(runId)}/stream`, { signal })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseUIMessageStream(res.body)
}

export async function streamAutomationRunMessage(runId: string, message: string, signal?: AbortSignal): Promise<ReadableStream<UIMessageChunk>> {
  const res = await fetchAPI(`/api/automations/runs/${encodeURIComponent(runId)}/chat/stream`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ message }),
    signal,
  })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseUIMessageStream(res.body)
}

export async function abortAutomationRun(runId: string): Promise<void> {
  await requestJSON(`/api/automations/runs/${encodeURIComponent(runId)}/abort`, { method: 'POST' })
}

export async function getAutomationRunMessages(runId: string): Promise<AgentUIMessage[]> {
  return requestJSON(`/api/automations/runs/${encodeURIComponent(runId)}/messages`)
}
