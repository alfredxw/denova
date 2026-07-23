import type { UIMessageChunk } from 'ai'
import { fetchAPI, jsonHeaders, parseUIMessageStream, readErrorMessage, requestJSON } from './client'
import type { AgentRunTrace, AgentRunTraceSummary, ContextAnalysis, IDEContext, SessionSummary, TextSelection } from './types'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { isKnownAgentCommandOutcome } from '@/lib/agent-command'

const chatStructuralCommandIDs = new Map<string, string>()

export interface AgentRunTraceExportFile {
  filename: string
  blob: Blob
}

export async function sendMessage(
  message: string,
  references: string[] = [],
  loreReferences: string[] = [],
  styleScenes: string[] = [],
  textSelections: TextSelection[] = [],
  signal?: AbortSignal,
  planMode?: boolean,
  writingSkill?: string,
  ideContext?: IDEContext,
  imagePresetId?: string,
  tellerId?: string,
  commandId: string = createAgentCommandID(),
): Promise<ReadableStream<UIMessageChunk>> {
  const res = await fetchAPI('/api/chat', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      command_id: commandId,
      message,
      references,
      lore_references: loreReferences,
      style_scenes: styleScenes,
      selections: textSelections.map((s) => ({
        file_name: s.fileName,
        start_line: s.startLine,
        end_line: s.endLine,
        content: s.content,
      })),
      ide_context: normalizeIDEContext(ideContext),
      plan_mode: planMode || false,
      writing_skill: writingSkill || undefined,
      image_preset_id: imagePresetId || undefined,
      teller_id: tellerId || undefined,
    }),
    signal,
  })

  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')

  return parseUIMessageStream(res.body)
}

export async function analyzeChatContext(
  message: string,
  references: string[] = [],
  loreReferences: string[] = [],
  styleScenes: string[] = [],
  textSelections: TextSelection[] = [],
  planMode?: boolean,
  writingSkill?: string,
  ideContext?: IDEContext,
  imagePresetId?: string,
  tellerId?: string,
): Promise<ContextAnalysis> {
  return requestJSON('/api/chat/context-analysis', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      message,
      references,
      lore_references: loreReferences,
      style_scenes: styleScenes,
      selections: textSelections.map((s) => ({
        file_name: s.fileName,
        start_line: s.startLine,
        end_line: s.endLine,
        content: s.content,
      })),
      ide_context: normalizeIDEContext(ideContext),
      plan_mode: planMode || false,
      writing_skill: writingSkill || undefined,
      image_preset_id: imagePresetId || undefined,
      teller_id: tellerId || undefined,
    }),
  })
}

function normalizeIDEContext(context?: IDEContext) {
  if (!context?.currentFile && !context?.openFiles?.length) return undefined
  return {
    current_file: context.currentFile || undefined,
    open_files: context.openFiles?.length ? context.openFiles : undefined,
  }
}

export async function removeChatContextCompaction(): Promise<boolean> {
  const key = 'remove-active-compaction'
  const commandId = chatStructuralCommandIDs.get(key) ?? createAgentCommandID()
  chatStructuralCommandIDs.set(key, commandId)
  try {
    const data = await requestJSON<{ removed?: boolean }>(
      `/api/chat/context-compaction/active?command_id=${encodeURIComponent(commandId)}`,
      { method: 'DELETE' },
    )
    chatStructuralCommandIDs.delete(key)
    return Boolean(data.removed)
  } catch (error) {
    if (isKnownAgentCommandOutcome(error)) chatStructuralCommandIDs.delete(key)
    throw error
  }
}

export type AgentCommandDelivery = 'follow_up' | 'steer'

export type AgentRuntimeRecoveryActionKind =
  'start_turn' | 'steer' | 'follow_up' | 'next_turn' | 'compact_context' | 'remove_compaction' | 'abort'

/** Public, payload-free identity selected from the server recovery projection. */
export interface AgentRuntimeRecoveryAction {
  kind: AgentRuntimeRecoveryActionKind
  command_id: string
  operation_id: string
}

export interface AgentRuntimeActiveOutput {
  operation_id: string
  cycle: number
  content: string
  thinking: string
  content_truncated?: boolean
  thinking_truncated?: boolean
}

export interface AgentRuntimeQueuedCommand {
  command_id: string
  operation_id: string
  delivery: AgentCommandDelivery
  message: string
  message_truncated?: boolean
}

export interface AgentRuntimeOpenTool {
  call_id: string
  name: string
  operation_id: string
  cycle: number
}

export interface AgentRuntimeOperation {
  operation_id: string
  command_id: string
  status: 'succeeded' | 'failed' | 'aborted' | 'interrupted' | string
  reason?: string
  reason_truncated?: boolean
}

export interface ActiveChatTask {
  active: boolean
  status?: string
  /** Exact backend display-stream identity. */
  task_id?: string
  /** Diagnostic-only latest server display cursor. Recovery uses only a checkpoint cursor delivered by the stream. */
  stream_cursor?: number
  /** Durable Agent Runtime journal cursor. Never use this as SSE `after`. */
  cursor?: number
  phase?: 'idle' | 'running' | 'settling' | 'failed' | string
  recovery_paused?: boolean
  runtime_recoverable?: boolean
  stream_attached?: boolean
  recovery_actions?: AgentRuntimeRecoveryAction[]
  active_operation_id?: string
  active_cycle?: number
  active_output?: AgentRuntimeActiveOutput
  queue?: AgentRuntimeQueuedCommand[]
  open_tools?: AgentRuntimeOpenTool[]
  last_operation?: AgentRuntimeOperation
}

export interface AgentCommandReceipt {
  command_id: string
  operation_id: string
  cursor: number
}

export interface AgentRuntimeRecoveryReceipt {
  task_id: string
  status: string
  stream_cursor: number
  cursor: number
  replayed: boolean
  recovery_action: AgentRuntimeRecoveryAction
}

/** Generate the idempotency key reused by one logical command submission. */
export function createAgentCommandID(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') return globalThis.crypto.randomUUID()
  return `agent-command-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

export async function getActiveChatTask(): Promise<ActiveChatTask> {
  return requestJSON('/api/chat/active')
}

/** Resume only the exact payload-free action selected by the backend. */
export function recoverChatAgentRuntime(action: AgentRuntimeRecoveryAction): Promise<AgentRuntimeRecoveryReceipt> {
  return requestJSON('/api/chat/recovery', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ action }),
  })
}

/** Submit a command to the exact operation currently shown by the client. */
export async function submitChatCommand(
  type: AgentCommandDelivery | 'abort',
  commandId: string,
  targetOperationId: string,
  input?: Record<string, unknown>,
  reason?: string,
): Promise<AgentCommandReceipt> {
  return requestJSON('/api/chat/commands', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      type,
      command_id: commandId,
      target_operation_id: targetOperationId,
      ...(input ? { input } : {}),
      ...(reason ? { reason } : {}),
    }),
  })
}

export async function executeCommand(command: string): Promise<string> {
  const data = await requestJSON<{ result?: string }>('/api/command', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ command }),
  })
  return data.result || ''
}

export async function getMessages(sessionId?: string): Promise<AgentUIMessage[]> {
  const query = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : ''
  return requestJSON(`/api/session/messages${query}`)
}

export const DEFAULT_SESSION_MESSAGE_PAGE_SIZE = 100

export interface SessionMessagesPage {
  messages: AgentUIMessage[]
  nextBefore: string
  hasMore: boolean
  total: number
}

export async function getMessagesPage(sessionId?: string, options: { limit?: number; before?: string } = {}): Promise<SessionMessagesPage> {
  const query = new URLSearchParams()
  if (sessionId) query.set('session_id', sessionId)
  query.set('limit', String(options.limit || DEFAULT_SESSION_MESSAGE_PAGE_SIZE))
  if (options.before) query.set('before', options.before)
  const data = await requestJSON<
    | AgentUIMessage[]
    | {
        messages?: AgentUIMessage[]
        page?: { next_before?: string; has_more?: boolean; total?: number }
      }
  >(`/api/session/messages?${query.toString()}`)
  if (Array.isArray(data)) {
    return {
      messages: data,
      nextBefore: '0',
      hasMore: false,
      total: data.length,
    }
  }
  return {
    messages: data.messages || [],
    nextBefore: data.page?.next_before || '0',
    hasMore: data.page?.has_more === true,
    total: data.page?.total || 0,
  }
}

export async function getSessions(): Promise<SessionSummary[]> {
  const data = await requestJSON<{ sessions: SessionSummary[] }>('/api/sessions')
  return data.sessions || []
}

export async function getAgentRunTraces(limit = 20): Promise<AgentRunTraceSummary[]> {
  const data = await requestJSON<{ runs: AgentRunTraceSummary[] }>(`/api/agent-runs?limit=${encodeURIComponent(String(limit))}`)
  return data.runs || []
}

export async function getAgentRunTrace(id: string): Promise<AgentRunTrace> {
  return requestJSON(`/api/agent-runs/${encodeURIComponent(id)}`)
}

export async function exportAgentRunTrace(id: string): Promise<AgentRunTraceExportFile> {
  const res = await fetchAPI(`/api/agent-runs/${encodeURIComponent(id)}/export`)
  if (!res.ok) throw new Error(await readErrorMessage(res))
  return {
    filename: `${id}.jsonl`,
    blob: await res.blob(),
  }
}

export function downloadAgentRunTrace(file: AgentRunTraceExportFile) {
  const href = URL.createObjectURL(file.blob)
  const link = document.createElement('a')
  link.href = href
  link.download = file.filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(href)
}

export async function createSession(title?: string): Promise<SessionSummary> {
  return requestJSON('/api/sessions', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ title: title ?? '' }),
  })
}

export async function switchSession(id: string): Promise<SessionSummary> {
  return requestJSON('/api/sessions/switch', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ id }),
  })
}

export async function renameSession(id: string, title: string): Promise<void> {
  await requestJSON('/api/sessions/rename', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ id, title }),
  })
}

export async function deleteSession(id: string): Promise<SessionSummary> {
  return requestJSON('/api/sessions/delete', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ id }),
  })
}
