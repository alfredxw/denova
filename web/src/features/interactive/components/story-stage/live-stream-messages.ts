import type { AgentMessageMetadata, AgentUIMessage } from '@/lib/agent-ui'
import { agentMessageDisplayText, createAgentReasoningMessage, createAgentTextMessage } from '@/lib/agent-ui-message'
import { readToolPresentation } from '@/lib/tool-presentation'

export type BufferedLiveMessage = {
  id?: string
  role: 'assistant' | 'reasoning'
  content: string
  metadata: AgentMessageMetadata
}

export function appendBufferedLiveMessage(messages: AgentUIMessage[], { id, role, content, metadata }: BufferedLiveMessage) {
  if (!content) return messages
  const last = messages[messages.length - 1]
  if (last && isStreamingMessageKind(last, role) && sameLiveMessageSource(last.metadata, metadata)) {
    return [
      ...messages.slice(0, -1),
      {
        ...last,
        metadata: {
          ...last.metadata,
          streaming_target_content: `${agentMessageDisplayText(last)}${content}`,
        },
      },
    ]
  }
  const nextMetadata = { ...metadata, streaming_target_content: content }
  if (role === 'assistant') {
    return [...messages, createAgentTextMessage({ id, role: 'assistant', text: '', state: 'streaming', metadata: nextMetadata })]
  }
  return [...messages, createAgentReasoningMessage({ id, text: '', state: 'streaming', metadata: nextMetadata })]
}

export function promoteMessageTargets(messages: AgentUIMessage[]) {
  let changed = false
  const nextMessages = messages.map((message) => {
    if (message.metadata?.streaming_target_content === undefined) return message
    changed = true
    return promoteMessageTarget(message)
  })
  return changed ? nextMessages : messages
}

export function promoteMessageTarget(message: AgentUIMessage): AgentUIMessage {
  const target = message.metadata?.streaming_target_content
  if (target === undefined) return message
  const { streaming_target_content: _target, ...metadata } = message.metadata || {}
  return {
    ...message,
    metadata,
    parts: message.parts.map((part) => part.type === 'text' || part.type === 'reasoning'
      ? { ...part, text: target }
      : part),
  } as AgentUIMessage
}

export function streamMetadataFromPayload(payload: Record<string, unknown>): AgentMessageMetadata {
  const runPath = Array.isArray(payload.run_path) ? payload.run_path.filter((item): item is string => typeof item === 'string') : undefined
  return {
    run_id: typeof payload.run_id === 'string' ? payload.run_id : undefined,
    display_segment_id: typeof payload.display_segment_id === 'string' ? payload.display_segment_id : undefined,
    display_phase: readStreamDisplayPhase(payload.display_phase),
    agent_kind: typeof payload.agent_kind === 'string' ? payload.agent_kind : undefined,
    agent_name: typeof payload.agent_name === 'string' ? payload.agent_name : undefined,
    root_agent_name: typeof payload.root_agent_name === 'string' ? payload.root_agent_name : undefined,
    run_path: runPath,
    subagent: readStreamBool(payload.subagent),
    subagent_session_id: typeof payload.subagent_session_id === 'string' ? payload.subagent_session_id : undefined,
    subagent_type: typeof payload.subagent_type === 'string' ? payload.subagent_type : undefined,
		parent_call_id: typeof payload.parent_call_id === 'string' ? payload.parent_call_id : undefined,
    tool_presentation: readToolPresentation(payload.tool_presentation),
  }
}

function readStreamDisplayPhase(value: unknown): AgentMessageMetadata['display_phase'] {
  return value === 'candidate' || value === 'progress' || value === 'final' || value === 'partial' ? value : undefined
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
  messages: AgentUIMessage[],
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

function findToolMessageIndex(messages: AgentUIMessage[], id?: string, name?: string) {
  if (id) {
    for (let i = messages.length - 1; i >= 0; i--) {
      const tool = toolPart(messages[i])
      if (tool && (tool.toolCallId === id || messages[i].id === id)) return i
    }
    return -1
  }
  if (name) {
    let match = -1
    for (let i = messages.length - 1; i >= 0; i--) {
      const tool = toolPart(messages[i])
      if (!tool || tool.toolName !== name) continue
      if (match >= 0) return -1
      match = i
    }
    return match
  }
  for (let i = messages.length - 1; i >= 0; i--) {
    if (toolPart(messages[i])) return i
  }
  return -1
}

export function updateToolMessageInput(message: AgentUIMessage, input: unknown): AgentUIMessage {
  return updateToolPart(message, (part) => ({ ...part, input }))
}

export function completeToolMessage(message: AgentUIMessage, result: string): AgentUIMessage {
  return updateToolPart(message, (part) => ({ ...part, state: 'output-available', output: result }))
}

export function toolMessageInputText(message: AgentUIMessage) {
  const input = toolPart(message)?.input
  if (typeof input === 'string') return input
  if (input === undefined) return ''
  try {
    return JSON.stringify(input)
  } catch {
    return String(input)
  }
}

function updateToolPart(message: AgentUIMessage, update: (part: Record<string, unknown>) => Record<string, unknown>): AgentUIMessage {
  return {
    ...message,
    parts: message.parts.map((part) => part.type === 'dynamic-tool'
      ? update(part as unknown as Record<string, unknown>) as AgentUIMessage['parts'][number]
      : part),
  }
}

function toolPart(message: AgentUIMessage): Record<string, unknown> | undefined {
  return message.parts.find((part) => part.type === 'dynamic-tool') as unknown as Record<string, unknown> | undefined
}

function isStreamingMessageKind(message: AgentUIMessage, role: BufferedLiveMessage['role']) {
  const type = role === 'assistant' ? 'text' : 'reasoning'
  return message.parts.some((part) => part.type === type && 'state' in part && part.state === 'streaming')
}

function sameLiveMessageSource(message: AgentMessageMetadata | undefined, metadata: AgentMessageMetadata) {
  if (Boolean(message?.subagent) !== Boolean(metadata.subagent)) return false
  if (message?.subagent || metadata.subagent) return subAgentSessionKey(message) === subAgentSessionKey(metadata)
  return true
}

function subAgentSessionKey(metadata?: AgentMessageMetadata) {
  if (!metadata?.subagent) return ''
  if (metadata.subagent_session_id) return metadata.subagent_session_id
  return [
    metadata.run_id || '',
    metadata.root_agent_name || '',
    metadata.agent_name || '',
    ...(metadata.run_path || []),
  ].filter(Boolean).join('/')
}

function readStreamBool(value: unknown) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'string') return value === 'true'
  return false
}
