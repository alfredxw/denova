import type { ChatTransport, UIMessage } from 'ai'
import { DefaultChatTransport } from 'ai'
import { fetchAPI } from './api-client/client'
import type { ChapterIllustration, ChatMessage, InteractiveImage, InteractiveImageError, PublicRuleRoll, TokenUsageCall } from './api-client/types'

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
}

type AgentDataPayload = Record<string, unknown>

type AgentToolUIPart = {
  type: 'dynamic-tool' | `tool-${string}`
  toolName?: string
  toolCallId?: string
  state?: string
  input?: unknown
  output?: unknown
  errorText?: string
  toolMetadata?: Record<string, unknown>
  callProviderMetadata?: unknown
}

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
}

export class AgentChatTransport implements ChatTransport<AgentUIMessage> {
  private readonly transport: DefaultChatTransport<AgentUIMessage>

  constructor() {
    this.transport = new DefaultChatTransport<AgentUIMessage>({
      api: '/api/chat/ui',
      fetch: fetchAPI,
      prepareSendMessagesRequest: ({ messages, body }) => ({
        body: {
          ...(body || {}),
          message: bodyMessage(body) || latestUserText(messages),
        },
      }),
      prepareReconnectToStreamRequest: () => ({
        api: '/api/chat/ui/stream',
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
  }
}

export function agentUIMessagesToChatMessages(messages: AgentUIMessage[]): ChatMessage[] {
  const result: ChatMessage[] = []
  for (const message of messages) {
    const meta = metadataToChatFields(message.metadata)
    if (message.role === 'user') {
      if (message.metadata?.display_hidden) continue
      const content = message.parts.map(part => part.type === 'text' ? part.text : '').join('')
      if (content) result.push({ id: message.id, role: 'user', content, ...meta })
      continue
    }
    if (message.role === 'system') {
      const content = message.parts.map(part => part.type === 'text' ? part.text : '').join('')
      if (content) result.push({ id: message.id, role: 'system', content, ...meta })
      continue
    }
    message.parts.forEach((part, index) => {
      const id = `${message.id}:${index}`
      if (part.type === 'text') {
        if (part.text) result.push({ id, role: 'assistant', content: part.text, streaming: part.state === 'streaming', ...meta, ...providerMetadataToChatFields(part.providerMetadata) })
        return
      }
      if (part.type === 'reasoning') {
        if (part.text) result.push({ id, role: 'thinking', content: part.text, streaming: part.state === 'streaming', ...meta, ...providerMetadataToChatFields(part.providerMetadata) })
        return
      }
      if (part.type === 'dynamic-tool' || isToolPartType(part.type)) {
        result.push(toolPartToChatMessage(part as AgentToolUIPart, id, meta))
        return
      }
      if (isAgentDataPartType(part.type)) {
        const converted = dataPartToChatMessage(part, id, meta)
        if (converted) result.push(converted)
      }
    })
  }
  return result
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

function toolPartToChatMessage(part: AgentToolUIPart, fallbackID: string, meta: Partial<ChatMessage>): ChatMessage {
  const name = part.type === 'dynamic-tool'
    ? firstNonEmpty(part.toolName, 'unknown_tool')
    : part.type.replace(/^tool-/, '')
  const args = stringifyInput(part.input)
  const status = toolStatus(part.state)
  const result = part.state === 'output-available' ? stringifyOutput(part.output) : undefined
  const errorText = part.state === 'output-error' ? part.errorText : undefined
  const illustration = readChapterIllustration(part.toolMetadata?.illustration)
  return {
    id: part.toolCallId || fallbackID,
    role: 'tool_call',
    content: args ? `${name}\n${args}` : name,
    name,
    args,
    status,
    result: result || errorText,
    illustration,
    streaming: part.state === 'input-streaming',
    ...meta,
    ...providerMetadataToChatFields(part.callProviderMetadata),
  }
}

function dataPartToChatMessage(part: { type: string; id?: string; data?: unknown }, fallbackID: string, meta: Partial<ChatMessage>): ChatMessage | null {
  const data = objectData(part.data)
  const id = part.id || readString(data.id) || fallbackID
  switch (part.type) {
    case 'data-agent-clear':
      return { id, type: 'clear', role: 'system', content: '', created_at: readString(data.created_at) || meta.created_at }
    case 'data-agent-context-compaction':
      return { id, role: 'context_compaction', content: readString(data.content), status: normalizeStatus(data.status), ...numericContextFields(data), ...meta }
    case 'data-agent-token-usage':
      return { id, role: 'token_usage', content: readString(data.content), ...tokenUsageFields(data), ...meta }
    case 'data-agent-plan-question':
      return { id, role: 'plan_question', content: readString(data.content), status: normalizeStatus(data.status), streaming: readString(data.status) === 'running', ...meta }
    case 'data-agent-proposed-plan':
      return { id, role: 'proposed_plan', content: readString(data.content), status: normalizeStatus(data.status), streaming: readString(data.status) === 'running', ...meta }
    case 'data-agent-system':
      return { id, role: 'system', content: readString(data.content), ...meta }
    case 'data-agent-error':
      return { id, role: 'error', content: readString(data.content) || readString(data.message) || readString(data.error), ...meta }
    case 'data-agent-interactive-image':
      return { id, role: 'tool_result', content: readString(data.content), name: readString(data.name), result: readString(data.result) || readString(data.content), interactive_image: readInteractiveImage(data.interactive_image), interactive_image_error: readInteractiveImageError(data.interactive_image_error), interactive_image_status: interactiveImageStatus(data), status: normalizeStatus(data.status), ...meta }
    case 'data-agent-rule-roll':
      return { id, role: 'rule_roll', content: readString(data.content), rule_roll: readRuleRoll(data.rule_roll) || readRuleRoll(data), ...meta }
    case 'data-agent-tool-result':
      return { id, role: 'tool_result', content: readString(data.content), name: readString(data.name), result: readString(data.result), illustration: readChapterIllustration(data.illustration), ...meta }
    default:
      return null
  }
}

function metadataToChatFields(metadata?: AgentMessageMetadata): Partial<ChatMessage> {
  if (!metadata) return {}
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
    sse_hidden_fields: metadata.sse_hidden_fields,
    sse_hidden_reason: metadata.sse_hidden_reason,
    sse_display_notice: metadata.sse_display_notice,
    sse_generated_chars: metadata.sse_generated_chars,
  }
}

function providerMetadataToChatFields(metadata: unknown): Partial<ChatMessage> {
  if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) return {}
  const agent = (metadata as Record<string, unknown>).agent
  if (!agent || typeof agent !== 'object' || Array.isArray(agent)) return {}
  return metadataToChatFields(agent as AgentMessageMetadata)
}

function normalizeStatus(value: unknown): ChatMessage['status'] {
  const status = readString(value)
  return status === 'running' || status === 'error' || status === 'success' ? status : undefined
}

function toolStatus(state: string | undefined): ChatMessage['status'] {
  if (state === 'output-error' || state === 'output-denied') return 'error'
  if (state === 'output-available') return 'success'
  return 'running'
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
  return result.length > 0 ? result : undefined
}

function numericContextFields(data: Record<string, unknown>): Partial<ChatMessage> {
  return {
    phase: readString(data.phase),
    attempt: readNumber(data.attempt),
    tokens_before: readNumber(data.tokens_before),
    tokens_after: readNumber(data.tokens_after),
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
    created_at: readString(data.created_at),
  }
}

function readUsageCalls(value: unknown): TokenUsageCall[] | undefined {
  if (!Array.isArray(value)) return undefined
  const calls = value
    .map((item): TokenUsageCall | null => {
      if (!item || typeof item !== 'object' || Array.isArray(item)) return null
      const call = item as Record<string, unknown>
      return {
        index: readNumber(call.index),
        created_at: readString(call.created_at),
        finish_reason: readString(call.finish_reason),
        requested_tools: readStringArray(call.requested_tools),
        after_tools: readStringArray(call.after_tools),
        prompt_tokens: readNumber(call.prompt_tokens),
        cached_prompt_tokens: readNumber(call.cached_prompt_tokens),
        uncached_prompt_tokens: readNumber(call.uncached_prompt_tokens),
        cache_hit_rate: readNumber(call.cache_hit_rate),
        completion_tokens: readNumber(call.completion_tokens),
        reasoning_tokens: readNumber(call.reasoning_tokens),
        total_tokens: readNumber(call.total_tokens),
      }
    })
    .filter((call): call is TokenUsageCall => call !== null)
  return calls.length > 0 ? calls : undefined
}

function isToolPartType(type: string): type is `tool-${string}` {
  return type.startsWith('tool-')
}

function isAgentDataPartType(type: string): type is `data-agent-${string}` {
  return type.startsWith('data-agent-')
}

function firstNonEmpty(...values: Array<string | undefined>) {
  return values.find(value => value && value.trim()) || ''
}

function readChapterIllustration(value: unknown): ChapterIllustration | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const data = value as Record<string, unknown>
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
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const data = value as Record<string, unknown>
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

function readInteractiveImageError(value: unknown): InteractiveImageError | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const data = value as Record<string, unknown>
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
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const data = value as Record<string, unknown>
  const rolls = Array.isArray(data.rolls)
    ? data.rolls.map(item => readNumber(item)).filter((item): item is number => item !== undefined)
    : undefined
  const stateChanges = Array.isArray(data.state_changes)
    ? data.state_changes
        .map((item) => {
          if (!item || typeof item !== 'object' || Array.isArray(item)) return null
          const change = item as Record<string, unknown>
          return {
            path: readString(change.path),
            change: readNumber(change.change) || 0,
            reason: readString(change.reason),
          }
        })
        .filter((item): item is NonNullable<typeof item> => Boolean(item?.path))
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

function interactiveImageStatus(data: Record<string, unknown>): ChatMessage['interactive_image_status'] {
  const status = readString(data.interactive_image_status) || readString(data.status)
  return status === 'running' || status === 'success' || status === 'error' ? status : undefined
}
