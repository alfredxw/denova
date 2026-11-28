import { APIError, fetchAPI, jsonHeaders, parseSSEStream, responseAPIError } from './client'
import { projectAPIPath } from './project-scope'

export type InlineEditMode = 'writing' | 'lore' | 'game_turn'

export interface InlineEditMeta {
  model: string
  style_name?: string
  lore_count: number
}

export interface InlineEditStreamRequest {
  projectId: string
  mode: InlineEditMode
  instruction: string
  selection?: string
  before?: string
  after?: string
  previous_result?: string
  lore_ids?: string[]
  include_lore?: boolean
  include_style?: boolean
  story_id?: string
}

export interface InlineEditStreamCallbacks {
  onMeta?: (meta: InlineEditMeta) => void
  onDelta?: (delta: string) => void
  onDone?: (finalText: string) => void
}

/**
 * Streams one stateless, tool-free localized prose edit over SSE.
 * Throws APIError (including machine-readable backend `code`) when preparation
 * or provider streaming fails.
 */
export async function streamInlineEdit(
  request: InlineEditStreamRequest,
  callbacks: InlineEditStreamCallbacks = {},
  signal?: AbortSignal,
): Promise<string> {
  const { projectId, ...payload } = request
  const res = await fetchAPI(projectAPIPath(projectId, '/inline-edit/stream'), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(payload),
    signal,
  })
  if (!res.ok) {
    throw await responseAPIError(res)
  }
  if (!res.body) {
    throw new APIError('Empty inline edit stream response', { status: res.status })
  }

  let accumulated = ''
  let finalText: string | null = null
  const stream = parseSSEStream(res.body)
  const reader = stream.getReader()
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (!value) continue

      let parsed: Record<string, unknown> = {}
      if (value.data) {
        try {
          parsed = JSON.parse(value.data) as Record<string, unknown>
        } catch {
          parsed = {}
        }
      }

      if (value.event === 'meta') {
        callbacks.onMeta?.({
          model: typeof parsed.model === 'string' ? parsed.model : '',
          style_name: typeof parsed.style_name === 'string' ? parsed.style_name : undefined,
          lore_count: typeof parsed.lore_count === 'number' ? parsed.lore_count : 0,
        })
      } else if (value.event === 'delta') {
        const chunk = typeof parsed.text === 'string' ? parsed.text : ''
        if (chunk) {
          accumulated += chunk
          callbacks.onDelta?.(chunk)
        }
      } else if (value.event === 'done') {
        finalText = typeof parsed.text === 'string' ? parsed.text : accumulated
        callbacks.onDone?.(finalText)
      } else if (value.event === 'error') {
        const message = typeof parsed.error === 'string' && parsed.error ? parsed.error : 'Inline edit failed'
        const code = typeof parsed.code === 'string' ? parsed.code : undefined
        throw new APIError(message, { status: 500, code, payload: parsed })
      }
    }
  } finally {
    reader.releaseLock()
  }

  return finalText ?? accumulated
}
