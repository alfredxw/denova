import { memo } from 'react'
import type { CSSProperties } from 'react'
import type { ChapterIllustration, ChatMessage, InteractiveImage, InteractiveImageError, PublicRuleRoll } from '@/lib/api'
import type { AgentMessageView, AgentPartRef } from '@/lib/agent-message-view'
import { MessageItem } from './MessageItem'

interface AgentMessageItemProps {
  view: AgentMessageView
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  subAgentPresentation?: 'card' | 'content'
  onSubmitPlanQuestion?: (ref: AgentPartRef, content: string, preview: string) => void
  onApprovePlan?: (ref: AgentPartRef) => void
  onContinuePlan?: (view: AgentMessageView) => void
  onExitPlanMode?: () => void
  onOpenTrace?: (runID: string) => void
  onPlanCardLayoutChange?: () => void
}

export const AgentMessageItem = memo(function AgentMessageItem({
  view,
  highlightDialogue = false,
  messageStyle,
  onOpenSubAgentSession,
  onInsertIllustration,
  onGenerateInteractiveImage,
  generatingInteractiveImageTurnId,
  activeSubAgentSessionKey,
  subAgentPresentation = 'card',
  onSubmitPlanQuestion,
  onApprovePlan,
  onContinuePlan,
  onExitPlanMode,
  onOpenTrace,
  onPlanCardLayoutChange,
}: AgentMessageItemProps) {
  const message = agentViewToChatMessage(view)
  if (!message) return null
  return (
    <MessageItem
      message={message}
      highlightDialogue={highlightDialogue}
      messageStyle={messageStyle}
      onOpenSubAgentSession={onOpenSubAgentSession ? () => onOpenSubAgentSession(view) : undefined}
      onInsertIllustration={onInsertIllustration}
      onGenerateInteractiveImage={onGenerateInteractiveImage ? () => onGenerateInteractiveImage(view) : undefined}
      generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
      activeSubAgentSessionKey={activeSubAgentSessionKey}
      subAgentPresentation={subAgentPresentation}
      onSubmitPlanQuestion={onSubmitPlanQuestion ? (_message, content, preview) => onSubmitPlanQuestion(view.ref, content, preview) : undefined}
      onApprovePlan={onApprovePlan ? () => onApprovePlan(view.ref) : undefined}
      onContinuePlan={onContinuePlan ? () => onContinuePlan(view) : undefined}
      onExitPlanMode={onExitPlanMode}
      onOpenTrace={onOpenTrace}
      onPlanCardLayoutChange={onPlanCardLayoutChange}
    />
  )
})

export function agentViewToChatMessage(view: AgentMessageView, options: { forceDone?: boolean } = {}): ChatMessage | null {
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
      const args = stringifyInput(view.input)
      const result = raw.state === 'output-error' ? view.errorText : stringifyOutput(view.output)
      return {
        id,
        role: 'tool_call',
        content: args ? `${view.toolName || ''}\n${args}` : view.toolName || '',
        name: view.toolName,
        args,
        status,
        result,
        illustration: readChapterIllustration(objectData(raw.toolMetadata).illustration),
        streaming,
        ...meta,
      }
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
    case 'plan-question':
      return { id, role: 'plan_question', content: view.content, status, streaming, thinking_preview: readString(data.thinking_preview), plan_action: readPlanAction(data.plan_action), ...meta }
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
    sse_hidden_fields: metadata.sse_hidden_fields,
    sse_hidden_reason: metadata.sse_hidden_reason,
    sse_display_notice: metadata.sse_display_notice,
    sse_generated_chars: metadata.sse_generated_chars,
    turn_id: metadata.turn_id,
    navigation_turn_id: metadata.navigation_turn_id,
    turn_versions: metadata.turn_versions,
    turn_version_index: metadata.turn_version_index,
  }
}

function contextFields(data: Record<string, unknown>): Partial<ChatMessage> {
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
    usage_calls: readTokenUsageCalls(data.usage_calls),
  }
}

function readTokenUsageCalls(value: unknown): ChatMessage['usage_calls'] {
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
  return calls.length ? calls : undefined
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
          if (!readString(change.path)) return null
          return { path: readString(change.path), change: readNumber(change.change) || 0, reason: readString(change.reason) }
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

function readInteractiveImageStatus(data: Record<string, unknown>): ChatMessage['interactive_image_status'] {
  const status = readString(data.interactive_image_status) || readString(data.status)
  return status === 'running' || status === 'success' || status === 'error' ? status : undefined
}

function readPlanAction(value: unknown): ChatMessage['plan_action'] {
  const action = readString(value)
  return action === 'answered' || action === 'approved' || action === 'continue' || action === 'exited' ? action : undefined
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
