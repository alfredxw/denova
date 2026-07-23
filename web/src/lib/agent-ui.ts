import type { ChatTransport, UIMessage } from 'ai'
import { DefaultChatTransport } from 'ai'
import { fetchAPI } from './api-client/client'
import type { ChatMessage, UserMessageReference } from './api-client/types'

export interface AgentMessageMetadata {
  created_at?: string
  display_role?: ChatMessage['role']
  history_type?: string
  run_id?: string
  agent_kind?: string
  agent_name?: string
  root_agent_name?: string
  run_path?: string[]
  subagent?: boolean
  subagent_session_id?: string
  subagent_type?: string
  sse_hidden_fields?: string[]
  sse_hidden_reason?: string
  sse_display_notice?: string
  sse_generated_chars?: number
  display_hidden?: boolean
  streaming_target_content?: string
  turn_id?: string
  navigation_turn_id?: string
  turn_versions?: { turn_id: string; ts: string; current?: boolean }[]
  turn_version_index?: number
  user_references?: UserMessageReference[]
}

type AgentDataPayload = Record<string, unknown>

export type AgentDataParts = {
  'agent-activity': AgentDataPayload
  'agent-clear': AgentDataPayload
  'agent-context-compaction': AgentDataPayload
  'agent-error': AgentDataPayload
  'agent-interactive-image': AgentDataPayload
  'agent-plan-question': AgentDataPayload
  'agent-proposed-plan': AgentDataPayload
  'agent-rule-roll': AgentDataPayload
  'agent-system': AgentDataPayload
  'agent-token-usage': AgentDataPayload
  'agent-tool-result': AgentDataPayload
  'agent-workspace-change': AgentDataPayload
}

export type AgentUIMessage = UIMessage<AgentMessageMetadata, AgentDataParts>

interface AgentChatRequestBody {
  references?: string[]
  lore_references?: string[]
  style_scenes?: string[]
  selections?: Array<{ file_name: string; start_line: number; end_line: number; content: string }>
  ide_context?: { current_file?: string; open_files?: string[] }
  plan_mode?: boolean
  writing_skill?: string
  image_preset_id?: string
  teller_id?: string
  review_feedback?: Array<{
    source?: 'workspace_change' | 'document'
    review_thread_id: string
    comment_ids: string[]
  }>
}

export class AgentChatTransport implements ChatTransport<AgentUIMessage> {
  private readonly transport: DefaultChatTransport<AgentUIMessage>

  constructor() {
    this.transport = new DefaultChatTransport<AgentUIMessage>({
      api: '/api/chat',
      fetch: fetchAPI,
      prepareSendMessagesRequest: ({ messages, body }) => ({
        body: {
          ...(body || {}),
          message: bodyMessage(body) || latestUserText(messages),
        },
      }),
      prepareReconnectToStreamRequest: () => ({
        api: '/api/chat/stream',
      }),
    })
  }

  sendMessages(options: Parameters<ChatTransport<AgentUIMessage>['sendMessages']>[0]) {
    return this.transport.sendMessages(options)
  }

  reconnectToStream(options: Parameters<ChatTransport<AgentUIMessage>['reconnectToStream']>[0]) {
    return this.transport.reconnectToStream(options)
  }
}

export function buildAgentChatRequestBody(body: AgentChatRequestBody): AgentChatRequestBody {
  const reviewFeedback = normalizeReviewFeedbackRefs(body.review_feedback)
  return {
    references: body.references || [],
    lore_references: body.lore_references || [],
    style_scenes: body.style_scenes || [],
    selections: body.selections || [],
    ide_context: body.ide_context,
    plan_mode: body.plan_mode || false,
    writing_skill: body.writing_skill || undefined,
    image_preset_id: body.image_preset_id || undefined,
    teller_id: body.teller_id || undefined,
    review_feedback: reviewFeedback.length ? reviewFeedback : undefined,
  }
}

function normalizeReviewFeedbackRefs(feedback: AgentChatRequestBody['review_feedback']): NonNullable<AgentChatRequestBody['review_feedback']> {
  const merged = new Map<string, NonNullable<AgentChatRequestBody['review_feedback']>[number]>()
  for (const selection of feedback ?? []) {
    const reviewThreadID = selection.review_thread_id.trim()
    const commentIDs = selection.comment_ids.map((id) => id.trim()).filter(Boolean)
    if (!reviewThreadID || !commentIDs.length) continue
    const source = selection.source || 'workspace_change'
    const key = `${source}\u0000${reviewThreadID}`
    const current = merged.get(key)
    merged.set(key, {
      ...(selection.source ? { source: selection.source } : {}),
      review_thread_id: reviewThreadID,
      comment_ids: Array.from(new Set([...(current?.comment_ids ?? []), ...commentIDs])),
    })
  }
  return [...merged.values()]
}

export function normalizeAgentUIMessages(messages: AgentUIMessage[]): AgentUIMessage[] {
  return normalizeRepeatedAgentUIParts(normalizeRepeatedAgentUIMessageIDs(messages))
}

function normalizeRepeatedAgentUIMessageIDs(messages: AgentUIMessage[]) {
  const indexByKey = new Map<string, number>()
  const normalized: AgentUIMessage[] = []
  for (const message of messages) {
    const key = message.id || `${message.role}:${normalized.length}`
    const existingIndex = indexByKey.get(key)
    if (existingIndex !== undefined) {
      normalized[existingIndex] = message
      continue
    }
    indexByKey.set(key, normalized.length)
    normalized.push(message)
  }
  return normalized
}

function normalizeRepeatedAgentUIParts(messages: AgentUIMessage[]) {
  const normalized = messages.map(message => ({ ...message, parts: [...message.parts] })) as AgentUIMessage[]
  const locationByKey = new Map<string, { messageIndex: number; partIndex: number }>()
  const removed = new Set<string>()

  normalized.forEach((message, messageIndex) => {
    message.parts.forEach((part, partIndex) => {
      const key = agentUIPartDedupeKey(message, part)
      if (!key) return
      const existing = locationByKey.get(key)
      if (!existing) {
        locationByKey.set(key, { messageIndex, partIndex })
        return
      }
      const existingMessage = normalized[existing.messageIndex]
      const existingPart = existingMessage.parts[existing.partIndex]
      if (!canMergeAgentUIParts(existingPart, part)) {
        locationByKey.set(`${key}#${messageIndex}:${partIndex}`, { messageIndex, partIndex })
        return
      }
      existingMessage.parts[existing.partIndex] = mergeDuplicateAgentUIPart(existingMessage.parts[existing.partIndex], part)
      existingMessage.metadata = mergeAgentMessageMetadata(existingMessage.metadata, message.metadata)
      removed.add(`${messageIndex}:${partIndex}`)
    })
  })

  return normalized
    .map((message, messageIndex) => ({
      ...message,
      parts: message.parts.filter((_part, partIndex) => !removed.has(`${messageIndex}:${partIndex}`)),
    }) as AgentUIMessage)
    .filter(message => message.parts.length > 0)
}

function agentUIPartDedupeKey(message: AgentUIMessage, part: AgentUIMessage['parts'][number]) {
  const raw = part as Record<string, unknown>
  const type = readString(raw.type)
  if (!type) return ''
  const metadata = agentPartMetadata(message, raw)
  const runID = firstNonEmpty(metadata.run_id || '', readString(objectData(raw.data).run_id))

  if (type === 'dynamic-tool' || type.startsWith('tool-')) {
    const toolCallID = readString(raw.toolCallId)
    if (!toolCallID) return ''
    return scopedAgentPartKey(runID, `tool:${toolCallID}`)
  }

  if (isAgentDataPartType(type)) {
    const data = objectData(raw.data)
    const id = firstNonEmpty(readString(raw.id), readString(data.id))
    if (id) return scopedAgentPartKey(runID, `data:${type}:${id}`)
    if (runID && (type === 'data-agent-token-usage' || type === 'data-agent-context-compaction')) {
      return `run:${runID}:data:${type}`
    }
    return ''
  }

  if ((type === 'text' || type === 'reasoning') && runID) {
    const text = readString(raw.text).trim()
    if (!text) return ''
    const fingerprint = type === 'reasoning'
      ? contentPrefixFingerprint(text)
      : textFingerprint(text)
    return `run:${runID}:content:${type}:${fingerprint}`
  }

  return ''
}

function agentPartMetadata(message: AgentUIMessage, raw: Record<string, unknown>): AgentMessageMetadata {
  return {
    ...(message.metadata || {}),
    ...agentMetadataFromProvider(raw.providerMetadata),
    ...agentMetadataFromProvider(raw.callProviderMetadata),
  }
}

function agentMetadataFromProvider(metadata: unknown): AgentMessageMetadata {
  if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) return {}
  const agent = (metadata as Record<string, unknown>).agent
  const raw = agent && typeof agent === 'object' && !Array.isArray(agent)
    ? agent as Record<string, unknown>
    : metadata as Record<string, unknown>
  return {
    run_id: readString(raw.run_id) || undefined,
    agent_kind: readString(raw.agent_kind) || undefined,
    agent_name: readString(raw.agent_name) || undefined,
    root_agent_name: readString(raw.root_agent_name) || undefined,
    subagent: typeof raw.subagent === 'boolean' ? raw.subagent : undefined,
    subagent_session_id: readString(raw.subagent_session_id) || undefined,
    subagent_type: readString(raw.subagent_type) || undefined,
  }
}

function scopedAgentPartKey(runID: string, key: string) {
  return runID ? `run:${runID}:${key}` : key
}

function mergeDuplicateAgentUIPart(existing: AgentUIMessage['parts'][number], incoming: AgentUIMessage['parts'][number]) {
  const existingRaw = existing as Record<string, unknown>
  const incomingRaw = incoming as Record<string, unknown>
  const type = readString(incomingRaw.type)
  if (type === 'dynamic-tool' || type.startsWith('tool-')) {
    const incomingRank = toolPartStateRank(readString(incomingRaw.state))
    const existingRank = toolPartStateRank(readString(existingRaw.state))
    if (incomingRank > existingRank) return incoming
    if (incomingRank < existingRank) return existing
    return toolPartPayloadScore(incomingRaw) >= toolPartPayloadScore(existingRaw)
      ? incoming
      : existing
  }
  if (isAgentDataPartType(type)) {
    const incomingStatus = readString(objectData(incomingRaw.data).status)
    const existingStatus = readString(objectData(existingRaw.data).status)
    const incomingRank = dataPartStatusRank(incomingStatus)
    const existingRank = dataPartStatusRank(existingStatus)
    if (incomingRank > existingRank) return incoming
    if (incomingRank < existingRank) return existing
    return dataPartPayloadScore(incomingRaw) >= dataPartPayloadScore(existingRaw)
      ? incoming
      : existing
  }
  if (type === 'text' || type === 'reasoning') {
    const incomingText = readString(incomingRaw.text)
    const existingText = readString(existingRaw.text)
    if (incomingText.length > existingText.length) return incoming
    if (incomingText.length < existingText.length) return existing
    const incomingDone = readString(incomingRaw.state) === 'done'
    const existingDone = readString(existingRaw.state) === 'done'
    if (incomingDone && !existingDone) return incoming
    if (!incomingDone && existingDone) return existing
    return incoming
  }
  return incoming
}

function canMergeAgentUIParts(existing: AgentUIMessage['parts'][number], incoming: AgentUIMessage['parts'][number]) {
  const existingRaw = existing as Record<string, unknown>
  const incomingRaw = incoming as Record<string, unknown>
  const existingType = readString(existingRaw.type)
  const incomingType = readString(incomingRaw.type)
  if (existingType !== incomingType) return false
  if (incomingType !== 'text' && incomingType !== 'reasoning') return true
  const existingText = readString(existingRaw.text).trim()
  const incomingText = readString(incomingRaw.text).trim()
  if (!existingText || !incomingText) return true
  return existingText === incomingText
    || existingText.startsWith(incomingText)
    || incomingText.startsWith(existingText)
}

function mergeAgentMessageMetadata(left?: AgentMessageMetadata, right?: AgentMessageMetadata): AgentMessageMetadata | undefined {
  if (!left) return right
  if (!right) return left
  return { ...left, ...right }
}

function toolPartStateRank(state: string) {
  if (state === 'output-available' || state === 'output-error' || state === 'output-denied') return 4
  if (state === 'approval-responded') return 3
  if (state === 'input-available') return 2
  if (state === 'approval-requested' || state === 'input-streaming') return 1
  return 0
}

function dataPartStatusRank(status: string) {
  if (status === 'success' || status === 'error') return 2
  if (status === 'running') return 1
  return 0
}

function toolPartPayloadScore(raw: Record<string, unknown>) {
  return valueScore(raw.input)
    + valueScore(raw.output)
    + readString(raw.errorText).length
    + readString(raw.state).length
}

function dataPartPayloadScore(raw: Record<string, unknown>) {
  const data = objectData(raw.data)
  return readString(raw.id).length
    + readString(raw.type).length
    + readString(data.content).length
    + readString(data.message).length
    + readString(data.error).length
    + readString(data.status).length
    + valueScore(data.result)
    + valueScore(data.plan_action)
}

function valueScore(value: unknown) {
  if (value === null || value === undefined) return 0
  if (typeof value === 'string') return value.length
  try {
    const serialized = JSON.stringify(value)
    return serialized ? serialized.length : 1
  } catch {
    return 1
  }
}

function textFingerprint(value: string) {
  let hash = 0
  for (let index = 0; index < value.length; index += 1) {
    hash = ((hash * 31) + value.charCodeAt(index)) | 0
  }
  return `${value.length}:${(hash >>> 0).toString(36)}`
}

function contentPrefixFingerprint(value: string) {
  const prefixRunes = 24
  const runes = [...value]
  const prefix = runes.length > prefixRunes ? runes.slice(0, prefixRunes).join('') : value
  return textFingerprint(prefix)
}

function bodyMessage(body: Record<string, any> | undefined) {
  const message = body?.message
  return typeof message === 'string' ? message : ''
}

function latestUserText(messages: AgentUIMessage[]) {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index]
    if (message.role !== 'user') continue
    const text = message.parts.map(part => part.type === 'text' ? part.text : '').join('').trim()
    if (text) return text
  }
  return ''
}

function objectData(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function isAgentDataPartType(type: string): type is `data-agent-${string}` {
  return type.startsWith('data-agent-')
}

function firstNonEmpty(...values: Array<string | undefined>) {
  return values.find(value => value && value.trim()) || ''
}
