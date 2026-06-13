import type { ChatMessage } from '@/lib/api'
import i18next from '@/i18n'
import { jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { BranchSummary, HotChoicesResponse, InteractiveSSEEvent, Snapshot, StoryIndex, StorySummary, Teller } from './types'

export function getInteractiveStories(): Promise<StoryIndex> {
  return requestJSON('/api/interactive/stories')
}

export function createInteractiveStory(input: { title: string; origin?: string; story_teller_id: string; reply_target_chars?: number }): Promise<StorySummary> {
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

export async function runInteractiveTellerAgentStream(instruction: string, tellerId = '', references: string[] = []): Promise<ReadableStream<InteractiveSSEEvent>> {
  let res: Response
  try {
    res = await fetch('/api/interactive/tellers/agent/stream', {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ instruction, teller_id: tellerId, references }),
    })
  } catch (error) {
    throw new Error(error instanceof Error && error.name === 'AbortError' ? i18next.t('settingPanel.tellerAgent.requestAborted') : i18next.t('settingPanel.tellerAgent.connectFailed'))
  }
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export function getInteractiveTellerAgentMessages(): Promise<ChatMessage[]> {
  return requestJSON('/api/interactive/tellers/agent/messages')
}

export async function clearInteractiveTellerAgentSession(): Promise<void> {
  await requestJSON('/api/interactive/tellers/agent/clear', { method: 'POST' })
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

export async function sendInteractiveMessage(input: { mode: 'story' | 'setting'; story_id: string; branch?: string; message: string; style_references?: string[]; regenerate_from_turn_id?: string; signal?: AbortSignal }): Promise<ReadableStream<InteractiveSSEEvent>> {
  const res = await fetch('/api/interactive/chat', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
    signal: input.signal,
  })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function abortInteractiveChat(): Promise<void> {
  await requestJSON('/api/interactive/chat/abort', { method: 'POST' })
}
