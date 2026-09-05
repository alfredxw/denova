import { describe, expect, it } from 'vitest'
import { formatToolJSON, inspectToolMessage } from './tool-inspection'

describe('tool inspection data', () => {
  it('formats without rounding integers, rewriting escapes or removing duplicate keys', () => {
    const raw = '{"id":9223372036854775807,"id":1,"text":"\\u4e2d","n":1e+09}'
    expect(formatToolJSON(raw)).toEqual({
      language: 'json',
      text: '{\n  "id": 9223372036854775807,\n  "id": 1,\n  "text": "\\u4e2d",\n  "n": 1e+09\n}',
    })
  })

  it.each(['', '{"command":"still streaming', '{"a":1,}', 'stdout\nstderr', '{"schema":"process.result.v1"}\ncommand output'])('preserves non-JSON or incomplete input: %s', (source) => {
    expect(formatToolJSON(source)).toEqual({ text: source, language: 'plaintext' })
  })

  it('retains the recorded output and identifies truncated display content', () => {
    const output = JSON.stringify({
      schema: 'process.result.v1', output_truncated: true,
      artifact: { purpose: 'complete_tool_output', complete: true, readable_path: 'outputs/command.txt' },
      artifacts: [{ purpose: 'attachment', complete: true, readable_path: 'attachment.txt' }],
    }) + '\nfirst page'
    expect(inspectToolMessage({ id: 'call-1', role: 'tool_call', name: 'bash', args: ' {"command":"pwd"} ', result: output })).toMatchObject({
      input: ' {"command":"pwd"} ', output, callId: 'call-1', truncated: true,
    })
  })

  it('does not invent tool input or output from an interaction record', () => {
    const ask = { schema: 'ask.pending.v1', id: 'question-1', tool_call_id: 'call-2', agent_kind: 'ide', status: 'pending' as const, questions: [{ id: 'q1', question: 'Which chapter?' }] }
    expect(inspectToolMessage({ role: 'ask', ask })).toMatchObject({ callId: 'call-2', input: undefined, output: undefined, interaction: ask })
  })
})
