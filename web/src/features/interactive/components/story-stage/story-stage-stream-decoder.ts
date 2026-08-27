import { z } from 'zod'
import type { InteractiveSSEEvent, InteractiveTurnPersistedEvent } from '../../types'
import {
  isHandledInteractiveStreamEventType,
  isInteractiveStreamEventType,
  type HandledInteractiveStreamEventType,
  type IgnoredInteractiveStreamEventType,
} from './story-stage-stream-events'

const metadataSchema = z.object({
  run_id: z.string().optional(),
  display_segment_id: z.string().optional(),
  display_phase: z.enum(['candidate', 'progress', 'final', 'partial']).optional(),
  agent_kind: z.string().optional(),
  agent_name: z.string().optional(),
  root_agent_name: z.string().optional(),
  run_path: z.array(z.string()).optional(),
  subagent: z.union([z.boolean(), z.literal('true'), z.literal('false')]).optional(),
  subagent_session_id: z.string().optional(),
  subagent_type: z.string().optional(),
	parent_call_id: z.string().optional(),
}).passthrough()

const contentSchema = metadataSchema.extend({ content: z.string().default('') })
const toolIdentitySchema = metadataSchema.extend({
  id: z.string().optional(),
  index: z.union([z.number(), z.string()]).optional(),
  name: z.string().optional(),
})

const handledEventSchemas = {
  task_checkpoint: z.object({
    complete: z.boolean().optional(),
    cursor: z.number().optional(),
    status: z.enum(['running', 'done', 'aborted', 'error']).optional(),
    terminal_reason: z.string().optional(),
  }).passthrough(),
  task_checkpoint_committed: z.object({}).passthrough(),
  task_rehydrate_required: z.object({
    cursor: z.number().int().nonnegative(),
    persistence_required: z.boolean().optional(),
    settled: z.boolean().optional(),
    status: z.enum(['running', 'done', 'aborted', 'error']).optional(),
    task_id: z.string().trim().min(1),
    terminal_reason: z.string().optional(),
  }).passthrough(),
  agent_cycle_started: z.object({
    command_id: z.string(),
    delivery: z.enum(['start_turn', 'steer', 'follow_up', 'next_turn']),
    message: z.string(),
    operation_id: z.string(),
    cycle: z.number(),
  }).passthrough(),
  chunk: contentSchema,
  thinking: contentSchema,
  interactive_content_reclassified: contentSchema,
  tool_call: toolIdentitySchema.extend({ args: z.string().optional() }),
  tool_args_delta: toolIdentitySchema.extend({
    args: z.string().optional(),
    delta: z.string().optional(),
  }),
  tool_started: toolIdentitySchema,
  tool_progress: toolIdentitySchema,
  tool_result: toolIdentitySchema.extend({ content: z.string().default('') }),
  context_compaction: z.object({ status: z.string().optional() }).passthrough(),
  token_usage: z.object({ run_id: z.string().optional() }).passthrough(),
  interactive_turn_persisted: z.custom<InteractiveTurnPersistedEvent>(isInteractiveTurnPersistedEvent),
  runtime_recovery_required: z.object({}).passthrough(),
  goal_evaluation_failed: z.object({
    code: z.literal('agent_runtime.goal_evaluation_failed'),
    detail: z.string().optional(),
    message: z.string().optional(),
  }).passthrough(),
  error: z.object({
    code: z.string().optional(),
    error: z.string().optional(),
    message: z.string().optional(),
    request_id: z.string().optional(),
  }).passthrough(),
  done: z.object({}).passthrough(),
  aborted: z.object({
    message: z.string().optional(),
    reason: z.string().optional(),
  }).passthrough(),
} as const satisfies Record<HandledInteractiveStreamEventType, z.ZodTypeAny>

export type DecodedHandledStreamEvent = {
  [Type in keyof typeof handledEventSchemas]: {
    type: Type
    data: z.infer<(typeof handledEventSchemas)[Type]>
  }
}[keyof typeof handledEventSchemas]

export type InteractiveStreamDecodeResult =
  | { kind: 'handled'; event: DecodedHandledStreamEvent }
  | { kind: 'ignored'; type: IgnoredInteractiveStreamEventType }
  | { kind: 'unknown'; type: string }
  | { kind: 'invalid'; type: HandledInteractiveStreamEventType; issue: string }

/** Parses and validates one event before any payload can mutate stage state. */
export function decodeInteractiveStreamEvent(event: InteractiveSSEEvent): InteractiveStreamDecodeResult {
  const type = event.event
  if (!isInteractiveStreamEventType(type)) return { kind: 'unknown', type }
  if (!isHandledInteractiveStreamEventType(type)) return { kind: 'ignored', type }

  let raw: unknown
  try {
    raw = JSON.parse(event.data)
  } catch (error) {
    return { kind: 'invalid', type, issue: error instanceof Error ? error.message : 'invalid JSON' }
  }

  const parsed = handledEventSchemas[type].safeParse(raw)
  if (!parsed.success) {
    return { kind: 'invalid', type, issue: parsed.error.issues.map(issue => issue.message).join('; ') }
  }

  // TypeScript cannot preserve the key/schema correlation across indexed map
  // access. safeParse has already established that exact runtime relationship.
  return { kind: 'handled', event: { type, data: parsed.data } as DecodedHandledStreamEvent }
}

function isInteractiveTurnPersistedEvent(value: unknown): value is InteractiveTurnPersistedEvent {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const event = value as Record<string, unknown>
  if (typeof event.story_id !== 'string' || typeof event.branch_id !== 'string') return false
  if (typeof event.turn_count !== 'number' || !Number.isFinite(event.turn_count)) return false
  if (!event.turn || typeof event.turn !== 'object' || Array.isArray(event.turn)) return false
  return typeof (event.turn as Record<string, unknown>).id === 'string'
}
