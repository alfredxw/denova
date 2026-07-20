import { subAgentSessionKey } from '@/components/Chat/subagent-session'
import type { ChatMessage } from '@/lib/api'

export type BufferedLiveMessage = {
  role: 'assistant' | 'thinking'
  content: string
  metadata: Partial<ChatMessage>
}

export function appendBufferedLiveMessage(messages: ChatMessage[], { role, content, metadata }: BufferedLiveMessage) {
  if (!content) return messages
  const last = messages[messages.length - 1]
  if (role === 'assistant' && last?.role === 'assistant' && last.streaming && sameLiveMessageSource(last, metadata)) {
    return [...messages.slice(0, -1), { ...last, streaming_target_content: `${last.streaming_target_content || last.content || ''}${content}` }]
  }
  if (role === 'thinking' && last?.role === 'thinking' && sameLiveMessageSource(last, metadata)) {
    return [
      ...messages.slice(0, -1),
      { ...last, streaming_target_content: `${last.streaming_target_content || last.content || ''}${content}`, streaming: true },
    ]
  }
  if (role === 'assistant') {
    return [...messages, { role, content: '', streaming_target_content: content, streaming: true, ...metadata }]
  }
  return [...messages, { role, content: '', streaming_target_content: content, streaming: true, ...metadata }]
}

export function promoteMessageTargets(messages: ChatMessage[]) {
  let changed = false
  const nextMessages = messages.map((message) => {
    if (message.streaming_target_content === undefined) return message
    changed = true
    return promoteMessageTarget(message)
  })
  return changed ? nextMessages : messages
}

export function promoteMessageTarget(message: ChatMessage): ChatMessage {
  if (message.streaming_target_content === undefined) return message
  const { streaming_target_content, ...rest } = message
  return { ...rest, content: streaming_target_content }
}

export function streamMetadataFromPayload(payload: Record<string, unknown>): Partial<ChatMessage> {
  const runPath = Array.isArray(payload.run_path) ? payload.run_path.filter((item): item is string => typeof item === 'string') : undefined
  return {
    run_id: typeof payload.run_id === 'string' ? payload.run_id : undefined,
    agent_name: typeof payload.agent_name === 'string' ? payload.agent_name : undefined,
    root_agent_name: typeof payload.root_agent_name === 'string' ? payload.root_agent_name : undefined,
    run_path: runPath,
    subagent: readStreamBool(payload.subagent),
    subagent_session_id: typeof payload.subagent_session_id === 'string' ? payload.subagent_session_id : undefined,
    subagent_type: typeof payload.subagent_type === 'string' ? payload.subagent_type : undefined,
  }
}

export function liveToolEventKeys(payload: Record<string, unknown>) {
  const metadata = streamMetadataFromPayload(payload)
  const path = metadata.run_path?.join('/') || ''
  const source = `${metadata.subagent ? 'sub' : 'root'}:${metadata.subagent_session_id || ''}:${metadata.agent_name || ''}:${path}`
  const keys: string[] = []
  if (typeof payload.id === 'string' && payload.id) keys.push(`${source}:id:${payload.id}`)
  if (typeof payload.index === 'number') keys.push(`${source}:index:${payload.index}`)
  if (typeof payload.index === 'string' && payload.index) keys.push(`${source}:index:${payload.index}`)
  return keys
}

export function findMappedLiveToolId(keys: string[], keyToMessageId: Record<string, string>) {
  for (const key of keys) {
    if (keyToMessageId[key]) return keyToMessageId[key]
  }
  return undefined
}

export function bindLiveToolEventKeys(keys: string[], keyToMessageId: Record<string, string>, toolId: string) {
  if (keys.length === 0) return keyToMessageId
  let changed = false
  const next = { ...keyToMessageId }
  for (const key of keys) {
    if (next[key] === toolId) continue
    next[key] = toolId
    changed = true
  }
  return changed ? next : keyToMessageId
}

export function findToolMessageIndexForPayload(
  messages: ChatMessage[],
  payload: Record<string, unknown> & { id?: string; name?: string },
  keyToMessageId: Record<string, string>,
) {
  const toolKeys = liveToolEventKeys(payload)
  const mappedId = findMappedLiveToolId(toolKeys, keyToMessageId)
  if (mappedId) return findToolMessageIndex(messages, mappedId)
  if (payload.id) return findToolMessageIndex(messages, payload.id)
  if (toolKeys.length > 0) return -1
  return findToolMessageIndex(messages, undefined, payload.name)
}

function findToolMessageIndex(messages: ChatMessage[], id?: string, name?: string) {
  if (id) {
    for (let i = messages.length - 1; i >= 0; i--) {
      const message = messages[i]
      if (message.role === 'tool_call' && message.id === id) return i
    }
    return -1
  }
  if (name) {
    let match = -1
    for (let i = messages.length - 1; i >= 0; i--) {
      const message = messages[i]
      if (message.role !== 'tool_call' || message.name !== name) continue
      if (match >= 0) return -1
      match = i
    }
    return match
  }
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'tool_call') return i
  }
  return -1
}

function sameLiveMessageSource(message: ChatMessage, metadata: Partial<ChatMessage>) {
  if (Boolean(message.subagent) !== Boolean(metadata.subagent)) return false
  if (message.subagent || metadata.subagent) return subAgentSessionKey(message) === subAgentSessionKey(metadata)
  return true
}

function readStreamBool(value: unknown) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'string') return value === 'true'
  return false
}
