import { jsonHeaders, requestJSON } from './client'

const ROOT = '/api/continual-learning'

export interface HarnessStateFile {
  path: string
  content: string
}

export interface HarnessStateSnapshot {
  revision: string
  published_revision: string
  files: HarnessStateFile[]
  source: 'user' | 'builtin'
  changed: boolean
  script_tools?: ScriptToolMetadata[]
  diagnostics?: HarnessStateDiagnostic[]
}

export interface HarnessStateDiagnostic {
  code: string
  path?: string
  line?: number
  column?: number
  message: string
}

export interface ScriptToolMetadata {
  name: string
  description: string
  agents: string[]
  enabled: boolean
  resource: string
  input_schema: Record<string, unknown>
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
  revision: string
  changed: boolean
}

export interface HarnessStatePublishResult {
  version?: HarnessStateVersion
  draft_revision: string
  published_revision: string
  changed: boolean
}

export interface HarnessDebugResult {
  revision: string
  agent_kind: string
  prompt_resource?: string
  contexts: Array<{ id: string; purpose: string; resource: string }>
  script_tools: ScriptToolMetadata[]
  subagents: Array<{ id: string; name: string; description: string; resource: string }>
  tool_descriptions: string[]
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

export function publishHarnessState(request: {
  draft_revision: string
  published_revision: string
  summary: string
}): Promise<HarnessStatePublishResult> {
  return requestJSON(`${ROOT}/publish`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(request),
  })
}

export function debugHarnessState(agentKind: string, revision: string): Promise<HarnessDebugResult> {
  const params = new URLSearchParams({ agent_kind: agentKind, revision })
  return requestJSON(`${ROOT}/debug?${params.toString()}`)
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
