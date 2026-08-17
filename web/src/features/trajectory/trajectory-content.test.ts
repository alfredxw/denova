import { describe, expect, it } from 'vitest'
import type { AgentRunTrace, AgentRunTraceRecord } from '@/lib/api'
import { analyzeTrajectory } from './trajectory-analysis'
import { analyzeTrajectoryContent } from './trajectory-content'

describe('trajectory content analysis', () => {
  it('reconstructs system, tools, context, assistant blocks, and tool results without duplicating history', () => {
    const trace = fixture()
    const content = analyzeTrajectoryContent(trace, analyzeTrajectory(trace))

    expect(content.available).toBe(true)
    expect(content.requests).toHaveLength(2)
    expect(content.entries.map((entry) => entry.kind)).toEqual(['system', 'user', 'assistant', 'tool', 'assistant'])
    expect(content.entries[0]).toMatchObject({
      label: 'Initial System Prompt',
      content: 'You are a careful assistant.',
      tools: [{ name: 'read', description: 'Read a file' }],
    })
    expect(content.entries[2]).toMatchObject({ reasoning: 'I should read it.', content: '', toolCalls: [{ id: 'call-read', name: 'read' }] })
    expect(content.entries[3]).toMatchObject({ toolName: 'read', toolCallID: 'call-read', content: '# Chapter', toolCall: { arguments: '{"path":"chapter.md"}' } })
    expect(content.entries[4]).toMatchObject({ content: '## Summary\n\nDone.', reasoning: 'The file is clear.' })
    expect(content.requests[0].outputNodes.map((node) => node.type)).toEqual(['message', 'tool-group'])
    expect(content.toolCalls[0]).toMatchObject({
      call: { id: 'call-read', name: 'read', arguments: '{"path":"chapter.md"}' },
      result: { content: '# Chapter' },
      definition: { name: 'read' },
    })
  })

  it('pairs a terminal tool output when no later model input contains the result', () => {
    const trace = fixture()
    trace.records = trace.records.slice(0, 3)
    trace.records.push(toolOutput('tool-1', 'call-read', 900, 'Terminal result'))
    const content = analyzeTrajectoryContent(trace, analyzeTrajectory(trace))

    expect(content.toolCalls).toHaveLength(1)
    expect(content.toolCalls[0]).toMatchObject({
      call: { id: 'call-read', name: 'read' },
      result: null,
      output: { callID: 'call-read', content: 'Terminal result', status: 'success', truncated: false },
    })
  })

  it('classifies model-only user messages as context and exposes a system update', () => {
    const trace = fixture()
    const firstInput = trace.records.find((record) => record.type === 'llm_input')
    const firstContent = firstInput?.data?.content as Record<string, unknown>
    firstContent.messages = [
      { role: 'system', content: 'Updated prompt.' },
      { role: 'user', content: 'Runtime context', extra: { 'agent.context.placement': 'before_user' } },
    ]
    const content = analyzeTrajectoryContent(trace, analyzeTrajectory(trace))

    expect(content.entries[0]).toMatchObject({ kind: 'system', content: 'Updated prompt.' })
    expect(content.entries[1]).toMatchObject({ kind: 'context', content: 'Runtime context' })
  })

  it('reports metadata-only traces as unavailable', () => {
    const trace: AgentRunTrace = { summary: fixture().summary, records: [span('llm_call', 'model-1', 0, 500)] }
    const content = analyzeTrajectoryContent(trace, analyzeTrajectory(trace))
    expect(content).toMatchObject({ available: false, requests: [], entries: [] })
  })
})

function fixture(): AgentRunTrace {
  const messages = [
    { role: 'system', content: 'You are a careful assistant.' },
    { role: 'user', content: 'Read the chapter.' },
  ]
  const firstOutput = {
    role: 'assistant', content: '', reasoning_content: 'I should read it.',
    tool_calls: [{ id: 'call-read', type: 'function', function: { name: 'read', arguments: '{"path":"chapter.md"}' } }],
  }
  const secondMessages = [...messages, firstOutput, { role: 'tool', content: '# Chapter', tool_call_id: 'call-read', tool_name: 'read' }]
  return {
    summary: {
      id: 'run-content', created_at: iso(0), path: '.denova/runs/run-content.jsonl', status: 'success',
      events: 0, context_parts: 0, duration_ms: 2_000, content_captured: true,
    },
    records: [
      input('model-1', 'llm-1', 0, messages),
      output('model-1', 'llm-1', 800, firstOutput),
      span('llm_call', 'model-1', 0, 800),
      input('model-2', 'llm-2', 1_000, secondMessages),
      output('model-2', 'llm-2', 2_000, { role: 'assistant', content: '## Summary\n\nDone.', reasoning_content: 'The file is clear.' }),
      span('llm_call', 'model-2', 1_000, 1_000),
    ],
  }
}

function input(spanID: string, callID: string, offset: number, messages: unknown[]): AgentRunTraceRecord {
  return {
    type: 'llm_input', run_id: 'run-content', created_at: iso(offset),
    data: {
      span_id: spanID, call_id: callID,
      content: {
        source: 'agent model boundary', purpose: 'developer trajectory inspection', messages,
        tools: [{ name: 'read', description: 'Read a file', parameters: { type: 'object', properties: { path: { type: 'string' } } } }],
      },
    },
  }
}

function output(spanID: string, callID: string, offset: number, message: unknown): AgentRunTraceRecord {
  return {
    type: 'llm_output', run_id: 'run-content', created_at: iso(offset),
    data: { span_id: spanID, call_id: callID, content: { status: 'success', message } },
  }
}

function span(type: string, spanID: string, offset: number, duration: number): AgentRunTraceRecord {
  return {
    type, run_id: 'run-content', created_at: iso(offset + duration),
    data: {
      span_id: spanID, status: 'success', started_at: iso(offset), ended_at: iso(offset + duration), duration_ms: duration,
      attrs: { model: 'model-a', ttft_ms: 200, prompt_tokens: 40, completion_tokens: 10, reasoning_tokens: 4 },
    },
  }
}

function toolOutput(spanID: string, callID: string, offset: number, result: string): AgentRunTraceRecord {
  return {
    type: 'tool_output', run_id: 'run-content', created_at: iso(offset),
    data: {
      span_id: spanID, call_id: callID,
      content: {
        source: 'agent tool boundary', purpose: 'developer trajectory inspection', tool_name: 'read',
        provider_call_id: callID, execution_id: 'execution-read', status: 'success', result,
        original_bytes: result.length, returned_bytes: result.length, truncated: false,
      },
    },
  }
}

function iso(offset: number) {
  return new Date(Date.parse('2026-08-13T10:00:00.000Z') + offset).toISOString()
}
