import type { AgentAskInteraction, ChapterIllustration, ChatMessage, ChatPlanAction, InteractiveImage, InteractiveImageError, InteractiveImageStatus, PublicRuleRoll, TokenUsageCall } from './api-client/types'
import type { AgentMessageMetadata, AgentUIMessage } from './agent-ui'
import { agentToolInputText } from './agent-ui-message'
import { readToolPresentation } from './tool-presentation'

export type AgentMessageViewKind =
  | 'user'
  | 'assistant'
  | 'reasoning'
  | 'tool'
  | 'tool-result'
  | 'ask'
  | 'rule-roll'
  | 'context-compaction'
  | 'token-usage'
  | 'proposed-plan'
  | 'system'
  | 'error'
  | 'activity'
  | 'interactive-image'
  | 'clear'

export interface AgentPartRef {
  messageId: string
  partId: string
  partIndex: number
  type: string
}

export interface AgentMessageView {
  key: string
  kind: AgentMessageViewKind
  messageId: string
  partId: string
  partIndex: number
  ref: AgentPartRef
  message: AgentUIMessage
  part: AgentUIMessage['parts'][number]
  metadata: AgentMessageMetadata
  data: Record<string, unknown>
  content: string
  status?: 'running' | 'success' | 'error'
  streaming: boolean
  toolName?: string
  input?: unknown
  inputText?: string
  output?: unknown
  errorText?: string
  approval?: AgentAskInteraction
}

export interface AgentTokenUsageRecord {
  id?: string
  role?: 'token_usage'
  run_id?: string
  agent_kind?: string
  created_at?: string
  prompt_tokens?: number
  cached_prompt_tokens?: number
  uncached_prompt_tokens?: number
  cache_hit_rate?: number
  completion_tokens?: number
  reasoning_tokens?: number
  total_tokens?: number
  model_calls?: number
  generated_bytes?: number
  usage_calls?: TokenUsageCall[]
}

// Agent message updates are immutable. Caching by message identity keeps
// completed history stable while only the active streaming message is rebuilt.
const messageViewsCache = new WeakMap<AgentUIMessage, AgentMessageView[]>()

export function buildAgentMessageViews(messages: AgentUIMessage[]): AgentMessageView[] {
  const views: AgentMessageView[] = []
  messages.forEach((message) => {
    const cachedViews = messageViewsCache.get(message)
    if (cachedViews) {
      views.push(...cachedViews)
      return
    }

    const messageViews: AgentMessageView[] = []
    if (!(message.role === 'user' && message.metadata?.display_hidden)) {
      message.parts.forEach((part, partIndex) => {
        const view = buildAgentMessageView(message, part, partIndex)
        if (view) messageViews.push(view)
      })
    }
    messageViewsCache.set(message, messageViews)
    views.push(...messageViews)
  })
  return projectToolApprovals(projectCurrentTodoPlans(projectInteractiveMediaResults(views)))
}

// Interactive image results have both the standard AI SDK tool result and a
// richer durable data part. Prefer the data part when both describe the same
// call; result-only histories still keep the standard tool result renderer.
function projectInteractiveMediaResults(views: AgentMessageView[]): AgentMessageView[] {
  const durableResults = new Set(
    views
      .filter((view) => view.kind === 'interactive-image')
      .map(interactiveMediaResultKey),
  )
  if (durableResults.size === 0) return views
  return views.filter((view) => !(
    view.kind === 'tool' &&
    view.metadata.tool_presentation?.result === 'interactive_media' &&
    durableResults.has(interactiveMediaResultKey(view))
  ))
}

function interactiveMediaResultKey(view: AgentMessageView) {
  const runID = view.metadata.run_id?.trim()
  return `${runID ? `run:${runID}` : `message:${view.messageId}`}:part:${view.partId}`
}

// Tool approvals are lifecycle state for an existing tool call, not a new
// timeline item. Correlate by the durable tool_call_id and keep unmatched
// records as trace diagnostics instead of letting them split the run fold.
function projectToolApprovals(views: AgentMessageView[]): AgentMessageView[] {
  const toolIndexes = new Map<string, number>()
  views.forEach((view, index) => {
    if (view.kind === 'tool' && view.partId) toolIndexes.set(view.partId, index)
  })
  const approvals = new Map<number, AgentAskInteraction>()
  const matchedAskIndexes = new Set<number>()
  views.forEach((view, index) => {
    if (view.kind !== 'ask') return
    const interaction = readAskInteraction(view.data)
    if (interaction?.kind !== 'tool_approval') return
    const toolIndex = toolIndexes.get(interaction.tool_call_id)
    if (toolIndex === undefined) return
    approvals.set(toolIndex, interaction)
    matchedAskIndexes.add(index)
  })
  if (approvals.size === 0) return views
  return views
    .map((view, index) => approvals.has(index) ? { ...view, approval: approvals.get(index) } : view)
    .filter((_, index) => !matchedAskIndexes.has(index))
}

// todo is a run-local replacement state, not an append-only activity feed.
// Keep only the latest pending/successful plan for each root/subagent run; a
// successful empty plan clears the card while failed calls remain visible as
// diagnostics without erasing the last committed plan.
function projectCurrentTodoPlans(views: AgentMessageView[]): AgentMessageView[] {
  const selected = new Map<string, number | null>()
  const projected = new Set<number>()
  views.forEach((view, index) => {
    if (view.kind !== 'tool' || view.metadata.tool_presentation?.call !== 'todo' || view.status === 'error') return
    projected.add(index)
    const scope = todoPlanScope(view)
    if (view.status === 'success' && todoPlanIsEmpty(view)) {
      selected.set(scope, null)
      return
    }
    selected.set(scope, index)
  })
  if (projected.size === 0) return views
  const visible = new Set(Array.from(selected.values()).filter((index): index is number => index !== null))
  return views.filter((_, index) => !projected.has(index) || visible.has(index))
}

function todoPlanScope(view: AgentMessageView) {
  const subagent = agentSubAgentSessionKey(view)
  return `${view.metadata.run_id || 'unscoped'}\u001f${subagent}`
}

function todoPlanIsEmpty(view: AgentMessageView) {
  const output = parseStructuredValue(view.output)
  if (output && output.schema === 'agent.todo.v1' && Array.isArray(output.items)) return output.items.length === 0
  return false
}

function parseStructuredValue(value: unknown): Record<string, unknown> | null {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>
  if (typeof value !== 'string' || !value.trim()) return null
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : null
  } catch {
    return null
  }
}

export function selectAgentTokenUsageRecords(messages: AgentUIMessage[]): AgentTokenUsageRecord[] {
  return buildAgentMessageViews(messages)
    .filter((view) => view.kind === 'token-usage')
    .map(agentTokenUsageRecordFromView)
}

export function countCompletedAgentTurnSignals(messages: AgentUIMessage[]): number {
  return buildAgentMessageViews(messages).filter((view) =>
    (view.kind === 'assistant' || view.kind === 'tool-result' || view.kind === 'tool') &&
    (Boolean(agentViewContent(view).trim()) || view.status === 'success'),
  ).length
}

export function hasCompletedAgentTurn(messages: AgentUIMessage[], isStreaming: boolean): boolean {
  return !isStreaming && countCompletedAgentTurnSignals(messages) > 0
}

export function agentViewContent(view: AgentMessageView) {
  return view.content || readString(view.data.content) || readString(view.data.message) || readString(view.data.error)
}

/** Content whose full height must be reserved for the next visible streaming frame. */
export function agentViewLayoutContent(view: AgentMessageView) {
  if (view.streaming && view.metadata.streaming_target_content !== undefined) {
    return view.metadata.streaming_target_content
  }
  return agentViewContent(view)
}

export function agentViewNavigationAnchor(view: AgentMessageView) {
  return view.metadata.navigation_turn_id || view.metadata.turn_id || ''
}

export function isAgentTraceView(view: AgentMessageView) {
  if (view.kind === 'interactive-image') return false
  if (view.metadata.tool_presentation?.call === 'interactive_media' || view.metadata.tool_presentation?.result === 'interactive_media') return false
  return view.kind === 'reasoning' || view.kind === 'tool' || view.kind === 'tool-result' ||
    (view.kind === 'ask' && agentViewAskInteraction(view)?.kind === 'tool_approval')
}

export function agentViewAskInteraction(view: AgentMessageView) {
  return view.approval || (view.kind === 'ask' ? readAskInteraction(view.data) : undefined)
}

export function agentViewAskID(view: AgentMessageView) {
  return agentViewAskInteraction(view)?.id?.trim() || ''
}

export function isAgentSubAgentTimelineView(view: AgentMessageView) {
  return view.metadata.subagent === true && Boolean(agentSubAgentSessionKey(view))
}

export function agentSubAgentSessionKey(view: AgentMessageView) {
  const metadata = view.metadata
  if (metadata.subagent_session_id) return metadata.subagent_session_id
  if (metadata.run_id && (metadata.agent_name || metadata.subagent_type)) {
    return `${metadata.run_id}:${metadata.agent_name || metadata.subagent_type}`
  }
  if (metadata.run_path?.length) return metadata.run_path.join('/')
  return ''
}

export interface AgentSubAgentTimelineGroup {
  key: string
  nextIndex: number
  sessionKeys: string[]
  startIndex: number
  views: AgentMessageView[]
}

/**
 * Projects one delegated invocation into one card while preserving its first
 * timeline position. Older sessions may have a new numeric session ID after
 * every child tool loop; matching ancestry keeps those adjacent legacy slices
 * together without crossing a root-Agent event or a different SubAgent path.
 */
export function buildAgentSubAgentTimelineGroups(views: AgentMessageView[]): AgentSubAgentTimelineGroup[] {
  const groups: AgentSubAgentTimelineGroup[] = []
  for (let startIndex = 0; startIndex < views.length; startIndex += 1) {
    const first = views[startIndex]
    if (!isAgentSubAgentTimelineView(first)) continue

    const groupedViews: AgentMessageView[] = []
    const sessionKeys = new Set<string>()
    let nextIndex = startIndex
    while (nextIndex < views.length) {
      const candidate = views[nextIndex]
      if (candidate.kind === 'token-usage' && candidate.metadata.run_id === first.metadata.run_id) {
        nextIndex += 1
        continue
      }
      if (isAgentSubAgentTimelineView(candidate) && sameSubAgentTimelineGroup(first, candidate)) {
        groupedViews.push(candidate)
        sessionKeys.add(agentSubAgentSessionKey(candidate))
        nextIndex += 1
        continue
      }
      // Historical approval records did not carry SubAgent metadata. Once
      // resolved, a tool-call link still proves that the interaction belongs
      // to this delegated timeline and must not split its card.
      if (isAgentSubAgentTimelineBridgeView(candidate, groupedViews)) {
        groupedViews.push(candidate)
        nextIndex += 1
        continue
      }
      break
    }
    groups.push({
      key: agentSubAgentSessionKey(first),
      nextIndex,
      sessionKeys: Array.from(sessionKeys),
      startIndex,
      views: groupedViews,
    })
    startIndex = nextIndex - 1
  }
  return groups
}

export function isAgentSubAgentTimelineBridgeView(candidate: AgentMessageView, groupedViews: AgentMessageView[]) {
  if (candidate.kind !== 'ask' || candidate.streaming) return false
  const toolCallID = readString(candidate.data.tool_call_id)
  if (!toolCallID) return false
  return groupedViews.some(view => isAgentSubAgentTimelineView(view) && view.kind === 'tool' && view.partId === toolCallID)
}

function sameSubAgentTimelineGroup(first: AgentMessageView, candidate: AgentMessageView) {
  const firstSession = agentSubAgentSessionKey(first)
  if (firstSession && firstSession === agentSubAgentSessionKey(candidate)) return true
  const firstSource = legacySubAgentSourceKey(first)
  return Boolean(firstSource && firstSource === legacySubAgentSourceKey(candidate))
}

function legacySubAgentSourceKey(view: AgentMessageView) {
  const metadata = view.metadata
  const runID = metadata.run_id?.trim() || ''
  const agentName = metadata.agent_name?.trim() || metadata.subagent_type?.trim() || ''
  if (!runID || !agentName) return ''
  const path = metadata.run_path?.map(step => step.trim()).filter(Boolean).join('\u0000') || ''
  return [runID, metadata.root_agent_name?.trim() || '', agentName, path].join('\u001f')
}

export function agentViewStableKey(view: AgentMessageView) {
  const runID = view.metadata.run_id?.trim()
  const segmentID = view.metadata.display_segment_id?.trim()
  if (runID && segmentID) return `${view.kind}:run:${runID}:segment:${segmentID}`
  if (runID && (view.kind === 'tool' || view.kind === 'tool-result') && view.partId) {
    const scope = agentSubAgentSessionKey(view) || 'root'
    return `${view.kind}:run:${runID}:scope:${scope}:part:${view.partId}`
  }
  return `${view.kind}:${view.messageId}:${view.partId}:${view.partIndex}`
}

export function isPlanProtocolToolName(name: string) {
  // The provider may emit a tool call beside the persisted plan card. Rendering
  // both would duplicate the same proposal in the timeline.
  return name === 'proposed_plan'
}

export function agentViewToRenderMessage(view: AgentMessageView, options: { forceDone?: boolean } = {}): ChatMessage | null {
  const data = view.data
  const meta = metadataToChatFields(view)
  const streaming = options.forceDone ? false : view.streaming
  const status = view.status
  const id = view.partId || view.messageId
  switch (view.kind) {
    case 'user':
      return { id, role: 'user', content: view.content, streaming, ...meta }
    case 'assistant':
      return { id, role: 'assistant', content: view.content, streaming, ...meta }
    case 'reasoning':
      return { id, role: 'thinking', content: view.content, streaming, ...meta }
    case 'tool': {
      const raw = view.part as Record<string, any>
      const args = view.inputText ?? stringifyInput(view.input)
      const result = raw.state === 'output-error' ? view.errorText : stringifyOutput(view.output)
      return {
        id,
        role: 'tool_call',
        content: args ? `${view.toolName || ''}\n${args}` : view.toolName || '',
        name: view.toolName,
        args,
        status,
        result,
        ask: view.approval,
        illustration: readChapterIllustration(objectData(raw.toolMetadata).illustration),
        streaming,
        ...meta,
      }
    }
    case 'ask':
      return {
        id,
        role: 'ask',
        content: view.content,
        status,
        streaming,
        ask: readAskInteraction(data),
        ...meta,
      }
    case 'tool-result':
      return {
        id,
        role: 'tool_result',
        content: view.content,
        name: view.toolName || readString(data.name),
        result: readString(data.result) || view.content,
        illustration: readChapterIllustration(data.illustration),
        status,
        streaming,
        ...meta,
      }
    case 'rule-roll':
      return { id, role: 'rule_roll', content: view.content, rule_roll: readRuleRoll(data.rule_roll) || readRuleRoll(data), streaming, ...meta }
    case 'context-compaction':
      return { id, role: 'context_compaction', content: view.content, status, streaming, ...contextFields(data), ...meta }
    case 'token-usage':
      return { id, role: 'token_usage', content: view.content, ...tokenUsageFields(data), ...meta }
    case 'proposed-plan':
      return { id, role: 'proposed_plan', content: view.content, status, streaming, thinking_preview: readString(data.thinking_preview), plan_action: readPlanAction(data.plan_action), ...meta }
    case 'system':
      return { id, role: 'system', content: view.content, streaming, ...meta }
    case 'error':
      return { id, role: 'error', content: view.content, streaming, ...meta }
    case 'activity':
      return { id, role: 'system', content: view.content, streaming, ...meta }
    case 'interactive-image':
      return {
        id,
        role: 'tool_result',
        content: view.content,
        name: view.toolName || readString(data.name),
        result: readString(data.result) || view.content,
        interactive_image: readInteractiveImage(data.interactive_image),
        interactive_images: readInteractiveImages(data.interactive_images),
        interactive_image_error: readInteractiveImageError(data.interactive_image_error),
        interactive_image_status: readInteractiveImageStatus(data),
        status,
        streaming,
        ...meta,
      }
    case 'clear':
      return { id, type: 'clear', role: 'system', content: '', created_at: readString(data.created_at) || meta.created_at }
    default:
      return null
  }
}

function buildAgentMessageView(message: AgentUIMessage, part: AgentUIMessage['parts'][number], partIndex: number): AgentMessageView | null {
  const raw = part as Record<string, any>
  const type = readString(raw.type)
  // AI SDK data chunks cannot carry providerMetadata. Their display metadata
  // remains inside data, while text/reasoning/tool parts use provider fields.
  const metadata = mergeMetadata(message.metadata, raw.data, raw.providerMetadata, raw.callProviderMetadata, raw.resultProviderMetadata)
  const partId = readString(raw.id) || readString(raw.toolCallId) || `${message.id}:${partIndex}`
  const ref = { messageId: message.id, partId, partIndex, type }
  const base = {
    key: `${message.id}:${partId}:${partIndex}`,
    messageId: message.id,
    partId,
    partIndex,
    ref,
    message,
    part,
    metadata,
    data: objectData(raw.data),
  }

  if (message.role === 'user' && type === 'text') {
    const content = readString(raw.text)
    if (!content) return null
    return { ...base, kind: 'user', content, streaming: false }
  }

  if (message.role === 'system' && type === 'text') {
    const content = readString(raw.text)
    if (!content) return null
    return { ...base, kind: 'system', content, streaming: false }
  }

  if (type === 'text') {
    const content = readString(raw.text)
    if (!content && raw.state !== 'streaming') return null
    return { ...base, kind: 'assistant', content, streaming: raw.state === 'streaming' }
  }

  if (type === 'reasoning') {
    const content = readString(raw.text)
    if (!content && raw.state !== 'streaming') return null
    return { ...base, kind: 'reasoning', content, streaming: raw.state === 'streaming' }
  }

  if (type === 'dynamic-tool' || type.startsWith('tool-')) {
    const toolName = type === 'dynamic-tool' ? firstNonEmpty(readString(raw.toolName), 'unknown_tool') : type.replace(/^tool-/, '')
    // ask has a dedicated durable data part emitted only after pending state is
    // committed. Hiding the speculative model tool frame avoids duplicate UI.
    if (metadata.tool_presentation?.call === 'interaction') return null
    const status = toolStatus(readString(raw.state))
    return {
      ...base,
      kind: 'tool',
      content: toolName,
      status,
      streaming: raw.state === 'input-streaming',
      toolName,
      input: raw.input,
      inputText: agentToolInputText(part),
      output: raw.output,
      errorText: readString(raw.errorText),
    }
  }

  if (!type.startsWith('data-agent-')) return null
  const data = objectData(raw.data)
  const content = readString(data.content) || readString(data.message) || readString(data.error)
  const status = normalizeStatus(data.status)
  const streaming = status === 'running'
  switch (type) {
    case 'data-agent-clear':
      return { ...base, kind: 'clear', data, content: '', streaming: false }
    case 'data-agent-ask':
      return { ...base, kind: 'ask', data, content: firstAskQuestion(data), status, streaming: readString(data.status) === 'pending' }
    case 'data-agent-context-compaction':
      return { ...base, kind: 'context-compaction', data, content, status, streaming }
    case 'data-agent-token-usage':
      return { ...base, kind: 'token-usage', data, content, streaming: false }
    case 'data-agent-proposed-plan':
      return { ...base, kind: 'proposed-plan', data, content, status, streaming }
    case 'data-agent-system':
      if (!content) return null
      return { ...base, kind: 'system', data, content, streaming: false }
    case 'data-agent-error':
      return { ...base, kind: 'error', data, content, streaming: false }
    case 'data-agent-interactive-image':
      return {
        ...base,
        kind: 'interactive-image',
        data,
        content,
        status,
        streaming,
        toolName: readString(data.name) || 'generate_interactive_image',
      }
    case 'data-agent-rule-roll':
      return { ...base, kind: 'rule-roll', data, content, streaming: false }
    case 'data-agent-tool-result':
      return {
        ...base,
        kind: 'tool-result',
        data,
        content,
        status,
        streaming,
        toolName: readString(data.name),
        output: data.result ?? data.content,
      }
    case 'data-agent-activity': {
      // Lifecycle activity payloads may carry the accepted user input in
      // `message` (for example agent_cycle_started). Only explicit `content`
      // is presentation text; treating `message` as display content echoes the
      // user's bubble back as a system message.
      const activityContent = readString(data.content)
      if (!activityContent) return null
      return { ...base, kind: 'activity', data, content: activityContent, status, streaming }
    }
    default:
      if (!content) return null
      return { ...base, kind: 'activity', data, content, streaming }
  }
}

function metadataToChatFields(view: AgentMessageView): Partial<ChatMessage> {
  const metadata = view.metadata
  return {
    created_at: metadata.created_at,
    run_id: metadata.run_id,
    agent_kind: metadata.agent_kind,
    agent_name: metadata.agent_name,
    root_agent_name: metadata.root_agent_name,
    run_path: metadata.run_path,
    subagent: metadata.subagent,
    subagent_session_id: metadata.subagent_session_id,
    subagent_type: metadata.subagent_type,
    parent_call_id: metadata.parent_call_id,
    sse_hidden_fields: metadata.sse_hidden_fields,
    sse_hidden_reason: metadata.sse_hidden_reason,
    sse_display_notice: metadata.sse_display_notice,
    sse_generated_chars: metadata.sse_generated_chars,
    streaming_target_content: metadata.streaming_target_content,
    turn_id: metadata.turn_id,
    navigation_turn_id: metadata.navigation_turn_id,
    turn_versions: metadata.turn_versions,
    turn_version_index: metadata.turn_version_index,
    user_references: metadata.user_references,
    tool_presentation: metadata.tool_presentation,
  }
}

function contextFields(data: Record<string, unknown>): Partial<ChatMessage> {
  return {
    phase: readString(data.phase),
    attempt: readNumber(data.attempt),
    tokens_before: readNumber(data.tokens_before),
    tokens_after: readNumber(data.tokens_after),
		projected_tokens_before: readNumber(data.projected_tokens_before),
		projected_tokens_after: readNumber(data.projected_tokens_after),
		reserved_completion_tokens: readNumber(data.reserved_completion_tokens),
		reserved_tool_result_tokens: readNumber(data.reserved_tool_result_tokens),
    context_window_tokens: readNumber(data.context_window_tokens),
    threshold: readNumber(data.threshold),
    target_ratio: readNumber(data.target_ratio),
    epoch: readNumber(data.epoch),
    source_message_count: readNumber(data.source_message_count),
    message_count_before: readNumber(data.message_count_before),
    message_count_after: readNumber(data.message_count_after),
    skipped_reason: readString(data.skipped_reason),
  }
}

function tokenUsageFields(data: Record<string, unknown>): Partial<ChatMessage> {
  return {
    run_id: readString(data.run_id),
    agent_kind: readString(data.agent_kind),
    prompt_tokens: readNumber(data.prompt_tokens),
    cached_prompt_tokens: readNumber(data.cached_prompt_tokens),
    uncached_prompt_tokens: readNumber(data.uncached_prompt_tokens),
    cache_hit_rate: readNumber(data.cache_hit_rate),
    completion_tokens: readNumber(data.completion_tokens),
    reasoning_tokens: readNumber(data.reasoning_tokens),
    total_tokens: readNumber(data.total_tokens),
    model_calls: readNumber(data.model_calls),
    generated_bytes: readNumber(data.generated_bytes),
    usage_calls: readUsageCalls(data.usage_calls),
  }
}

function stringifyInput(input: unknown) {
  if (input === undefined) return ''
  if (typeof input === 'string') return input
  try {
    return JSON.stringify(input)
  } catch {
    return String(input)
  }
}

function stringifyOutput(output: unknown) {
  if (output === undefined) return ''
  if (typeof output === 'string') return output
  try {
    return JSON.stringify(output, null, 2)
  } catch {
    return String(output)
  }
}

function readChapterIllustration(value: unknown): ChapterIllustration | undefined {
  const data = objectData(value)
  const schema = readString(data.schema)
  const imagePath = readString(data.image_path)
  if (!schema || !imagePath) return undefined
  return {
    schema,
    chapter_path: readString(data.chapter_path),
    image_path: imagePath,
    meta_path: readString(data.meta_path),
    markdown: readString(data.markdown),
    alt_text: readString(data.alt_text),
    profile_id: readString(data.profile_id),
    provider: readString(data.provider),
    model: readString(data.model),
    size: readString(data.size) || undefined,
    quality: readString(data.quality) || undefined,
    output_format: readString(data.output_format) || undefined,
    created_at: readString(data.created_at) || undefined,
    revised_prompt: readString(data.revised_prompt) || undefined,
    mime_type: readString(data.mime_type) || undefined,
    size_bytes: readNumber(data.size_bytes),
  }
}

function readInteractiveImage(value: unknown): InteractiveImage | undefined {
  const data = objectData(value)
  const schema = readString(data.schema)
  const imagePath = readString(data.image_path)
  if (!schema || !imagePath) return undefined
  return {
    schema,
    story_id: readString(data.story_id),
    branch_id: readString(data.branch_id),
    turn_id: readString(data.turn_id),
    image_path: imagePath,
    meta_path: readString(data.meta_path),
    alt_text: readString(data.alt_text),
    profile_id: readString(data.profile_id),
    provider: readString(data.provider),
    model: readString(data.model),
    size: readString(data.size),
    quality: readString(data.quality),
    output_format: readString(data.output_format),
    created_at: readString(data.created_at),
    revised_prompt: readString(data.revised_prompt),
    mime_type: readString(data.mime_type),
    size_bytes: readNumber(data.size_bytes),
  }
}

function readInteractiveImages(value: unknown): InteractiveImage[] | undefined {
  if (!Array.isArray(value)) return undefined
  const images = value.map(readInteractiveImage).filter((item): item is InteractiveImage => Boolean(item))
  return images.length ? images : undefined
}

function readInteractiveImageError(value: unknown): InteractiveImageError | undefined {
  const data = objectData(value)
  const schema = readString(data.schema)
  if (!schema) return undefined
  return {
    schema,
    story_id: readString(data.story_id),
    branch_id: readString(data.branch_id),
    turn_id: readString(data.turn_id),
    message: readString(data.message),
    created_at: readString(data.created_at),
  }
}

function readRuleRoll(value: unknown): PublicRuleRoll | undefined {
  const data = objectData(value)
  if (Object.keys(data).length === 0) return undefined
  const rolls = Array.isArray(data.rolls)
    ? data.rolls.map(item => readNumber(item)).filter((item): item is number => item !== undefined)
    : undefined
  const stateChanges = Array.isArray(data.state_changes)
    ? data.state_changes
        .map((item) => {
          const change = objectData(item)
          const actorId = readString(change.actor_id)
          const fieldId = readString(change.field_id)
          if (!actorId || !fieldId) return null
          return { actor_id: actorId, field_id: fieldId, change: readNumber(change.change) || 0, reason: readString(change.reason) }
        })
        .filter((item): item is NonNullable<typeof item> => Boolean(item))
    : undefined
  return {
    resolution_id: readString(data.resolution_id),
    label: readString(data.label),
    difficulty: readString(data.difficulty),
    dice: readString(data.dice),
    roll_mode: readString(data.roll_mode),
    rolls,
    kept_roll: readNumber(data.kept_roll),
    base_target: readNumber(data.base_target),
    target: readNumber(data.target),
    bonus_total: readNumber(data.bonus_total),
    total: readNumber(data.total),
    outcome: readString(data.outcome),
    result: readString(data.result),
    cost: readString(data.cost),
    stakes: readString(data.stakes),
    state_changes: stateChanges,
  }
}

function readInteractiveImageStatus(data: Record<string, unknown>): InteractiveImageStatus | undefined {
  const status = readString(data.interactive_image_status) || readString(data.status)
  return status === 'running' || status === 'success' || status === 'error' ? status : undefined
}

function readPlanAction(value: unknown): ChatPlanAction | undefined {
  const action = readString(value)
  return action === 'approved' || action === 'continue' || action === 'exited' ? action : undefined
}

function agentTokenUsageRecordFromView(view: AgentMessageView): AgentTokenUsageRecord {
  const data = view.data
  return {
    id: view.partId,
    role: 'token_usage',
    run_id: readString(data.run_id) || view.metadata.run_id,
    agent_kind: readString(data.agent_kind) || view.metadata.agent_kind,
    created_at: readString(data.created_at) || view.metadata.created_at,
    prompt_tokens: readNumber(data.prompt_tokens),
    cached_prompt_tokens: readNumber(data.cached_prompt_tokens),
    uncached_prompt_tokens: readNumber(data.uncached_prompt_tokens),
    cache_hit_rate: readNumber(data.cache_hit_rate),
    completion_tokens: readNumber(data.completion_tokens),
    reasoning_tokens: readNumber(data.reasoning_tokens),
    total_tokens: readNumber(data.total_tokens),
    model_calls: readNumber(data.model_calls),
    generated_bytes: readNumber(data.generated_bytes),
    usage_calls: readUsageCalls(data.usage_calls),
  }
}

function mergeMetadata(...values: unknown[]): AgentMessageMetadata {
  const result: AgentMessageMetadata = {}
  for (const value of values) {
    const metadata = providerAgentMetadata(value)
    for (const [key, candidate] of Object.entries(metadata)) {
      if (candidate !== undefined) Object.assign(result, { [key]: candidate })
    }
  }
  return result
}

function providerAgentMetadata(value: unknown): AgentMessageMetadata {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const raw = value as Record<string, unknown>
  const agent = raw.agent && typeof raw.agent === 'object' && !Array.isArray(raw.agent)
    ? raw.agent as Record<string, unknown>
    : raw
	return {
		created_at: readString(agent.created_at) || undefined,
		display_role: readString(agent.display_role) as AgentMessageMetadata['display_role'] || undefined,
		display_phase: readDisplayPhase(agent.display_phase),
    display_segment_id: readString(agent.display_segment_id) || undefined,
    history_type: readString(agent.history_type) || undefined,
    run_id: readString(agent.run_id) || undefined,
    agent_kind: readString(agent.agent_kind) || undefined,
    agent_name: readString(agent.agent_name) || undefined,
    root_agent_name: readString(agent.root_agent_name) || undefined,
    run_path: readStringArray(agent.run_path),
    subagent: agent.subagent === true || undefined,
    subagent_session_id: readString(agent.subagent_session_id) || undefined,
    subagent_type: readString(agent.subagent_type) || undefined,
    parent_call_id: readString(agent.parent_call_id) || undefined,
    sse_hidden_fields: readStringArray(agent.sse_hidden_fields),
    sse_hidden_reason: readString(agent.sse_hidden_reason) || undefined,
    sse_display_notice: readString(agent.sse_display_notice) || undefined,
    sse_generated_chars: readNumber(agent.sse_generated_chars),
    display_hidden: agent.display_hidden === true || undefined,
    streaming_target_content: readString(agent.streaming_target_content) || undefined,
    turn_id: readString(agent.turn_id) || undefined,
    navigation_turn_id: readString(agent.navigation_turn_id) || undefined,
    turn_versions: readTurnVersions(agent.turn_versions),
    turn_version_index: readNumber(agent.turn_version_index),
    user_references: readUserMessageReferences(agent.user_references),
    tool_presentation: readToolPresentation(agent.tool_presentation),
  }
}

function readDisplayPhase(value: unknown): AgentMessageMetadata['display_phase'] {
	return value === 'candidate' || value === 'progress' || value === 'final' || value === 'partial' ? value : undefined
}

function readUserMessageReferences(value: unknown): AgentMessageMetadata['user_references'] {
  if (!Array.isArray(value)) return undefined
  const references = value
    .map((item) => {
      if (!item || typeof item !== 'object' || Array.isArray(item)) return null
      const data = item as Record<string, unknown>
      const kind = readString(data.kind)
      const label = readString(data.label)
      if (!label || !['file', 'lore', 'style', 'selection', 'review_comment'].includes(kind)) return null
      return {
        kind: kind as NonNullable<AgentMessageMetadata['user_references']>[number]['kind'],
        id: readString(data.id) || undefined,
        label,
        detail: readString(data.detail) || undefined,
        start_line: readNumber(data.start_line),
        end_line: readNumber(data.end_line),
      }
    })
    .filter((item): item is NonNullable<typeof item> => Boolean(item))
  return references.length ? references : undefined
}

function readUsageCalls(value: unknown): TokenUsageCall[] | undefined {
  if (!Array.isArray(value)) return undefined
  const calls = value
    .map((item) => {
      if (!item || typeof item !== 'object' || Array.isArray(item)) return null
      const data = item as Record<string, unknown>
      return {
        index: readNumber(data.index),
        created_at: readString(data.created_at),
        finish_reason: readString(data.finish_reason),
        requested_tools: readStringArray(data.requested_tools),
        after_tools: readStringArray(data.after_tools),
        prompt_tokens: readNumber(data.prompt_tokens),
        cached_prompt_tokens: readNumber(data.cached_prompt_tokens),
        uncached_prompt_tokens: readNumber(data.uncached_prompt_tokens),
        cache_hit_rate: readNumber(data.cache_hit_rate),
        completion_tokens: readNumber(data.completion_tokens),
        reasoning_tokens: readNumber(data.reasoning_tokens),
        total_tokens: readNumber(data.total_tokens),
      }
    })
    .filter((item): item is NonNullable<typeof item> => Boolean(item))
  return calls.length ? calls as TokenUsageCall[] : undefined
}

function readTurnVersions(value: unknown): AgentMessageMetadata['turn_versions'] {
  if (!Array.isArray(value)) return undefined
  const versions = value
    .map((item) => {
      if (!item || typeof item !== 'object' || Array.isArray(item)) return null
      const data = item as Record<string, unknown>
      const turnID = readString(data.turn_id)
      const ts = readString(data.ts)
      if (!turnID || !ts) return null
      return { turn_id: turnID, ts, current: data.current === true || undefined }
    })
    .filter((item): item is NonNullable<typeof item> => Boolean(item))
  return versions.length ? versions : undefined
}

function firstAskQuestion(data: Record<string, unknown>) {
  const questions = Array.isArray(data.questions) ? data.questions : []
  return readString(objectData(questions[0]).question)
}

function readAskInteraction(data: Record<string, unknown>): AgentAskInteraction | undefined {
  const id = readString(data.id)
  const toolCallID = readString(data.tool_call_id)
  const agentKind = readString(data.agent_kind)
  const status = readString(data.status)
  if (!id || !toolCallID || !agentKind || !['pending', 'answered', 'cancelled'].includes(status) || !Array.isArray(data.questions)) {
    return undefined
  }
  return data as unknown as AgentAskInteraction
}

function toolStatus(state: string | undefined): AgentMessageView['status'] {
  if (state === 'output-error' || state === 'output-denied') return 'error'
  if (state === 'output-available') return 'success'
  return 'running'
}

function normalizeStatus(value: unknown): AgentMessageView['status'] {
  const status = readString(value)
  return status === 'running' || status === 'error' || status === 'success' ? status : undefined
}

function objectData(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readNumber(value: unknown) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(numberValue) ? numberValue : undefined
}

function readStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  const result = value.map(item => readString(item)).filter(Boolean)
  return result.length ? result : undefined
}

function firstNonEmpty(...values: string[]) {
  return values.find(value => value.trim()) || ''
}
