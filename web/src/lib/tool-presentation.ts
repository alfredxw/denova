import type { ChatMessage, ToolPresentation, ToolPresentationKind } from './api-client/types'

const TOOL_PRESENTATION_KINDS: Record<ToolPresentationKind, true> = {
  generic: true,
  file: true,
  search: true,
  terminal: true,
  web: true,
  browser: true,
  image: true,
  interactive_media: true,
  todo: true,
  interaction: true,
  delegation: true,
  script: true,
}

export type ToolCallRenderer = 'generic' | 'image' | 'interactive_media' | 'todo' | 'interaction'
export type ToolResultRenderer = 'generic' | 'image' | 'interactive_media'

/** Accepts only the backend-owned presentation vocabulary at the UI boundary. */
export function readToolPresentation(value: unknown): ToolPresentation | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const raw = value as Record<string, unknown>
  if (!isToolPresentationKind(raw.call) || !isToolPresentationKind(raw.result)) return undefined
  return { call: raw.call, result: raw.result }
}

export function toolPresentationKind(message: ChatMessage, phase: 'call' | 'result'): ToolPresentationKind {
  return message.tool_presentation?.[phase] || 'generic'
}

/** Maps every presentation kind explicitly so vocabulary growth cannot silently fall back. */
export function toolCallRenderer(message: ChatMessage): ToolCallRenderer {
  const kind = toolPresentationKind(message, 'call')
  switch (kind) {
    case 'image':
    case 'interactive_media':
    case 'todo':
    case 'interaction':
      return kind
    case 'generic':
    case 'file':
    case 'search':
    case 'terminal':
    case 'web':
    case 'browser':
    case 'delegation':
    case 'script':
      return 'generic'
    default: {
      const exhaustive: never = kind
      return exhaustive
    }
  }
}

/** Result cards currently specialize media while preserving the full vocabulary. */
export function toolResultRenderer(message: ChatMessage): ToolResultRenderer {
  const kind = toolPresentationKind(message, 'result')
  switch (kind) {
    case 'image':
    case 'interactive_media':
      return kind
    case 'generic':
    case 'file':
    case 'search':
    case 'terminal':
    case 'web':
    case 'browser':
    case 'todo':
    case 'interaction':
    case 'delegation':
    case 'script':
      return 'generic'
    default: {
      const exhaustive: never = kind
      return exhaustive
    }
  }
}

function isToolPresentationKind(value: unknown): value is ToolPresentationKind {
  return typeof value === 'string' && Object.hasOwn(TOOL_PRESENTATION_KINDS, value)
}
