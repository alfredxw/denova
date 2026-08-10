import { describe, expect, it } from 'vitest'
import { decodeInteractiveStreamEvent } from './story-stage-stream-decoder'

describe('interactive stream decoder', () => {
  it('preserves validated payload metadata for handled events', () => {
    expect(decodeInteractiveStreamEvent({
      event: 'chunk',
      data: JSON.stringify({
        content: '继续前进',
        run_id: 'run-1',
        agent_kind: 'interactive_story',
        custom_trace_field: 'kept',
      }),
    })).toEqual({
      kind: 'handled',
      event: {
        type: 'chunk',
        data: {
          content: '继续前进',
          run_id: 'run-1',
          agent_kind: 'interactive_story',
          custom_trace_field: 'kept',
        },
      },
    })
  })

  it('isolates invalid JSON and schema violations', () => {
    expect(decodeInteractiveStreamEvent({ event: 'thinking', data: '{broken' })).toMatchObject({
      kind: 'invalid',
      type: 'thinking',
    })
    expect(decodeInteractiveStreamEvent({
      event: 'agent_cycle_started',
      data: JSON.stringify({ command_id: 'command-1' }),
    })).toMatchObject({
      kind: 'invalid',
      type: 'agent_cycle_started',
    })
  })

  it('does not parse intentionally ignored or unknown event payloads', () => {
    expect(decodeInteractiveStreamEvent({ event: 'workspace_change', data: '{ignored' })).toEqual({
      kind: 'ignored',
      type: 'workspace_change',
    })
    expect(decodeInteractiveStreamEvent({ event: 'future_event', data: '{unknown' })).toEqual({
      kind: 'unknown',
      type: 'future_event',
    })
  })
})
