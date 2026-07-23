import type { Snapshot } from '../../types'

export type LiveTurnRenderKeys = {
  user: string
  assistant: string
}

export function createLiveTurnRenderKeys(): LiveTurnRenderKeys {
  const id = `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return {
    user: `interactive-live-user-${id}`,
    assistant: `interactive-live-assistant-${id}`,
  }
}

export function storyStageSnapshotKey(storyId: string, branchId: string, snapshot?: Snapshot | null) {
  const turns = snapshot?.turns || []
  return `${storyId || snapshot?.story_id || 'none'}:${snapshot?.branch_id || branchId || 'main'}:${turns[turns.length - 1]?.id || 'empty'}`
}

export function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}

export function normalizeMessageContent(value: string) {
  return value.replace(/\r\n/g, '\n').trim()
}

export function parseInlineStyleScenes(input: string): string[] {
  const result = new Set<string>()
  const regex = /(?:^|\s)#([^\s#]+)/g
  let match: RegExpExecArray | null
  while ((match = regex.exec(input)) !== null) result.add(match[1])
  return Array.from(result)
}

export function isNativeComposingKeyboardEvent(event: KeyboardEvent) {
  return event.isComposing || event.key === 'Process' || event.keyCode === 229
}
