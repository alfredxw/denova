import type { ChatMessage } from '@/lib/api'
import type { BranchSummary, InteractiveSSEEvent, Snapshot, StoryIndex, StorySummary, Teller } from './types'

async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
  return data
}

export function getInteractiveStories(): Promise<StoryIndex> {
  return requestJSON('/api/interactive/stories')
}

export function createInteractiveStory(input: { title: string; origin?: string; story_teller_id: string }): Promise<StorySummary> {
  return requestJSON('/api/interactive/stories', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function updateInteractiveStory(id: string, input: { title?: string; story_teller_id?: string }): Promise<StorySummary> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function deleteInteractiveStory(id: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(id)}`, { method: 'DELETE' })
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
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function updateInteractiveTeller(id: string, input: Partial<Teller>): Promise<Teller> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function deleteInteractiveTeller(id: string): Promise<void> {
  return requestJSON(`/api/interactive/tellers/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function runInteractiveTellerAgentStream(instruction: string, tellerId = ''): Promise<ReadableStream<InteractiveSSEEvent>> {
  let res: Response
  try {
    res = await fetch('/api/interactive/tellers/agent/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ instruction, teller_id: tellerId }),
    })
  } catch (error) {
    throw new Error(error instanceof Error && error.name === 'AbortError'
      ? '讲述者 Agent 请求已中断'
      : '讲述者 Agent 连接后端失败，请确认后端已重启并加载最新代码')
  }
  if (!res.ok) {
    let message = `HTTP ${res.status}`
    try {
      const data = await res.json()
      message = data.error || message
    } catch {
      // keep HTTP fallback
    }
    throw new Error(message)
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
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function deleteInteractiveBranch(storyId: string, branchId: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches/${encodeURIComponent(branchId)}`, { method: 'DELETE' })
}

export function switchInteractiveBranch(storyId: string, branchId: string): Promise<void> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/switch-branch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branch_id: branchId }),
  })
}

export async function sendInteractiveMessage(input: {
  mode: 'story' | 'setting'
  story_id: string
  branch?: string
  message: string
  signal?: AbortSignal
}): Promise<ReadableStream<InteractiveSSEEvent>> {
  const res = await fetch('/api/interactive/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
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

function parseSSEStream(body: ReadableStream<Uint8Array>): ReadableStream<InteractiveSSEEvent> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  return new ReadableStream<InteractiveSSEEvent>({
    async pull(controller) {
      while (true) {
        const { done, value } = await reader.read()
        if (done) {
          controller.close()
          return
        }
        buffer += decoder.decode(value, { stream: true })
        const events = buffer.split('\n\n')
        buffer = events.pop() || ''
        for (const eventStr of events) {
          if (!eventStr.trim()) continue
          const lines = eventStr.split('\n')
          let event = ''
          let data = ''
          for (const line of lines) {
            if (line.startsWith('event: ')) event = line.slice(7)
            if (line.startsWith('data: ')) data = line.slice(6)
          }
          if (event) controller.enqueue({ event, data })
        }
      }
    },
  })
}
