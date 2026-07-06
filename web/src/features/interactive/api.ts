import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { ContextAnalysis, InteractiveImage } from '@/lib/api-client'
import type { ActorStateModule, BranchSummary, DirectorPlan, DirectorPlanStatus, EventPackageModule, HotChoicesResponse, ImagePreset, InteractiveMemoryEntry, InteractiveMemoryState, InteractiveSSEEvent, OpeningRollRequest, OpeningRollResult, OpeningSelectorModule, RuleResolution, RuleResolutionRerollInput, RuleSystemModule, Snapshot, StateOp, StoryDirector, StoryMemoryStructureModule, StyleReference, StyleReferenceFileDocument, StoryImageSettings, StoryIndex, StoryMemoryRecord, StoryMemorySettings, StoryMemoryState, StoryMemoryStructure, StoryOpeningConfig, StorySummary, Teller, UpdateDirectorPlanInput } from './types'

export function getInteractiveStories(): Promise<StoryIndex> {
  return requestJSON('/api/interactive/stories')
}

export function createInteractiveStory(input: { title: string; origin?: string; story_teller_id: string; story_director_id?: string; reply_target_chars?: number; image_settings?: StoryImageSettings; opening?: StoryOpeningConfig; initial_state_ops?: StateOp[] }): Promise<StorySummary> {
  return requestJSON('/api/interactive/stories', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function rollInteractiveOpening(input: OpeningRollRequest): Promise<OpeningRollResult> {
  return requestJSON('/api/interactive/opening/roll', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateInteractiveStory(
  id: string,
  input: {
    title?: string
    story_teller_id?: string
    story_director_id?: string
    reply_target_chars?: number
    image_settings?: StoryImageSettings
    opening?: StoryOpeningConfig
  },
): Promise<StorySummary> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
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

export function getInteractiveDirectorStatus(storyId: string, branchId?: string): Promise<DirectorPlanStatus> {
  const query = branchId ? `?branch=${encodeURIComponent(branchId)}` : ''
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director/status${query}`)
}

export function updateInteractiveDirector(storyId: string, input: UpdateDirectorPlanInput): Promise<DirectorPlan> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function rebuildInteractiveDirector(storyId: string, branchId?: string): Promise<DirectorPlan> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director/rebuild`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ branch_id: branchId }),
  })
}

export function runInteractiveDirector(storyId: string, branchId?: string): Promise<DirectorPlanStatus> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director/run`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ branch_id: branchId }),
  })
}

export function analyzeInteractiveDirectorContext(storyId: string, input: { branch_id?: string; turn_id?: string } = {}): Promise<ContextAnalysis> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/director/context-analysis`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function getInteractiveMemory(storyId: string, branchId?: string, includeArchived = false): Promise<InteractiveMemoryState> {
  const params = new URLSearchParams()
  if (branchId) params.set('branch', branchId)
  if (includeArchived) params.set('include_archived', 'true')
  const query = params.toString()
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/memory${query ? `?${query}` : ''}`)
}

export function createInteractiveMemory(storyId: string, input: Partial<InteractiveMemoryEntry> & { branch_id: string }): Promise<InteractiveMemoryEntry> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/memory`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateInteractiveMemory(storyId: string, memoryId: string, input: Partial<InteractiveMemoryEntry>): Promise<InteractiveMemoryEntry> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/memory/${encodeURIComponent(memoryId)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function setInteractiveMemoryArchived(storyId: string, memoryId: string, archived: boolean): Promise<InteractiveMemoryEntry> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/memory/${encodeURIComponent(memoryId)}/archive`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ archived }),
  })
}

export function getStoryMemory(storyId: string, branchId?: string, includeArchived = false): Promise<StoryMemoryState> {
  const params = new URLSearchParams()
  if (branchId) params.set('branch', branchId)
  if (includeArchived) params.set('include_archived', 'true')
  const query = params.toString()
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/story-memory${query ? `?${query}` : ''}`)
}

export function updateStoryMemorySettings(storyId: string, input: Partial<StoryMemorySettings>): Promise<StoryMemorySettings> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/story-memory/settings`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function saveStoryMemoryStructure(storyId: string, input: Partial<StoryMemoryStructure>): Promise<StoryMemoryStructure> {
  const id = input.id?.trim()
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/story-memory/structures${id ? `/${encodeURIComponent(id)}` : ''}`, {
    method: id ? 'PATCH' : 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function deleteStoryMemoryStructure(storyId: string, structureId: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/story-memory/structures/${encodeURIComponent(structureId)}`, { method: 'DELETE' })
}

export function saveStoryMemoryRecord(storyId: string, input: Partial<StoryMemoryRecord> & { structure_id: string; branch_id?: string; values: Record<string, string> }): Promise<StoryMemoryRecord> {
  const id = input.id?.trim()
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/story-memory/records${id ? `/${encodeURIComponent(id)}` : ''}`, {
    method: id ? 'PATCH' : 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function setStoryMemoryRecordArchived(storyId: string, recordId: string, branchId: string | undefined, archived: boolean): Promise<StoryMemoryRecord> {
  const query = branchId ? `?branch=${encodeURIComponent(branchId)}` : ''
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/story-memory/records/${encodeURIComponent(recordId)}/archive${query}`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ archived }),
  })
}

export function generateStoryMemory(storyId: string, branchId?: string): Promise<StoryMemoryState> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/story-memory/generate`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ branch_id: branchId }),
  })
}

export async function generateStoryMemoryStream(storyId: string, branchId?: string, source: 'manual' | 'auto' = 'manual', signal?: AbortSignal): Promise<ReadableStream<InteractiveSSEEvent>> {
  const res = await fetchAPI(`/api/interactive/stories/${encodeURIComponent(storyId)}/story-memory/generate/stream`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ branch_id: branchId, source }),
    signal,
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
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

export function updateInteractiveTeller(id: string, input: Partial<Teller>, baseRevision?: string): Promise<Teller> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...input, base_revision: baseRevision } : input),
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

export function deleteStyleReference(path: string): Promise<void> {
  return requestJSON(`/api/styles?path=${encodeURIComponent(path)}`, { method: 'DELETE' })
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

export function updateStoryDirector(id: string, input: Partial<StoryDirector>, baseRevision?: string): Promise<StoryDirector> {
  return requestJSON(`/api/story-directors/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...input, base_revision: baseRevision } : input),
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

export function updateEventPackage(id: string, input: Partial<EventPackageModule>, baseRevision?: string): Promise<EventPackageModule> {
  return requestJSON(`/api/event-packages/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...input, base_revision: baseRevision } : input),
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

export function updateRuleSystem(id: string, input: Partial<RuleSystemModule>, baseRevision?: string): Promise<RuleSystemModule> {
  return requestJSON(`/api/rule-systems/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...input, base_revision: baseRevision } : input),
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

export function updateActorState(id: string, input: Partial<ActorStateModule>, baseRevision?: string): Promise<ActorStateModule> {
  return requestJSON(`/api/actor-states/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...input, base_revision: baseRevision } : input),
  })
}

export function deleteActorState(id: string): Promise<void> {
  return requestJSON(`/api/actor-states/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getStoryMemoryStructures(): Promise<StoryMemoryStructureModule[]> {
  const data = await requestJSON<{ story_memory_structures: StoryMemoryStructureModule[] }>('/api/story-memory-structures')
  return data.story_memory_structures || []
}

export function createStoryMemoryStructure(input: Partial<StoryMemoryStructureModule>): Promise<StoryMemoryStructureModule> {
  return requestJSON('/api/story-memory-structures', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateStoryMemoryStructure(id: string, input: Partial<StoryMemoryStructureModule>, baseRevision?: string): Promise<StoryMemoryStructureModule> {
  return requestJSON(`/api/story-memory-structures/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...input, base_revision: baseRevision } : input),
  })
}

export function deleteStoryMemoryStructurePreset(id: string): Promise<void> {
  return requestJSON(`/api/story-memory-structures/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function getOpeningSelectors(): Promise<OpeningSelectorModule[]> {
  const data = await requestJSON<{ opening_selectors: OpeningSelectorModule[] }>('/api/opening-selectors')
  return data.opening_selectors || []
}

export function createOpeningSelector(input: Partial<OpeningSelectorModule>): Promise<OpeningSelectorModule> {
  return requestJSON('/api/opening-selectors', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function updateOpeningSelector(id: string, input: Partial<OpeningSelectorModule>, baseRevision?: string): Promise<OpeningSelectorModule> {
  return requestJSON(`/api/opening-selectors/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...input, base_revision: baseRevision } : input),
  })
}

export function deleteOpeningSelector(id: string): Promise<void> {
  return requestJSON(`/api/opening-selectors/${encodeURIComponent(id)}`, {
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

export function updateImagePreset(id: string, input: Partial<ImagePreset>, baseRevision?: string): Promise<ImagePreset> {
  return requestJSON(`/api/image-presets/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...input, base_revision: baseRevision } : input),
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

export function generateInteractiveHotChoices(storyId: string, input: { branch?: string; exclude_choices?: string[]; signal?: AbortSignal }): Promise<HotChoicesResponse> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/hot-choices`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      branch: input.branch,
      exclude_choices: input.exclude_choices,
    }),
    signal: input.signal,
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
