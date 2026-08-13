import type { ChatTransport, UIMessage } from 'ai'
import { DefaultChatTransport } from 'ai'
import { fetchAPI, responseAPIError } from './api-client/client'
import type { UserMessageReference } from './api-client/types'

export type AgentDisplayRole = 'user' | 'assistant' | 'thinking' | 'tool_call' | 'tool_result' | 'ask' | 'rule_roll' | 'context_compaction' | 'token_usage' | 'proposed_plan' | 'system' | 'error'

export interface AgentMessageMetadata {
	created_at?: string
	display_role?: AgentDisplayRole
	display_phase?: 'candidate' | 'progress' | 'final' | 'partial'
	history_type?: string
  run_id?: string
  display_segment_id?: string
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
  'agent-ask': AgentDataPayload
  'agent-clear': AgentDataPayload
  'agent-context-compaction': AgentDataPayload
  'agent-error': AgentDataPayload
  'agent-interactive-image': AgentDataPayload
  'agent-proposed-plan': AgentDataPayload
  'agent-rule-roll': AgentDataPayload
  'agent-system': AgentDataPayload
  'agent-token-usage': AgentDataPayload
  'agent-tool-result': AgentDataPayload
  'agent-workspace-change': AgentDataPayload
}

export type AgentUIMessage = UIMessage<AgentMessageMetadata, AgentDataParts>

export interface AgentChatTransportOptions {
  /** Endpoint that accepts a new turn. */
  api?: string
  /** Endpoint used to reconnect to an exact display task. */
  streamApi?: string
  /** Immutable server-owned binding echoed on every request and reconnect. */
  scope?: Record<string, string>
}

interface AgentChatRequestBody {
  command_id?: string
  references?: string[]
  lore_references?: string[]
  style_scenes?: string[]
  selections?: Array<{
    file_name: string
    start_line: number
    end_line: number
    content: string
  }>
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
  private activeStreamTaskID = ''
  private activeStreamAfter = 0
  private activeStreamScope: Record<string, string> = {}
  private readonly initialSubmissionOutcomes = new Map<string, InitialSubmissionOutcome>()

  private readonly streamApi: string
  private readonly scope: Record<string, string>

  constructor(options: AgentChatTransportOptions = {}) {
    this.streamApi = options.streamApi || '/api/chat/stream'
    this.scope = { ...(options.scope || {}) }
    this.transport = new DefaultChatTransport<AgentUIMessage>({
      api: options.api || '/api/chat',
      fetch: async (input, init) => {
        const commandID = initialCommandIDFromRequest(init)
        try {
          const response = await fetchAPI(input, init)
          if (commandID) this.initialSubmissionOutcomes.set(commandID, initialSubmissionOutcomeForStatus(response.status))
          if (!response.ok) throw await responseAPIError(response)
          return response
        } catch (error) {
          if (commandID && !this.initialSubmissionOutcomes.has(commandID)) {
            this.initialSubmissionOutcomes.set(commandID, 'uncertain')
          }
          throw error
        }
      },
      prepareSendMessagesRequest: ({ messages, body }) => ({
        body: {
          ...(body || {}),
          message: bodyMessage(body) || latestUserText(messages),
          ...this.scope,
        },
      }),
      prepareReconnectToStreamRequest: () => ({ api: this.activeStreamURL() }),
    })
  }

  sendMessages(options: Parameters<ChatTransport<AgentUIMessage>['sendMessages']>[0]) {
    // A new POST creates a new backend task. It must be rebound from `/active`
    // before any reconnect can target a stream.
    this.activeStreamTaskID = ''
    this.activeStreamAfter = 0
    this.activeStreamScope = {}
    return this.transport.sendMessages(options)
  }

  reconnectToStream(options: Parameters<ChatTransport<AgentUIMessage>['reconnectToStream']>[0]) {
    return this.transport.reconnectToStream(options)
  }

  /** Select the exact backend task and optional server-issued display cursor. */
  setActiveStreamTarget(taskID: string, after?: number, scope: Record<string, string> = {}) {
    const nextTaskID = taskID.trim()
    if (!nextTaskID) throw new Error('Cannot select an empty Agent stream task')
    const nextAfter = after ?? 0
    if (!Number.isSafeInteger(nextAfter) || nextAfter < 0) {
      throw new Error('Cannot select an invalid Agent stream cursor')
    }
    this.activeStreamTaskID = nextTaskID
    this.activeStreamAfter = nextAfter
    this.activeStreamScope = { ...scope }
  }

  clearActiveStreamTarget() {
    this.activeStreamTaskID = ''
    this.activeStreamAfter = 0
    this.activeStreamScope = {}
  }

  /** Consume the acceptance classification captured at the HTTP boundary. */
  takeInitialSubmissionOutcome(
    commandID: string,
    missingOutcome: InitialSubmissionOutcome = 'uncertain',
  ): InitialSubmissionOutcome {
    const key = commandID.trim()
    // Every real HTTP path records an outcome. Missing means the transport was
    // substituted by a caller, so the send/catch boundary chooses the proof
    // appropriate to its own result.
    const outcome = this.initialSubmissionOutcomes.get(key) || missingOutcome
    this.initialSubmissionOutcomes.delete(key)
    return outcome
  }

  private activeStreamURL() {
    if (!this.activeStreamTaskID) throw new Error('Cannot reconnect without an exact Agent stream task')
    const query = new URLSearchParams({ task_id: this.activeStreamTaskID })
    for (const [key, value] of Object.entries(this.activeStreamScope)) query.set(key, value)
    for (const [key, value] of Object.entries(this.scope)) query.set(key, value)
    if (this.activeStreamAfter > 0) query.set('after', String(this.activeStreamAfter))
    // The recovery cursor is a one-connection hand-off. If that connection is
    // interrupted, the next inspection canonically reloads again before
    // selecting another replay/suffix boundary.
    this.activeStreamAfter = 0
    return `${this.streamApi}?${query.toString()}`
  }
}

export function buildAgentChatRequestBody(body: AgentChatRequestBody): AgentChatRequestBody {
  const reviewFeedback = normalizeReviewFeedbackRefs(body.review_feedback)
  return {
    ...(body.command_id?.trim() ? { command_id: body.command_id.trim() } : {}),
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

export type InitialSubmissionOutcome = 'accepted' | 'rejected' | 'uncertain'

/** 2xx proves durable acceptance; 4xx proves rejection; 5xx remains ambiguous. */
export function initialSubmissionOutcomeForStatus(status: number): InitialSubmissionOutcome {
  if (status >= 200 && status < 300) return 'accepted'
  if (status >= 400 && status < 500) return 'rejected'
  return 'uncertain'
}

function initialCommandIDFromRequest(init?: RequestInit) {
  if (typeof init?.body !== 'string') return ''
  try {
    const body = JSON.parse(init.body) as { command_id?: unknown }
    return typeof body.command_id === 'string' ? body.command_id.trim() : ''
  } catch {
    return ''
  }
}

function normalizeReviewFeedbackRefs(
  feedback: AgentChatRequestBody['review_feedback'],
): NonNullable<AgentChatRequestBody['review_feedback']> {
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

/**
 * Reuses a normalized stable prefix while one cumulative streaming message is
 * growing. Structural changes still take the complete dedupe path, so history
 * replay and tool/data part replacement keep their canonical behavior.
 */
export class AgentUIMessageNormalizer {
  private source: AgentUIMessage[] = []
  private normalized: AgentUIMessage[] = []

  normalize(messages: AgentUIMessage[]): AgentUIMessage[] {
    const lastIndex = messages.length - 1
    const previousLast = this.source[lastIndex]
    const nextLast = messages[lastIndex]
    let stableGrowingTail =
      messages.length > 0 &&
      messages.length === this.source.length &&
      this.normalized.length === messages.length &&
      nextLast.role === previousLast?.role &&
      nextLast.parts.length === previousLast?.parts.length
    for (let index = 0; stableGrowingTail && index < messages.length; index += 1) {
      const message = messages[index]
      stableGrowingTail =
        Boolean(message.id) &&
        message.id === this.source[index]?.id &&
        message.id === this.normalized[index]?.id &&
        this.normalized[index]?.parts.length === this.source[index]?.parts.length &&
        (index === lastIndex || message === this.source[index])
    }

    let normalized: AgentUIMessage[]
    if (stableGrowingTail) {
      const tail = normalizeAgentUIMessages([nextLast])[0]
      if (tail) {
        normalized = [...this.normalized]
        normalized[lastIndex] = tail
      } else {
        normalized = normalizeAgentUIMessages(messages)
      }
    } else {
      normalized = normalizeAgentUIMessages(messages)
    }
    this.source = [...messages]
    this.normalized = normalized
    return normalized
  }
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

interface AgentUIPartDedupeIdentity {
  primaryKey: string
  legacyContentKey?: string
  stableContentKey?: string
}

interface AgentUIPartLocation {
  messageIndex: number
  partIndex: number
}

interface AgentUIContentFallbackLocation {
  location: AgentUIPartLocation
  stableContentKey: string
}

const messagePartDedupeIdentitiesCache = new WeakMap<
  AgentUIMessage,
  {
    metadata: AgentUIMessage['metadata']
    partReferences: AgentUIMessage['parts']
    identities: AgentUIPartDedupeIdentity[]
  }
>()

function normalizeRepeatedAgentUIParts(messages: AgentUIMessage[]) {
  const normalized = [...messages]
  const locationByKey = new Map<string, AgentUIPartLocation>()
  const contentFallbackByKey = new Map<string, AgentUIContentFallbackLocation | null>()
  const removedByMessage = new Map<number, Set<number>>()

  messages.forEach((message, messageIndex) => {
    const identities = agentUIPartDedupeIdentities(message)
    message.parts.forEach((part, partIndex) => {
      const identity = identities[partIndex]
      if (!identity?.primaryKey) return
      let existing = locationByKey.get(identity.primaryKey)
      if (!existing && identity.legacyContentKey) {
        const fallback = contentFallbackByKey.get(identity.legacyContentKey)
        if (
          fallback &&
          (!identity.stableContentKey || !fallback.stableContentKey || identity.stableContentKey === fallback.stableContentKey)
        ) {
          existing = fallback.location
        }
      }
      if (!existing) {
        const location = { messageIndex, partIndex }
        locationByKey.set(identity.primaryKey, location)
        rememberContentFallback(contentFallbackByKey, identity, location)
        return
      }
      locationByKey.set(identity.primaryKey, existing)
      rememberContentFallback(contentFallbackByKey, identity, existing)
      const existingMessage = normalized[existing.messageIndex]
      const existingPart = existingMessage.parts[existing.partIndex]
      const mergedPart = mergeDuplicateAgentUIPart(existingPart, part)
      const mergedMetadata = mergeAgentMessageMetadata(existingMessage.metadata, message.metadata)
      if (mergedPart !== existingPart || mergedMetadata !== existingMessage.metadata) {
        const parts = mergedPart === existingPart ? existingMessage.parts : [...existingMessage.parts]
        if (parts !== existingMessage.parts) parts[existing.partIndex] = mergedPart
        normalized[existing.messageIndex] = {
          ...existingMessage,
          parts,
          metadata: mergedMetadata,
        } as AgentUIMessage
      }
      const removedParts = removedByMessage.get(messageIndex) || new Set<number>()
      removedParts.add(partIndex)
      removedByMessage.set(messageIndex, removedParts)
    })
  })

  return normalized
    .map((message, messageIndex) => {
      const removedParts = removedByMessage.get(messageIndex)
      if (!removedParts?.size) return message
      return {
        ...message,
        parts: message.parts.filter((_part, partIndex) => !removedParts.has(partIndex)),
      } as AgentUIMessage
    })
    .filter((message) => message.parts.length > 0)
}

function rememberContentFallback(
  fallbacks: Map<string, AgentUIContentFallbackLocation | null>,
  identity: AgentUIPartDedupeIdentity,
  location: AgentUIPartLocation,
) {
  if (!identity.legacyContentKey) return
  const current = fallbacks.get(identity.legacyContentKey)
  if (current === null) return
  if (!current) {
    fallbacks.set(identity.legacyContentKey, {
      location,
      stableContentKey: identity.stableContentKey || '',
    })
    return
  }
  if (current.location.messageIndex !== location.messageIndex || current.location.partIndex !== location.partIndex) {
    fallbacks.set(identity.legacyContentKey, null)
    return
  }
  if (!current.stableContentKey && identity.stableContentKey) {
    fallbacks.set(identity.legacyContentKey, { location, stableContentKey: identity.stableContentKey })
  }
}

function agentUIPartDedupeIdentities(message: AgentUIMessage) {
  const cached = messagePartDedupeIdentitiesCache.get(message)
  if (
    cached &&
    cached.metadata === message.metadata &&
    cached.partReferences.length === message.parts.length &&
    cached.partReferences.every((part, index) => part === message.parts[index])
  ) {
    return cached.identities
  }
  const identities = message.parts.map((part) => agentUIPartDedupeIdentity(message, part))
  messagePartDedupeIdentitiesCache.set(message, {
    metadata: message.metadata,
    // AI SDK 在持久化交互结算时可能原地追加 part，因此不能只比较数组引用。
    partReferences: [...message.parts],
    identities,
  })
  return identities
}

function agentUIPartDedupeIdentity(
  message: AgentUIMessage,
  part: AgentUIMessage['parts'][number],
): AgentUIPartDedupeIdentity {
  const raw = part as Record<string, unknown>
  const type = readString(raw.type)
  if (!type) return { primaryKey: '' }
  const metadata = agentPartMetadata(message, raw)
  const runID = firstNonEmpty(metadata.run_id || '', readString(objectData(raw.data).run_id))

  if (type === 'dynamic-tool' || type.startsWith('tool-')) {
    const toolCallID = readString(raw.toolCallId)
    if (!toolCallID) return { primaryKey: '' }
    return { primaryKey: scopedAgentPartKey(runID, `tool:${toolCallID}`) }
  }

  if (isAgentDataPartType(type)) {
    const data = objectData(raw.data)
    const id = firstNonEmpty(readString(raw.id), readString(data.id))
    if (id) return { primaryKey: scopedAgentPartKey(runID, `data:${type}:${id}`) }
    if (runID && (type === 'data-agent-token-usage' || type === 'data-agent-context-compaction')) {
      return { primaryKey: `run:${runID}:data:${type}` }
    }
    return { primaryKey: '' }
  }

  if ((type === 'text' || type === 'reasoning') && runID) {
    const segmentID = firstNonEmpty(metadata.display_segment_id || '', readString(raw.id))
    if (segmentID) {
      const stableContentKey = `run:${runID}:content:${type}:id:${segmentID}`
      if (type !== 'reasoning') return { primaryKey: stableContentKey, stableContentKey }
      const text = readString(raw.text).trim()
      const legacyContentKey = text ? `run:${runID}:content:${type}:${contentPrefixFingerprint(text)}` : undefined
      return { primaryKey: stableContentKey, legacyContentKey, stableContentKey }
    }
    const text = readString(raw.text).trim()
    if (!text) return { primaryKey: '' }
    const fingerprint = type === 'reasoning' ? contentPrefixFingerprint(text) : textFingerprint(text)
    const legacyContentKey = `run:${runID}:content:${type}:${fingerprint}`
    return {
      primaryKey: legacyContentKey,
      legacyContentKey,
    }
  }

  return { primaryKey: '' }
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
  const raw =
    agent && typeof agent === 'object' && !Array.isArray(agent) ? (agent as Record<string, unknown>) : (metadata as Record<string, unknown>)
	return {
		run_id: readString(raw.run_id) || undefined,
		display_segment_id: readString(raw.display_segment_id) || undefined,
		display_phase: readDisplayPhase(raw.display_phase),
    agent_kind: readString(raw.agent_kind) || undefined,
    agent_name: readString(raw.agent_name) || undefined,
    root_agent_name: readString(raw.root_agent_name) || undefined,
    subagent: typeof raw.subagent === 'boolean' ? raw.subagent : undefined,
    subagent_session_id: readString(raw.subagent_session_id) || undefined,
    subagent_type: readString(raw.subagent_type) || undefined,
  }
}

function readDisplayPhase(value: unknown): AgentMessageMetadata['display_phase'] {
	return value === 'candidate' || value === 'progress' || value === 'final' || value === 'partial' ? value : undefined
}

function scopedAgentPartKey(runID: string, key: string) {
  return runID ? `run:${runID}:${key}` : key
}

function mergeDuplicateAgentUIPart(existing: AgentUIMessage['parts'][number], incoming: AgentUIMessage['parts'][number]) {
  const existingRaw = existing as Record<string, unknown>
  const incomingRaw = incoming as Record<string, unknown>
  const type = readString(incomingRaw.type)
  if (type === 'dynamic-tool' || type.startsWith('tool-')) {
    return toolPartStateRank(readString(incomingRaw.state)) >= toolPartStateRank(readString(existingRaw.state)) ? incoming : existing
  }
  if (isAgentDataPartType(type)) {
    const incomingStatus = readString(objectData(incomingRaw.data).status)
    const existingStatus = readString(objectData(existingRaw.data).status)
    return dataPartStatusRank(incomingStatus) >= dataPartStatusRank(existingStatus) ? incoming : existing
  }
  if ((type === 'text' || type === 'reasoning') && !readString(incomingRaw.id)) {
    const existingID = readString(existingRaw.id)
    if (existingID) return { ...incomingRaw, id: existingID } as AgentUIMessage['parts'][number]
  }
  return incoming
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
  if (status === 'success' || status === 'error' || status === 'answered' || status === 'cancelled') return 2
  if (status === 'running' || status === 'pending') return 1
  return 0
}

function textFingerprint(value: string) {
  let hash = 0
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) | 0
  }
  return `${value.length}:${(hash >>> 0).toString(36)}`
}

function contentPrefixFingerprint(value: string) {
  const prefix = value.length > 24 ? value.slice(0, 24) : value
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
    const text = message.parts
      .map((part) => (part.type === 'text' ? part.text : ''))
      .join('')
      .trim()
    if (text) return text
  }
  return ''
}

function objectData(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : {}
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function isAgentDataPartType(type: string): type is `data-agent-${string}` {
  return type.startsWith('data-agent-')
}

function firstNonEmpty(...values: Array<string | undefined>) {
  return values.find((value) => value && value.trim()) || ''
}
