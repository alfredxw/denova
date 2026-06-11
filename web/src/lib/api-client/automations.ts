import { jsonHeaders, requestJSON } from './client'
import type { AutomationRunResult, AutomationTask } from './types'

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
