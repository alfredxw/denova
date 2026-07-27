export type ToolResultSeverity = 'success' | 'warning' | 'error'

export interface ToolResultContinuation {
  kind: 'offset' | 'cursor'
  value: string
}

export interface ToolResultEnvelope {
  schema: string
  status: string
  severity: ToolResultSeverity
  truncated: boolean
  exitCode?: number
  continuation?: ToolResultContinuation
  recovery?: string
  retryStrategy?: string
}

const maxEnvelopeCharacters = 64 * 1024
const supportedSchemas = new Set([
  'process.result.v1',
  'resource.read.v1',
  'workspace.search.v1',
  'web_fetch.v1',
  'web_search.v1',
  'browser.result.v1',
])

/**
 * Decodes the bounded metadata envelope that canonical tools place either in
 * the complete JSON result or on its first line. Payload text after that line
 * is deliberately left opaque and continues to use the generic detail view.
 */
export function decodeToolResultEnvelope(result: string): ToolResultEnvelope | null {
  const metadata = parseMetadata(result)
  if (!metadata) return null
  const schema = stringValue(metadata.schema)
  if (!supportedSchemas.has(schema)) return null

  const status = stringValue(metadata.status) || 'success'
  const limits = recordValue(metadata.limits)
  const truncated = metadata.output_truncated === true || limits.truncated === true
  const continuation = continuationFrom(limits)
  const recoveryData = recordValue(metadata.recovery)
  const recovery = stringValue(recoveryData.suggestion) || stringValue(metadata.suggested_action) || undefined
  const retryStrategy = stringValue(metadata.retry_strategy) || undefined

  return {
    schema,
    status,
    severity: resultSeverity(schema, status, truncated, continuation !== undefined),
    truncated,
    exitCode: finiteNumber(metadata.exit_code),
    continuation,
    recovery,
    retryStrategy,
  }
}

function parseMetadata(result: string): Record<string, unknown> | null {
  const body = result.trimStart()
  if (!body) return null

  if (body.length <= maxEnvelopeCharacters) {
    const complete = parseRecord(body)
    if (complete) return complete
  }

  const newline = body.indexOf('\n')
  const firstLine = newline >= 0 ? body.slice(0, newline) : body
  if (!firstLine || firstLine.length > maxEnvelopeCharacters) return null
  return parseRecord(firstLine)
}

function parseRecord(value: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed as Record<string, unknown>
      : null
  } catch {
    return null
  }
}

function continuationFrom(limits: Record<string, unknown>): ToolResultContinuation | undefined {
  const nextOffset = finiteNumber(limits.next_offset)
  if (nextOffset !== undefined) return { kind: 'offset', value: String(nextOffset) }
  const nextCursor = stringValue(limits.next_cursor)
  if (nextCursor) return { kind: 'cursor', value: nextCursor }
  return undefined
}

function resultSeverity(schema: string, status: string, truncated: boolean, hasContinuation: boolean): ToolResultSeverity {
  if (schema === 'web_fetch.v1' || schema === 'web_search.v1') {
    return status === 'success' && !truncated ? 'success' : 'warning'
  }
  if (schema === 'process.result.v1') {
    // A non-zero exit is a completed process outcome, and is often the useful
    // result of a diagnostic command. Process-start and protocol failures do
    // not use this envelope and still surface as real tool errors.
    if (status === 'failed') return 'warning'
    if (status === 'timed_out' || status === 'cancelled') return 'error'
    return truncated ? 'warning' : 'success'
  }
  if (schema === 'browser.result.v1') {
    return status === 'completed' || status === 'success' ? 'success' : 'error'
  }
  if (status === 'failed' || status === 'error' || status === 'cancelled' || status === 'timed_out') return 'error'
  // read/search use "partial" for a bounded page even when they returned the
  // requested range successfully. A continuation is useful metadata, not a
  // warning; partial results without a continuation still need attention.
  if (status === 'partial' && hasContinuation && (schema === 'resource.read.v1' || schema === 'workspace.search.v1')) return 'success'
  return status === 'partial' || truncated ? 'warning' : 'success'
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function finiteNumber(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}
