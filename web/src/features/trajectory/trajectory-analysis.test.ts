import { describe, expect, it } from 'vitest'
import type { AgentRunTrace, AgentRunTraceRecord } from '@/lib/api'
import {
  analyzeTrajectory,
  projectTimeline,
  timelineRangeSpanIDs,
  visibleTreeSpanIDs,
} from './trajectory-analysis'

const startedAt = Date.parse('2026-08-13T10:00:00.000Z')

describe('trajectory analysis', () => {
  it('builds the full parent tree and derives latency, token, and idle metrics', () => {
    const analysis = analyzeTrajectory(traceFixture())

    expect(analysis.roots).toHaveLength(1)
    expect(analysis.roots[0].id).toBe('run')
    expect(analysis.roots[0].children.map((span) => span.id)).toEqual(['model-1', 'model-2'])
    expect(analysis.roots[0].children[0].children[0].id).toBe('tool-1')
    expect(analysis.roots[0].children[0].children[0].children[0].id).toBe('context-1')

    const firstModel = analysis.spans.find((span) => span.id === 'model-1')
    const secondModel = analysis.spans.find((span) => span.id === 'model-2')
    expect(firstModel).toMatchObject({ ttftMs: 500, generationMs: 2_500, gapBeforeMs: 0 })
    expect(secondModel).toMatchObject({ ttftMs: 1_000, generationMs: 3_000, gapBeforeMs: 2_000 })
    expect(analysis.metrics).toMatchObject({
      totalMs: 10_000,
      busyMs: 7_000,
      idleMs: 3_000,
      modelMs: 7_000,
      toolMs: 3_000,
      modelCalls: 2,
      toolCalls: 2,
      averageTTFTMs: 750,
      promptTokens: 1_500,
      cachedTokens: 850,
      completionTokens: 150,
    })
    expect(analysis.metrics.cacheHitRate).toBeCloseTo(850 / 1_500)
    expect(analysis.events.map((event) => event.label)).toEqual(['Agent cycle 2'])
  })

  it('projects wall-clock, idle-compressed, and equal-width timelines', () => {
    const analysis = analyzeTrajectory(traceFixture())
    const actual = projectTimeline(analysis, 'actual')
    const duration = projectTimeline(analysis, 'duration')
    const sequence = projectTimeline(analysis, 'sequence')

    const actualSecondModel = actual.spans.find((span) => span.id === 'model-2')
    const durationSecondModel = duration.spans.find((span) => span.id === 'model-2')
    expect(actualSecondModel?.start).toBe(startedAt + 5_000)
    expect(durationSecondModel?.start).toBe(startedAt + 3_000)
    expect(sequence).toMatchObject({ start: 0, end: 5 })
    expect(sequence.spans.map((span) => span.end - span.start)).toEqual([1, 1, 1, 1, 1])
    expect(actual.spans.find((span) => span.id === 'model-1')?.ttftEnd).toBe(startedAt + 500)
  })

  it('keeps ancestors visible when a selected interval only overlaps a deep child', () => {
    const analysis = analyzeTrajectory(traceFixture())
    const projection = projectTimeline(analysis, 'actual')
    const selected = timelineRangeSpanIDs(projection, startedAt + 1_400, startedAt + 1_600)
    const visible = visibleTreeSpanIDs(analysis.roots, new Set(['context-1']))
    const allVisible = visibleTreeSpanIDs(analysis.roots, new Set(analysis.spans.map((span) => span.id)))

    expect([...selected].sort()).toEqual(['context-1', 'model-1', 'tool-1'])
    expect([...visible].sort()).toEqual(['context-1', 'model-1', 'run', 'tool-1'])
    expect([...allVisible].sort()).toEqual(analysis.spans.map((span) => span.id).sort())
  })

  it('treats malformed optional timing metadata as missing', () => {
    const trace = traceFixture()
    const firstModelRecord = trace.records.find((record) => record.data?.span_id === 'model-1')
    if (!firstModelRecord?.data) throw new Error('fixture is missing model-1')
    firstModelRecord.data.attrs = { ...firstModelRecord.data.attrs as object, ttft_ms: 'not-a-number' }

    expect(analyzeTrajectory(trace).spans.find((span) => span.id === 'model-1')?.ttftMs).toBeNull()
  })
})

function traceFixture(): AgentRunTrace {
  return {
    summary: {
      id: 'run-test',
      created_at: iso(0),
      path: '.denova/runs/run-test.jsonl',
      status: 'success',
      events: 7,
      context_parts: 1,
      duration_ms: 10_000,
    },
    records: [
      span('agent_run', 'run', '', 0, 10_000, { agent_kind: 'writing' }),
      span('llm_call', 'model-1', 'run', 0, 3_000, {
        model: 'model-a', ttft_ms: 500, prompt_tokens: 1_000, cached_prompt_tokens: 600, completion_tokens: 100,
      }),
      span('tool_call', 'tool-1', 'model-1', 1_000, 1_000, { tool_name: 'read_file' }),
      span('context_build', 'context-1', 'tool-1', 1_400, 200, { name: 'Tool result context' }),
      span('llm_call', 'model-2', 'run', 5_000, 4_000, {
        model: 'model-a', ttft_ms: 1_000, prompt_tokens: 500, cached_prompt_tokens: 250, completion_tokens: 50,
      }),
      span('tool_call', 'tool-2', 'model-2', 6_000, 2_000, { tool_name: 'write_file' }),
      {
        type: 'agent_cycle',
        run_id: 'run-test',
        created_at: iso(4_000),
        data: { count: 2 },
      },
    ],
  }
}

function span(
  type: string,
  spanID: string,
  parentSpanID: string,
  offsetMs: number,
  durationMs: number,
  attrs: Record<string, unknown>,
): AgentRunTraceRecord {
  return {
    type,
    run_id: 'run-test',
    created_at: iso(offsetMs + durationMs),
    data: {
      span_id: spanID,
      parent_span_id: parentSpanID,
      started_at: iso(offsetMs),
      ended_at: iso(offsetMs + durationMs),
      duration_ms: durationMs,
      status: 'success',
      attrs,
    },
  }
}

function iso(offsetMs: number) {
  return new Date(startedAt + offsetMs).toISOString()
}
