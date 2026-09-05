export type InteractiveStreamEventHandling = 'handled' | 'ignored'

/**
 * Explicit backend-to-stage stream contract. Every known event must appear in
 * the consumer switch, including transport or observability events that do not
 * change the game-stage projection.
 */
export const INTERACTIVE_STREAM_EVENT_CONTRACT = {
  task_checkpoint: 'handled',
  task_checkpoint_committed: 'handled',
  task_rehydrate_required: 'handled',
  agent_cycle_started: 'handled',
  chunk: 'handled',
  subagent_settled: 'handled',
  thinking: 'handled',
  interactive_content_reclassified: 'handled',
  tool_call: 'handled',
  tool_args_delta: 'handled',
  tool_started: 'handled',
  tool_progress: 'handled',
  tool_result: 'handled',
  context_compaction: 'handled',
  token_usage: 'handled',
  interactive_turn_persisted: 'handled',
  runtime_recovery_required: 'handled',
  goal_evaluation_failed: 'handled',
  error: 'handled',
  done: 'handled',
  aborted: 'handled',
  ask_pending: 'ignored',
  ask_resolved: 'ignored',
  context_cleanup: 'ignored',
  context_normalizer: 'ignored',
  post_run_verification: 'ignored',
  run_state: 'ignored',
  subagent_artifact: 'ignored',
  subagent_transcript_synchronized: 'ignored',
  tool_target: 'ignored',
  verification: 'ignored',
  workspace_change: 'ignored',
} as const satisfies Record<string, InteractiveStreamEventHandling>

export type InteractiveStreamEventType = keyof typeof INTERACTIVE_STREAM_EVENT_CONTRACT
export type HandledInteractiveStreamEventType = {
  [Type in InteractiveStreamEventType]: typeof INTERACTIVE_STREAM_EVENT_CONTRACT[Type] extends 'handled' ? Type : never
}[InteractiveStreamEventType]
export type IgnoredInteractiveStreamEventType = Exclude<InteractiveStreamEventType, HandledInteractiveStreamEventType>

export function isInteractiveStreamEventType(value: string): value is InteractiveStreamEventType {
  return Object.prototype.hasOwnProperty.call(INTERACTIVE_STREAM_EVENT_CONTRACT, value)
}

export function isHandledInteractiveStreamEventType(value: InteractiveStreamEventType): value is HandledInteractiveStreamEventType {
  return INTERACTIVE_STREAM_EVENT_CONTRACT[value] === 'handled'
}
