import type { ChatMessage } from './api-client/types'
import { normalizeAgentUIMessages, type AgentDataParts, type AgentMessageMetadata, type AgentUIMessage } from './agent-ui'

export function chatMessagesToAgentUIMessages(messages: ChatMessage[]): AgentUIMessage[] {
  return normalizeAgentUIMessages(messages.map(chatMessageToAgentUIMessage).filter((message): message is AgentUIMessage => Boolean(message)))
}

export function chatMessageToAgentUIMessage(message: ChatMessage, index = 0): AgentUIMessage | null {
  const id = message.id || message.render_key || `legacy-${index}`
  const metadata = metadataFromChatMessage(message)
  if (message.type === 'clear') {
    return dataMessage(id, 'agent-clear', { created_at: message.created_at }, metadata)
  }
  switch (message.role) {
    case 'user':
      return textMessage(id, 'user', message.content || '', metadata)
    case 'assistant':
      return assistantMessage(id, message, metadata)
    case 'thinking':
      return {
        id,
        role: 'assistant',
        metadata,
        parts: [{ type: 'reasoning', text: message.content || '', state: message.streaming ? 'streaming' : 'done' }],
      } as AgentUIMessage
    case 'tool_call':
      return toolCallMessage(id, message, metadata)
    case 'tool_result':
      return dataMessage(id, message.interactive_image || message.interactive_images || message.interactive_image_error ? 'agent-interactive-image' : 'agent-tool-result', payloadFromChatMessage(message), metadata)
    case 'rule_roll':
      return dataMessage(id, 'agent-rule-roll', payloadFromChatMessage(message), metadata)
    case 'context_compaction':
      return dataMessage(id, 'agent-context-compaction', payloadFromChatMessage(message), metadata)
    case 'token_usage':
      return dataMessage(id, 'agent-token-usage', payloadFromChatMessage(message), metadata)
    case 'plan_question':
      return dataMessage(id, 'agent-plan-question', payloadFromChatMessage(message), metadata)
    case 'proposed_plan':
      return dataMessage(id, 'agent-proposed-plan', payloadFromChatMessage(message), metadata)
    case 'system':
      return dataMessage(id, 'agent-system', payloadFromChatMessage(message), metadata)
    case 'error':
      return dataMessage(id, 'agent-error', payloadFromChatMessage(message), metadata)
    default:
      return dataMessage(id, 'agent-activity', payloadFromChatMessage(message), metadata)
  }
}

function textMessage(id: string, role: 'user' | 'assistant', content: string, metadata?: AgentMessageMetadata, state: 'streaming' | 'done' = 'done'): AgentUIMessage | null {
  if (!content && state !== 'streaming') return null
  return {
    id,
    role,
    metadata,
    parts: [{ type: 'text', text: content, state }],
  } as AgentUIMessage
}

function assistantMessage(id: string, message: ChatMessage, metadata?: AgentMessageMetadata): AgentUIMessage | null {
  const parts: AgentUIMessage['parts'] = []
  if (message.content || message.streaming) {
    parts.push({ type: 'text', text: message.content || '', state: message.streaming ? 'streaming' : 'done' } as AgentUIMessage['parts'][number])
  }
  if (message.interactive_image || message.interactive_images?.length || message.interactive_image_error || message.interactive_image_status) {
    parts.push({
      type: 'data-agent-interactive-image',
      id: `${id}:interactive-image`,
      data: payloadFromChatMessage(message),
    } as AgentUIMessage['parts'][number])
  }
  if (parts.length === 0) return null
  return { id, role: 'assistant', metadata, parts } as AgentUIMessage
}

function toolCallMessage(id: string, message: ChatMessage, metadata?: AgentMessageMetadata): AgentUIMessage {
  const state = message.status === 'error'
    ? 'output-error'
    : message.status === 'success'
      ? 'output-available'
      : message.streaming
        ? 'input-streaming'
        : 'input-available'
  const part: Record<string, unknown> = {
    type: 'dynamic-tool',
    toolName: message.name || 'unknown_tool',
    toolCallId: id,
    state,
    input: parseJSONValue(message.args || ''),
  }
  if (message.result) {
    if (state === 'output-error') part.errorText = message.result
    else part.output = message.result
  }
  if (message.illustration) part.toolMetadata = { illustration: message.illustration }
  return { id, role: 'assistant', metadata, parts: [part] } as AgentUIMessage
}

function dataMessage(id: string, type: keyof AgentDataParts, data: Record<string, unknown>, metadata?: AgentMessageMetadata): AgentUIMessage {
  return {
    id,
    role: 'assistant',
    metadata,
    parts: [{ type: `data-${type}`, id, data }],
  } as AgentUIMessage
}

function metadataFromChatMessage(message: ChatMessage): AgentMessageMetadata | undefined {
  const metadata: AgentMessageMetadata = {
    created_at: message.created_at,
    display_role: message.role,
    run_id: message.run_id,
    agent_kind: message.agent_kind,
    agent_name: message.agent_name,
    root_agent_name: message.root_agent_name,
    run_path: message.run_path,
    subagent: message.subagent,
    subagent_session_id: message.subagent_session_id,
    subagent_type: message.subagent_type,
    sse_hidden_fields: message.sse_hidden_fields,
    sse_hidden_reason: message.sse_hidden_reason,
    sse_display_notice: message.sse_display_notice,
    sse_generated_chars: message.sse_generated_chars,
    streaming_target_content: message.streaming_target_content,
    turn_id: message.turn_id,
    navigation_turn_id: message.navigation_turn_id,
    turn_versions: message.turn_versions,
    turn_version_index: message.turn_version_index,
  }
  return Object.fromEntries(Object.entries(metadata).filter(([, value]) => value !== undefined && value !== '')) as AgentMessageMetadata
}

function payloadFromChatMessage(message: ChatMessage): Record<string, unknown> {
  return Object.fromEntries(Object.entries({
    type: message.type,
    role: message.role,
    content: message.content,
    id: message.id,
    name: message.name,
    args: message.args,
    status: message.status,
    result: message.result,
    streaming_target_content: message.streaming_target_content,
    illustration: message.illustration,
    interactive_image: message.interactive_image,
    interactive_images: message.interactive_images,
    interactive_image_error: message.interactive_image_error,
    interactive_image_status: message.interactive_image_status,
    rule_roll: message.rule_roll,
    phase: message.phase,
    attempt: message.attempt,
    tokens_before: message.tokens_before,
    tokens_after: message.tokens_after,
		projected_tokens_before: message.projected_tokens_before,
		projected_tokens_after: message.projected_tokens_after,
		reserved_completion_tokens: message.reserved_completion_tokens,
		reserved_tool_result_tokens: message.reserved_tool_result_tokens,
    context_window_tokens: message.context_window_tokens,
    threshold: message.threshold,
    target_ratio: message.target_ratio,
    epoch: message.epoch,
    source_message_count: message.source_message_count,
    message_count_before: message.message_count_before,
    message_count_after: message.message_count_after,
    skipped_reason: message.skipped_reason,
    prompt_tokens: message.prompt_tokens,
    cached_prompt_tokens: message.cached_prompt_tokens,
    uncached_prompt_tokens: message.uncached_prompt_tokens,
    cache_hit_rate: message.cache_hit_rate,
    completion_tokens: message.completion_tokens,
    reasoning_tokens: message.reasoning_tokens,
    total_tokens: message.total_tokens,
    model_calls: message.model_calls,
    generated_bytes: message.generated_bytes,
    usage_calls: message.usage_calls,
    thinking_preview: message.thinking_preview,
    plan_action: message.plan_action,
    created_at: message.created_at,
  }).filter(([, value]) => value !== undefined && value !== ''))
}

function parseJSONValue(value: string) {
  if (!value) return undefined
  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}
