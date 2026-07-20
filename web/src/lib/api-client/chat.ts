import type { UIMessageChunk } from 'ai'
import { fetchAPI, jsonHeaders, parseUIMessageStream, requestJSON } from './client'
import type { AgentRunTrace, AgentRunTraceSummary, ContextAnalysis, IDEContext, SessionSummary, TextSelection } from './types'
import type { AgentUIMessage } from '@/lib/agent-ui'

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
): Promise<ReadableStream<UIMessageChunk>> {
  const res = await fetchAPI('/api/chat', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      message,
      references,
      lore_references: loreReferences,
      style_scenes: styleScenes,
      selections: textSelections.map(s => ({
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
      selections: textSelections.map(s => ({
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
  const data = await requestJSON<{ removed?: boolean }>('/api/chat/context-compaction/active', { method: 'DELETE' })
  return Boolean(data.removed)
}

export async function getActiveChatTask(): Promise<{ active: boolean; status?: string }> {
  return requestJSON('/api/chat/active')
}

export async function streamActiveChat(signal?: AbortSignal): Promise<ReadableStream<UIMessageChunk>> {
  const res = await fetchAPI('/api/chat/stream', { signal })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseUIMessageStream(res.body)
}

export async function abortChat(): Promise<void> {
  await requestJSON('/api/chat/abort', { method: 'POST' })
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
  const data = await requestJSON<AgentUIMessage[] | {
    messages?: AgentUIMessage[]
    page?: { next_before?: string; has_more?: boolean; total?: number }
  }>(`/api/session/messages?${query.toString()}`)
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
