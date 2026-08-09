import { jsonHeaders, requestJSON } from './client'
import type { AutomationExecutionTarget, AutomationInboxActionResult, AutomationInboxItem, AutomationRunRecord, AutomationTask, AutomationTaskTemplate, AutomationTaskUpdate, AutomationTriggerEvidence } from './types'

export async function getAutomations(target: AutomationExecutionTarget): Promise<AutomationTask[]> {
  const data = await requestJSON<{ tasks: AutomationTask[] }>(`/api/automations?${automationProjectQuery(target)}`)
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

export async function getAutomationInbox(target: AutomationExecutionTarget): Promise<AutomationInboxItem[]> {
  const data = await requestJSON<{ items: AutomationInboxItem[] }>(`/api/automations/inbox?${automationProjectQuery(target)}`)
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

export async function updateAutomation(id: string, task: AutomationTaskUpdate, baseRevision?: string): Promise<AutomationTask> {
  return requestJSON(`/api/automations/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ ...task, base_revision: baseRevision }),
  })
}

export async function deleteAutomation(id: string): Promise<void> {
  await requestJSON(`/api/automations/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function startAutomationRun(
  id: string,
  commandId: string,
  triggerEvidence: AutomationTriggerEvidence[] = [],
): Promise<AutomationRunRecord> {
  const data = await requestJSON<{ run: AutomationRunRecord }>(`/api/automations/${encodeURIComponent(id)}/run`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ command_id: commandId, trigger_evidence: triggerEvidence }),
  })
  return data.run
}

function automationProjectQuery(target: AutomationExecutionTarget): string {
  if (target.kind !== 'workspace' || (!target.project_id?.trim() && !target.workspace?.trim())) {
    throw new Error('自动化需要当前项目 / Automation requires a current Project')
  }
  const params = new URLSearchParams()
  if (target.project_id) params.set('project_id', target.project_id)
  if (target.workspace) params.set('workspace', target.workspace)
  return params.toString()
}
