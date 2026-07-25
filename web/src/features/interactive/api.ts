import { APIError, createAgentCommandID, fetchAPI, jsonHeaders, parseSSEStream, requestJSON } from '@/lib/api-client'
import type { AgentCommandReceipt, AgentRuntimeActiveOutput, AgentRuntimeOpenTool, AgentRuntimeOperation, AgentRuntimeQueuedCommand, AgentRuntimeRecoveryAction, AgentRuntimeRecoveryReceipt, ContextAnalysis, InteractiveImage } from '@/lib/api-client'
import { isKnownAgentCommandOutcome } from '@/lib/agent-command'
import type { ActorStateModule, ActorTraitRollRequest, ActorTraitRollResult, BranchSummary, DirectorPlan, DirectorPlanStatus, EventPackageModule, ImagePreset, InitialActorTraitRoll, InteractiveSSEEvent, RuleResolution, RuleResolutionRerollInput, RuleSystemModule, Snapshot, StoryDirector, StoryDirectorModuleRefs, StoryDirectorRunPolicy, StoryHistoryPage, StoryStateSchemaPolicy, StyleReference, StyleReferenceFileDocument, StoryImageSettings, StoryIndex, StoryOpeningConfig, StorySummary, Teller, UpdateDirectorPlanInput, UpdateTurnNarrativeResult } from './types'

function presetMutationBody<T extends object>(input: T, baseRevision?: string, workspace?: string) {
  return {
    ...input,
    ...(baseRevision ? { base_revision: baseRevision } : {}),
    ...(workspace ? { workspace } : {}),
  }
}

export function getInteractiveStories(): Promise<StoryIndex> {
  return requestJSON('/api/interactive/stories')
}

export function createInteractiveStory(input: { title: string; origin?: string; story_teller_id: string; story_director_id?: string; director_run_policy?: StoryDirectorRunPolicy; module_refs?: StoryDirectorModuleRefs; reply_target_chars?: number; choice_count?: number; image_settings?: StoryImageSettings; opening?: StoryOpeningConfig; initial_trait_rolls?: InitialActorTraitRoll[]; state_schema_policy?: StoryStateSchemaPolicy }): Promise<StorySummary> {
  return requestJSON('/api/interactive/stories', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function rollInteractiveActorTraits(input: ActorTraitRollRequest): Promise<ActorTraitRollResult> {
  return requestJSON('/api/interactive/actor-traits/roll', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateInteractiveStory(
  id: string,
  input: {
    title?: string
    origin?: string
    story_teller_id?: string
    story_director_id?: string
    director_run_policy?: StoryDirectorRunPolicy
    module_refs?: StoryDirectorModuleRefs
    reply_target_chars?: number
    choice_count?: number
    image_settings?: StoryImageSettings
    opening?: StoryOpeningConfig
    state_schema_policy?: StoryStateSchemaPolicy
  },
): Promise<StorySummary> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function selectInteractiveStory(id: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}/select`, {
    method: 'POST',
  })
}

export function deleteInteractiveStory(id: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function getInteractiveSnapshot(storyId: string, branchId?: string): Promise<Snapshot> {
  const query = branchId ? `?branch=${encodeURIComponent(branchId)}` : ''
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/snapshot${query}`)
}

export function getInteractiveHistoryPage(storyId: string, branchId: string, before: string, limit = 100): Promise<StoryHistoryPage> {
  const query = new URLSearchParams({ branch: branchId, before, limit: String(limit) })
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/history?${query.toString()}`)
}

export function rerollInteractiveRuleResolution(storyId: string, resolutionId: string, input: RuleResolutionRerollInput = {}): Promise<RuleResolution> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/rules/resolutions/${encodeURIComponent(resolutionId)}/reroll`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function getInteractiveDirector(storyId: string, branchId?: string): Promise<DirectorPlan> {
  const query = branchId ? `?branch=${encodeURIComponent(branchId)}` : ''
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director${query}`)
}

export function updateInteractiveDirector(storyId: string, input: UpdateDirectorPlanInput): Promise<DirectorPlan> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function rebuildInteractiveDirector(storyId: string, branchId?: string, options: { resetEvents?: boolean } = {}): Promise<DirectorPlan> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director/rebuild`, {
    method: 'POST',
    headers: jsonHeaders,
		body: JSON.stringify({ branch_id: branchId, ...(options.resetEvents ? { reset_events: true } : {}) }),
  })
}

export function runInteractiveDirector(storyId: string, branchId?: string, options: { forceEventEvaluation?: boolean } = {}): Promise<DirectorPlanStatus> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director/run`, {
    method: 'POST',
    headers: jsonHeaders,
		body: JSON.stringify({ branch_id: branchId, ...(options.forceEventEvaluation ? { force_event_evaluation: true } : {}) }),
  })
}

export function analyzeInteractiveDirectorContext(storyId: string, input: { branch_id?: string; turn_id?: string } = {}): Promise<ContextAnalysis> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director/context-analysis`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function getInteractiveTellers(): Promise<Teller[]> {
  const data = await requestJSON<{ tellers: Teller[] }>('/api/interactive/tellers')
  return data.tellers || []
}

export function createInteractiveTeller(input: Partial<Teller>): Promise<Teller> {
  return requestJSON('/api/interactive/tellers', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateInteractiveTeller(id: string, input: Partial<Teller>, baseRevision?: string, workspace?: string): Promise<Teller> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision, workspace)),
  })
}

export function deleteInteractiveTeller(id: string): Promise<void> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getStyleReferences(): Promise<StyleReference[]> {
  const data = await requestJSON<{ styles: StyleReference[] }>('/api/styles')
  return data.styles || []
}

export function saveStyleReference(input: { name: string; description?: string; filename?: string; content: string }): Promise<StyleReference> {
  return requestJSON('/api/styles', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function readStyleReferenceFile(path: string): Promise<StyleReferenceFileDocument> {
  return requestJSON(`/api/styles/file?path=${encodeURIComponent(path)}`)
}

export function updateStyleReferenceFile(input: { path: string; content: string; base_revision?: string }): Promise<StyleReferenceFileDocument> {
  return requestJSON('/api/styles/file', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function getStoryDirectors(): Promise<StoryDirector[]> {
  const data = await requestJSON<{ directors: StoryDirector[] }>('/api/story-directors')
  return data.directors || []
}

export function createStoryDirector(input: Partial<StoryDirector>): Promise<StoryDirector> {
  return requestJSON('/api/story-directors', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateStoryDirector(id: string, input: Partial<StoryDirector>, baseRevision?: string, workspace?: string): Promise<StoryDirector> {
  return requestJSON(`/api/story-directors/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision, workspace)),
  })
}

export function deleteStoryDirector(id: string): Promise<void> {
  return requestJSON(`/api/story-directors/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getEventPackages(): Promise<EventPackageModule[]> {
  const data = await requestJSON<{ event_packages: EventPackageModule[] }>('/api/event-packages')
  return data.event_packages || []
}

export function createEventPackage(input: Partial<EventPackageModule>): Promise<EventPackageModule> {
  return requestJSON('/api/event-packages', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateEventPackage(id: string, input: Partial<EventPackageModule>, baseRevision?: string, workspace?: string): Promise<EventPackageModule> {
  return requestJSON(`/api/event-packages/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision, workspace)),
  })
}

export function deleteEventPackage(id: string): Promise<void> {
  return requestJSON(`/api/event-packages/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getRuleSystems(): Promise<RuleSystemModule[]> {
  const data = await requestJSON<{ rule_systems: RuleSystemModule[] }>('/api/rule-systems')
  return data.rule_systems || []
}

export function createRuleSystem(input: Partial<RuleSystemModule>): Promise<RuleSystemModule> {
  return requestJSON('/api/rule-systems', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateRuleSystem(id: string, input: Partial<RuleSystemModule>, baseRevision?: string, workspace?: string): Promise<RuleSystemModule> {
  return requestJSON(`/api/rule-systems/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision, workspace)),
  })
}

export function deleteRuleSystem(id: string): Promise<void> {
  return requestJSON(`/api/rule-systems/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getActorStates(): Promise<ActorStateModule[]> {
  const data = await requestJSON<{ actor_states: ActorStateModule[] }>('/api/actor-states')
  return data.actor_states || []
}

export function createActorState(input: Partial<ActorStateModule>): Promise<ActorStateModule> {
  return requestJSON('/api/actor-states', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateActorState(id: string, input: Partial<ActorStateModule>, baseRevision?: string, workspace?: string): Promise<ActorStateModule> {
  return requestJSON(`/api/actor-states/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision, workspace)),
  })
}

export function deleteActorState(id: string): Promise<void> {
  return requestJSON(`/api/actor-states/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getImagePresets(): Promise<ImagePreset[]> {
  const data = await requestJSON<{ presets: ImagePreset[] }>('/api/image-presets')
  return data.presets || []
}

export function createImagePreset(input: Partial<ImagePreset>): Promise<ImagePreset> {
  return requestJSON('/api/image-presets', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateImagePreset(id: string, input: Partial<ImagePreset>, baseRevision?: string, workspace?: string): Promise<ImagePreset> {
  return requestJSON(`/api/image-presets/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(presetMutationBody(input, baseRevision, workspace)),
  })
}

export function deleteImagePreset(id: string): Promise<void> {
  return requestJSON(`/api/image-presets/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getInteractiveBranches(storyId: string): Promise<BranchSummary[]> {
  const data = await requestJSON<{ branches: BranchSummary[] }>(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches`)
  return data.branches || []
}

export function createInteractiveBranch(storyId: string, input: { parent_event_id: string; title: string }): Promise<BranchSummary> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function deleteInteractiveBranch(storyId: string, branchId: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches/${encodeURIComponent(branchId)}`, { method: 'DELETE' })
}

export function switchInteractiveBranch(storyId: string, branchId: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/switch-branch`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ branch_id: branchId }),
  })
}

export function switchInteractiveTurnVersion(storyId: string, input: { branch_id: string; turn_id: string; version_turn_id: string }): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/switch-turn-version`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateInteractiveTurnNarrative(storyId: string, turnId: string, input: { branch_id: string; narrative: string; expected_narrative: string }): Promise<UpdateTurnNarrativeResult> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/turns/${encodeURIComponent(turnId)}/narrative`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function generateInteractiveImage(storyId: string, input: { command_id: string; branch_id?: string; turn_id: string; source: 'manual' | 'auto'; force?: boolean }): Promise<{ enabled?: boolean; skipped?: boolean; skipped_reason?: string; image?: InteractiveImage }> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/images/generate`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export interface InteractiveStartInput {
  command_id: string
  mode: 'story' | 'setting'
  story_id: string
  branch?: string
  message: string
  style_scenes?: string[]
  regenerate_from_turn_id?: string
  signal?: AbortSignal
}

export async function sendInteractiveMessage(input: InteractiveStartInput): Promise<ReadableStream<InteractiveSSEEvent>> {
  const { signal, ...body } = input
  const res = await fetchAPI('/api/interactive/chat', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(body),
    signal,
  })
  if (!res.ok) throw await interactiveStreamResponseError(res)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

async function interactiveStreamResponseError(res: Response) {
  const text = await res.text()
  let payload: Record<string, unknown> = {}
  if (text) {
    try {
      payload = JSON.parse(text) as Record<string, unknown>
    } catch {
      payload = { error: text }
    }
  }
  const message = typeof payload.error === 'string' && payload.error ? payload.error : `HTTP ${res.status}`
  const code = typeof payload.code === 'string' && payload.code ? payload.code : undefined
  const details = payload.details && typeof payload.details === 'object' && !Array.isArray(payload.details)
    ? payload.details as Record<string, unknown>
    : undefined
  return new APIError(message, { status: res.status, code, details, payload })
}

export interface ActiveInteractiveChat {
  active: boolean
  status?: 'running' | 'done' | 'aborted' | 'error'
  task_id?: string
  command_id?: string
  story_id?: string
  branch_id?: string
  message?: string
  regenerate_from_turn_id?: string
  /** Diagnostic-only latest raw SSE cursor; runtime `cursor` is durable state. */
  stream_cursor?: number
  cursor?: number
  phase?: 'idle' | 'running' | string
  recovery_paused?: boolean
  runtime_recoverable?: boolean
  stream_attached?: boolean
  recovery_actions?: AgentRuntimeRecoveryAction[]
  active_operation_id?: string
  active_cycle?: number
  active_output?: AgentRuntimeActiveOutput
  queue?: AgentRuntimeQueuedCommand[]
  open_tools?: AgentRuntimeOpenTool[]
  last_operation?: AgentRuntimeOperation
}

export interface InteractiveAgentCommand {
  type: 'abort'
  commandId: string
  targetOperationId: string
  storyId: string
  branchId?: string
  reason?: string
}

/** Abort the exact game operation shown by the stage. */
export function submitInteractiveAgentCommand(command: InteractiveAgentCommand): Promise<AgentCommandReceipt> {
  return requestJSON('/api/interactive/chat/commands', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      type: command.type,
      command_id: command.commandId,
      target_operation_id: command.targetOperationId,
      story_id: command.storyId,
      ...(command.branchId ? { branch_id: command.branchId } : {}),
      ...(command.reason ? { reason: command.reason } : {}),
    }),
  })
}

export function getActiveInteractiveChat(storyId: string, branchId?: string): Promise<ActiveInteractiveChat> {
  return requestJSON(`/api/interactive/chat/active?${interactiveChatQuery(storyId, branchId)}`)
}

/** Recover accepted game work by server-projected identity, never by browser-cached input. */
export function recoverInteractiveAgentRuntime(input: {
  storyId: string
  branchId?: string
  action: AgentRuntimeRecoveryAction
}): Promise<AgentRuntimeRecoveryReceipt> {
  return requestJSON('/api/interactive/chat/recovery', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      action: input.action,
      story_id: input.storyId,
      ...(input.branchId ? { branch: input.branchId } : {}),
    }),
  })
}

export async function streamActiveInteractiveChat(input: { storyId: string; branchId?: string; taskId?: string; after?: string; signal?: AbortSignal }): Promise<ReadableStream<InteractiveSSEEvent>> {
  const taskId = input.taskId?.trim()
  if (!taskId) throw new Error('Cannot reconnect without an exact Agent stream task')
  const res = await fetchAPI(`/api/interactive/chat/stream?${interactiveChatQuery(input.storyId, input.branchId, taskId, input.after)}`, { signal: input.signal })
  if (!res.ok) throw await interactiveStreamResponseError(res)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

function interactiveChatQuery(storyId: string, branchId?: string, taskId?: string, after?: string) {
  const params = new URLSearchParams({ story_id: storyId })
  if (branchId) params.set('branch', branchId)
  if (taskId) params.set('task_id', taskId)
  if (after) params.set('after', after)
  return params.toString()
}

export function analyzeInteractiveContext(input: { mode: 'story'; story_id: string; branch?: string; message: string; style_scenes?: string[] }): Promise<ContextAnalysis> {
  return requestJSON('/api/interactive/chat/context-analysis', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

const interactiveStructuralCommandIDs = new Map<string, string>()

export async function compactInteractiveContext(storyId: string, branchId?: string): Promise<void> {
  const key = `compact:${storyId}:${branchId ?? ''}`
  const commandId = interactiveStructuralCommandIDs.get(key) ?? createAgentCommandID()
  interactiveStructuralCommandIDs.set(key, commandId)
  try {
    await requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/context-compaction`, {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ command_id: commandId, branch_id: branchId }),
    })
    interactiveStructuralCommandIDs.delete(key)
  } catch (error) {
    if (isKnownAgentCommandOutcome(error)) interactiveStructuralCommandIDs.delete(key)
    throw error
  }
}

export async function removeInteractiveContextCompaction(storyId: string, branchId?: string): Promise<boolean> {
  const key = `remove:${storyId}:${branchId ?? ''}`
  const commandId = interactiveStructuralCommandIDs.get(key) ?? createAgentCommandID()
  interactiveStructuralCommandIDs.set(key, commandId)
  const query = new URLSearchParams({ command_id: commandId })
  if (branchId) query.set('branch', branchId)
  try {
    const data = await requestJSON<{ removed?: boolean }>(`/api/interactive/stories/${encodeURIComponent(storyId)}/context-compaction/active?${query}`, {
      method: 'DELETE',
    })
    interactiveStructuralCommandIDs.delete(key)
    return Boolean(data.removed)
  } catch (error) {
    if (isKnownAgentCommandOutcome(error)) interactiveStructuralCommandIDs.delete(key)
    throw error
  }
}
