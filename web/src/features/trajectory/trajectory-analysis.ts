import type { AgentRunTrace, AgentRunTraceRecord } from '@/lib/api'

export type TrajectoryCategory = 'run' | 'model' | 'tool' | 'context' | 'verification' | 'event'
export type TrajectoryTimelineMode = 'sequence' | 'duration' | 'actual'

export interface TrajectorySpan {
  id: string
  parentId: string
  recordIndex: number
  record: AgentRunTraceRecord
  type: string
  category: TrajectoryCategory
  label: string
  status: string
  startedAt: number
  endedAt: number
  durationMs: number
  gapBeforeMs: number
  ttftMs: number | null
  generationMs: number | null
  inputTokens: number
  cachedTokens: number
  outputTokens: number
  reasoningTokens: number
  attrs: Record<string, unknown>
  children: TrajectorySpan[]
}

export interface TrajectoryEventRecord {
  id: string
  recordIndex: number
  record: AgentRunTraceRecord
  category: TrajectoryCategory
  label: string
  status: string
  timestamp: number
  data: Record<string, unknown>
}

export interface TrajectoryMetrics {
  totalMs: number
  busyMs: number
  idleMs: number
  modelMs: number
  toolMs: number
  modelCalls: number
  toolCalls: number
  errors: number
  averageTTFTMs: number | null
  p50TTFTMs: number | null
  p95TTFTMs: number | null
  averageThroughput: number | null
  promptTokens: number
  cachedTokens: number
  completionTokens: number
  cacheHitRate: number
}

export interface TrajectoryAnalysis {
  roots: TrajectorySpan[]
  spans: TrajectorySpan[]
  events: TrajectoryEventRecord[]
  startMs: number
  endMs: number
  metrics: TrajectoryMetrics
}

export interface TimelineSpan {
  id: string
  category: TrajectoryCategory
  label: string
  status: string
  start: number
  end: number
  durationMs: number
  ttftMs: number | null
  ttftEnd: number | null
}

export interface TimelineProjection {
  start: number
  end: number
  unit: 'time' | 'step'
  spans: TimelineSpan[]
}

const SPAN_CATEGORIES: Record<string, TrajectoryCategory> = {
  agent_run: 'run',
  llm_call: 'model',
  tool_call: 'tool',
  context_build: 'context',
  context_compaction: 'context',
  post_run_verification: 'verification',
}

/** Builds the semantic span tree, chronological event stream, and latency aggregates for one run. */
export function analyzeTrajectory(trace: AgentRunTrace): TrajectoryAnalysis {
  const spanRecords: TrajectorySpan[] = trace.records.flatMap((record, recordIndex): TrajectorySpan[] => {
    if (record.type === 'llm_input' || record.type === 'llm_output') return []
    const data = objectValue(record.data)
    const spanID = stringValue(data.span_id)
    if (!spanID) return []
    const startedAt = timestampValue(data.started_at) ?? timestampValue(record.created_at)
    if (startedAt === null) return []
    const durationMs = Math.max(0, numberValue(data.duration_ms))
    const endedAt = timestampValue(data.ended_at) ?? startedAt + durationMs
    const attrs = objectValue(data.attrs)
    const category = spanCategory(record.type)
    return [{
      id: spanID,
      parentId: stringValue(data.parent_span_id),
      recordIndex,
      record,
      type: record.type,
      category,
      label: spanLabel(record.type, attrs),
      status: stringValue(data.status) || 'success',
      startedAt,
      endedAt: Math.max(startedAt, endedAt),
      durationMs,
      gapBeforeMs: 0,
      ttftMs: optionalNumber(attrs.ttft_ms),
      generationMs: null,
      inputTokens: numberValue(attrs.prompt_tokens),
      cachedTokens: numberValue(attrs.cached_prompt_tokens),
      outputTokens: numberValue(attrs.completion_tokens),
      reasoningTokens: numberValue(attrs.reasoning_tokens),
      attrs,
      children: [],
    }]
  })

  const orderedSpans = [...spanRecords].sort(compareSpanTime)
  const orderedOperations = orderedSpans.filter((span) => span.category !== 'run')
  let coveredUntil = orderedOperations[0]?.startedAt ?? 0
  for (const span of orderedOperations) {
    span.gapBeforeMs = Math.max(0, span.startedAt - coveredUntil)
    coveredUntil = Math.max(coveredUntil, span.endedAt)
  }
  for (const span of orderedSpans) {
    span.generationMs = span.ttftMs === null ? null : Math.max(0, span.durationMs - span.ttftMs)
  }

  const byID = new Map(orderedSpans.map((span) => [span.id, span]))
  const roots: TrajectorySpan[] = []
  for (const span of orderedSpans) {
    const parent = byID.get(span.parentId)
    if (parent && parent !== span) parent.children.push(span)
    else roots.push(span)
  }
  sortSpanTree(roots)

  const events = trace.records.flatMap((record, recordIndex) => {
    const data = objectValue(record.data)
    if (stringValue(data.span_id)) return []
    return [{
      id: `${record.type}:${record.created_at}:${recordIndex}`,
      recordIndex,
      record,
      category: eventCategory(record),
      label: eventLabel(record),
      status: eventStatus(record),
      timestamp: timestampValue(record.created_at) ?? 0,
      data,
    } satisfies TrajectoryEventRecord]
  })

  const timestamps = [
    ...orderedSpans.flatMap((span) => [span.startedAt, span.endedAt]),
    ...events.map((event) => event.timestamp).filter((value) => value > 0),
  ]
  const fallbackStart = timestampValue(trace.summary.created_at) ?? Date.now()
  const startMs = timestamps.length > 0 ? Math.min(...timestamps) : fallbackStart
  const endFromSummary = trace.summary.duration_ms && trace.summary.duration_ms > 0
    ? fallbackStart + trace.summary.duration_ms
    : fallbackStart
  const endMs = Math.max(startMs, endFromSummary, ...timestamps)

  return {
    roots,
    spans: orderedSpans,
    events,
    startMs,
    endMs,
    metrics: deriveMetrics(orderedSpans, startMs, endMs),
  }
}

/** Projects operation spans into sequence, idle-compressed duration, or wall-clock time. */
export function projectTimeline(
  analysis: TrajectoryAnalysis,
  mode: TrajectoryTimelineMode,
): TimelineProjection {
  const operations = analysis.spans.filter((span) => span.category !== 'run')
  if (mode === 'sequence') {
    return {
      start: 0,
      end: Math.max(1, operations.length),
      unit: 'step',
      spans: operations.map((span, index) => ({
        id: span.id,
        category: span.category,
        label: span.label,
        status: span.status,
        start: index,
        end: index + 1,
        durationMs: span.durationMs,
        ttftMs: span.ttftMs,
        ttftEnd: span.ttftMs === null ? null : index + Math.min(1, span.ttftMs / Math.max(1, span.durationMs)),
      })),
    }
  }

  let removedIdle = 0
  let coveredUntil: number | null = null
  const removedBefore = new Map<string, number>()
  for (const span of operations) {
    if (mode === 'duration' && coveredUntil !== null && span.startedAt > coveredUntil) {
      removedIdle += span.startedAt - coveredUntil
    }
    removedBefore.set(span.id, removedIdle)
    coveredUntil = coveredUntil === null ? span.endedAt : Math.max(coveredUntil, span.endedAt)
  }
  const spans = operations.map((span): TimelineSpan => {
    const offset = removedBefore.get(span.id) ?? 0
    const start = span.startedAt - offset
    const end = Math.max(start + 1, span.endedAt - offset)
    return {
      id: span.id,
      category: span.category,
      label: span.label,
      status: span.status,
      start,
      end,
      durationMs: span.durationMs,
      ttftMs: span.ttftMs,
      ttftEnd: span.ttftMs === null ? null : Math.min(end, start + span.ttftMs),
    }
  })
  if (spans.length === 0) return { start: 0, end: 1, unit: 'time', spans: [] }
  return {
    start: Math.min(...spans.map((span) => span.start)),
    end: Math.max(...spans.map((span) => span.end)),
    unit: 'time',
    spans,
  }
}

/** Returns IDs of spans that overlap an inclusive timeline interval. */
export function timelineRangeSpanIDs(projection: TimelineProjection, start: number, end: number) {
  const low = Math.min(start, end)
  const high = Math.max(start, end)
  return new Set(projection.spans.filter((span) => span.start <= high && span.end >= low).map((span) => span.id))
}

/** Keeps matching descendants and their ancestors so filtered trees remain navigable. */
export function visibleTreeSpanIDs(
  roots: readonly TrajectorySpan[],
  matches: ReadonlySet<string>,
): ReadonlySet<string> {
  const visible = new Set<string>()
  const visit = (span: TrajectorySpan): boolean => {
    // Visit every child even after one branch matches; Array.some would short-circuit
    // and silently drop later sibling call chains from the visible ledger.
    let childMatches = false
    for (const child of span.children) {
      if (visit(child)) childMatches = true
    }
    if (matches.has(span.id) || childMatches) {
      visible.add(span.id)
      return true
    }
    return false
  }
  roots.forEach(visit)
  return visible
}

export function formatTrajectoryDuration(milliseconds: number | null | undefined): string {
  if (milliseconds === null || milliseconds === undefined || !Number.isFinite(milliseconds)) return '—'
  if (milliseconds < 1_000) return `${Math.round(milliseconds)} ms`
  if (milliseconds < 60_000) return `${(milliseconds / 1_000).toFixed(milliseconds < 10_000 ? 2 : 1)} s`
  const minutes = Math.floor(milliseconds / 60_000)
  const seconds = Math.round((milliseconds % 60_000) / 1_000)
  return `${minutes}m ${seconds}s`
}

function deriveMetrics(spans: readonly TrajectorySpan[], startMs: number, endMs: number): TrajectoryMetrics {
  const operations = spans.filter((span) => span.category !== 'run' && span.durationMs > 0)
  const totalMs = Math.max(0, endMs - startMs)
  const busyMs = intervalUnionDuration(operations.map((span) => [span.startedAt, span.endedAt]))
  const models = spans.filter((span) => span.category === 'model')
  const tools = spans.filter((span) => span.category === 'tool')
  const ttfts = models.flatMap((span) => span.ttftMs === null ? [] : [span.ttftMs])
  const promptTokens = models.reduce((total, span) => total + span.inputTokens, 0)
  const cachedTokens = models.reduce((total, span) => total + span.cachedTokens, 0)
  const generatedTokens = models.reduce((total, span) => total + span.outputTokens, 0)
  const generationMs = models.reduce((total, span) => total + Math.max(0, span.generationMs ?? 0), 0)
  return {
    totalMs,
    busyMs,
    idleMs: Math.max(0, totalMs - busyMs),
    modelMs: models.reduce((total, span) => total + span.durationMs, 0),
    toolMs: tools.reduce((total, span) => total + span.durationMs, 0),
    modelCalls: models.length,
    toolCalls: tools.length,
    errors: spans.filter((span) => isErrorStatus(span.status)).length,
    averageTTFTMs: ttfts.length === 0 ? null : ttfts.reduce((total, value) => total + value, 0) / ttfts.length,
    p50TTFTMs: percentile(ttfts, 0.5),
    p95TTFTMs: percentile(ttfts, 0.95),
    averageThroughput: generatedTokens > 0 && generationMs > 0 ? generatedTokens / (generationMs / 1_000) : null,
    promptTokens,
    cachedTokens,
    completionTokens: models.reduce((total, span) => total + span.outputTokens, 0),
    cacheHitRate: promptTokens === 0 ? 0 : cachedTokens / promptTokens,
  }
}

function percentile(values: readonly number[], ratio: number): number | null {
  if (values.length === 0) return null
  const sorted = [...values].sort((left, right) => left - right)
  const index = Math.min(sorted.length - 1, Math.max(0, (sorted.length - 1) * ratio))
  const lower = Math.floor(index)
  const upper = Math.ceil(index)
  if (lower === upper) return sorted[lower]
  return sorted[lower] + (sorted[upper] - sorted[lower]) * (index - lower)
}

function intervalUnionDuration(intervals: ReadonlyArray<readonly [number, number]>): number {
  if (intervals.length === 0) return 0
  const ordered = [...intervals].sort((left, right) => left[0] - right[0])
  let [start, end] = ordered[0]
  let total = 0
  for (const [nextStart, nextEnd] of ordered.slice(1)) {
    if (nextStart <= end) {
      end = Math.max(end, nextEnd)
      continue
    }
    total += Math.max(0, end - start)
    start = nextStart
    end = nextEnd
  }
  return total + Math.max(0, end - start)
}

function sortSpanTree(spans: TrajectorySpan[]) {
  spans.sort(compareSpanTime)
  for (const span of spans) sortSpanTree(span.children)
}

function compareSpanTime(left: TrajectorySpan, right: TrajectorySpan) {
  return left.startedAt - right.startedAt || left.endedAt - right.endedAt || left.recordIndex - right.recordIndex
}

function spanCategory(type: string): TrajectoryCategory {
  return SPAN_CATEGORIES[type] ?? (type.includes('tool') ? 'tool' : type.includes('llm') || type.includes('model') ? 'model' : 'event')
}

function eventCategory(record: AgentRunTraceRecord): TrajectoryCategory {
  if (record.type.includes('tool')) return 'tool'
  if (record.type.includes('llm') || record.type === 'token_usage') return 'model'
  if (record.type.includes('context')) return 'context'
  if (record.type.includes('verification') || record.type === 'mutations') return 'verification'
  if (record.type.includes('run') || record.type === 'agent_cycle') return 'run'
  return 'event'
}

function spanLabel(type: string, attrs: Record<string, unknown>): string {
  if (type === 'agent_run') return stringValue(attrs.agent_kind) || 'Agent run'
  if (type === 'llm_call') return stringValue(attrs.model) || 'Model call'
  if (type === 'tool_call') return stringValue(attrs.tool_name) || 'Tool call'
  if (type === 'context_compaction') return 'Context compaction'
  if (type === 'context_build') return 'Context build'
  return stringValue(attrs.name) || type
}

function eventLabel(record: AgentRunTraceRecord): string {
  const data = objectValue(record.data)
  if (record.type === 'agent_cycle') return `Agent cycle ${numberValue(data.count) || ''}`.trim()
  if (record.type === 'tool_decision') {
    const decision = objectValue(data.decision)
    return stringValue(decision.tool_name) || 'Tool decision'
  }
  if (record.type === 'tool_execution') {
    const result = objectValue(data.result)
    return stringValue(result.tool_name) || 'Tool execution'
  }
  if (record.type === 'event') return stringValue(data.event_type) || 'Event'
  return record.type.replaceAll('_', ' ')
}

function eventStatus(record: AgentRunTraceRecord): string {
  const data = objectValue(record.data)
  return stringValue(data.status)
    || stringValue(objectValue(data.result).status)
    || stringValue(objectValue(data.decision).action)
    || 'recorded'
}

function isErrorStatus(status: string) {
  const normalized = status.toLowerCase()
  return normalized === 'error' || normalized === 'failed' || normalized === 'blocked' || normalized === 'aborted'
}

export function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

export function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

export function numberValue(value: unknown): number {
  const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : 0
  return Number.isFinite(parsed) ? parsed : 0
}

function optionalNumber(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : Number.NaN
  return Number.isFinite(parsed) ? Math.max(0, parsed) : null
}

function timestampValue(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value !== 'string' || !value.trim()) return null
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : null
}
