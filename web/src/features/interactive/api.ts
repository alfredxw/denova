import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { ContextAnalysis, InteractiveImage } from '@/lib/api-client'
import type { BranchSummary, HotChoicesResponse, ImagePreset, InteractiveMemoryEntry, InteractiveMemoryState, InteractiveSSEEvent, Snapshot, StoryImageSettings, StoryIndex, StoryMemoryRecord, StoryMemorySettings, StoryMemoryState, StoryMemoryStructure, StoryOpeningConfig, StorySummary, Teller } from './types'

export function getInteractiveStories(): Promise<StoryIndex> {
  return requestJSON('/api/interactive/stories')
}

export function createInteractiveStory(input: { title: string; origin?: string; story_teller_id: string; reply_target_chars?: number; image_settings?: StoryImageSettings; opening?: StoryOpeningConfig }): Promise<StorySummary> {
  return requestJSON('/api/interactive/stories', {
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

export function updateInteractiveTeller(id: string, input: Partial<Teller>): Promise<Teller> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function deleteInteractiveTeller(id: string): Promise<void> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, {
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

export function updateImagePreset(id: string, input: Partial<ImagePreset>): Promise<ImagePreset> {
  return requestJSON(`/api/image-presets/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(input),
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
