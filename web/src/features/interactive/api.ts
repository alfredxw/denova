import { fetchAPI, jsonHeaders, parseSSEStream, requestJSON } from '@/lib/api-client'
import type { ContextAnalysis, InteractiveImage } from '@/lib/api-client'
import type { ActorStateModule, ActorTraitRollRequest, ActorTraitRollResult, BranchSummary, DirectorPlan, DirectorPlanStatus, EventPackageModule, ImagePreset, InitialActorTraitRoll, InteractiveSSEEvent, RuleResolution, RuleResolutionRerollInput, RuleSystemModule, Snapshot, StoryDirector, StoryDirectorModuleRefs, StoryStateSchemaPolicy, StyleReference, StyleReferenceFileDocument, StoryImageSettings, StoryIndex, StoryOpeningConfig, StorySummary, Teller, UpdateDirectorPlanInput, UpdateTurnNarrativeResult } from './types'

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

export function createInteractiveStory(input: { title: string; origin?: string; story_teller_id: string; story_director_id?: string; module_refs?: StoryDirectorModuleRefs; reply_target_chars?: number; choice_count?: number; image_settings?: StoryImageSettings; opening?: StoryOpeningConfig; initial_trait_rolls?: InitialActorTraitRoll[]; state_schema_policy?: StoryStateSchemaPolicy }): Promise<StorySummary> {
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

export function generateInteractiveImage(storyId: string, input: { branch_id?: string; turn_id: string; source: 'manual' | 'auto'; force?: boolean }): Promise<{ enabled?: boolean; skipped?: boolean; skipped_reason?: string; image?: InteractiveImage }> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/images/generate`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function sendInteractiveMessage(input: { mode: 'story' | 'setting'; story_id: string; branch?: string; message: string; style_scenes?: string[]; regenerate_from_turn_id?: string; signal?: AbortSignal }): Promise<ReadableStream<InteractiveSSEEvent>> {
  const res = await fetchAPI('/api/interactive/chat', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
    signal: input.signal,
  })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export interface ActiveInteractiveChat {
  active: boolean
  status?: 'running' | 'done' | 'aborted' | 'error'
  task_id?: string
  story_id?: string
  branch_id?: string
  message?: string
  regenerate_from_turn_id?: string
}

export function getActiveInteractiveChat(storyId: string, branchId?: string): Promise<ActiveInteractiveChat> {
  return requestJSON(`/api/interactive/chat/active?${interactiveChatQuery(storyId, branchId)}`)
}

export async function streamActiveInteractiveChat(input: { storyId: string; branchId?: string; taskId?: string; signal?: AbortSignal }): Promise<ReadableStream<InteractiveSSEEvent>> {
  const res = await fetchAPI(`/api/interactive/chat/stream?${interactiveChatQuery(input.storyId, input.branchId, input.taskId)}`, { signal: input.signal })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

function interactiveChatQuery(storyId: string, branchId?: string, taskId?: string) {
  const params = new URLSearchParams({ story_id: storyId })
  if (branchId) params.set('branch', branchId)
  if (taskId) params.set('task_id', taskId)
  return params.toString()
}

export function analyzeInteractiveContext(input: { mode: 'story'; story_id: string; branch?: string; message: string; style_scenes?: string[] }): Promise<ContextAnalysis> {
  return requestJSON('/api/interactive/chat/context-analysis', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function compactInteractiveContext(storyId: string, branchId?: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/context-compaction`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ branch_id: branchId }),
  })
}

export async function removeInteractiveContextCompaction(storyId: string, branchId?: string): Promise<boolean> {
  const query = branchId ? `?branch=${encodeURIComponent(branchId)}` : ''
  const data = await requestJSON<{ removed?: boolean }>(`/api/interactive/stories/${encodeURIComponent(storyId)}/context-compaction/active${query}`, {
    method: 'DELETE',
  })
  return Boolean(data.removed)
}

export async function abortInteractiveChat(): Promise<void> {
  await requestJSON('/api/interactive/chat/abort', { method: 'POST' })
}
