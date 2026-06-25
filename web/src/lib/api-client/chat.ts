import { fetchAPI, jsonHeaders, parseSSEStream, requestJSON } from './client'
import type { AgentRunTrace, AgentRunTraceSummary, ChatMessage, ContextAnalysis, SSEEvent, SessionSummary, TextSelection } from './types'

export async function sendMessage(
  message: string,
  references: string[] = [],
  loreReferences: string[] = [],
  styleScenes: string[] = [],
  textSelections: TextSelection[] = [],
  signal?: AbortSignal,
  planMode?: boolean,
  writingSkill?: string,
): Promise<ReadableStream<SSEEvent>> {
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
      plan_mode: planMode || false,
      writing_skill: writingSkill || undefined,
    }),
    signal,
  })

  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')

  return parseSSEStream(res.body)
}

export async function analyzeChatContext(
  message: string,
  references: string[] = [],
  loreReferences: string[] = [],
  styleScenes: string[] = [],
  textSelections: TextSelection[] = [],
  planMode?: boolean,
  writingSkill?: string,
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
      plan_mode: planMode || false,
      writing_skill: writingSkill || undefined,
    }),
  })
}

export async function compactChatContext(): Promise<void> {
  await requestJSON('/api/chat/context-compaction', { method: 'POST' })
}

export async function removeChatContextCompaction(): Promise<boolean> {
  const data = await requestJSON<{ removed?: boolean }>('/api/chat/context-compaction/active', { method: 'DELETE' })
  return Boolean(data.removed)
}

export async function getActiveChatTask(): Promise<{ active: boolean; status?: string }> {
  return requestJSON('/api/chat/active')
}

export async function streamActiveChat(signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetchAPI('/api/chat/stream', { signal })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
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

export async function getMessages(sessionId?: string): Promise<ChatMessage[]> {
  const query = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : ''
  return requestJSON(`/api/session/messages${query}`)
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
