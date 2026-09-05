import type { TFunction } from 'i18next'
import { describe, expect, it, vi } from 'vitest'
import { buildAgentMessageViews } from '@/lib/agent-message-view'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { createAgentReasoningMessage } from '@/lib/agent-ui-message'
import type { InteractiveSSEEvent } from '../../types'
import { createStoryStageStreamConsumer } from './story-stage-stream-consumer'
import { INTERACTIVE_STREAM_EVENT_CONTRACT } from './story-stage-stream-events'
import type { LiveMessageAccumulator } from './use-live-message-accumulator'

function eventStream(events: InteractiveSSEEvent[]) {
  return new ReadableStream<InteractiveSSEEvent>({
    start(controller) {
      for (const event of events) controller.enqueue(event)
      controller.close()
    },
  })
}

function consumerFixture(initialMessages: AgentUIMessage[] = []) {
  let messages = initialMessages
  const setMessages = vi.fn((updater: AgentUIMessage[] | ((current: AgentUIMessage[]) => AgentUIMessage[])) => {
    messages = typeof updater === 'function' ? updater(messages) : updater
  })
  const liveAccumulator = {
    appendAssistant: vi.fn(),
    appendContextCompaction: vi.fn(),
    appendThinking: vi.fn(),
    bindPersistedTurn: vi.fn(),
    collapseNonNarrative: vi.fn(),
    finishMessages: vi.fn(),
    flush: vi.fn(),
    prepareTurn: vi.fn(),
    resetAssistant: vi.fn(),
    resetCompaction: vi.fn(),
    resetForCheckpoint: vi.fn(() => setMessages([])),
  } as unknown as LiveMessageAccumulator
  const setActivity = vi.fn()
  const consumer = createStoryStageStreamConsumer({
    liveAccumulator,
    liveTurnNavigationAnchorId: 'live-turn',
    onRuntimeRecoveryRequired: vi.fn().mockResolvedValue(undefined),
    onTurnPersisted: vi.fn(),
    setActivity,
    setMessages,
    setStageRuntime: vi.fn(),
    t: ((key: string) => key) as unknown as TFunction,
    updateStageRun: vi.fn(),
  })
  return { consumer, liveAccumulator, messages: () => messages, setActivity }
}

describe('story stage stream event contract', () => {
  it('treats a user abort as a paused terminal state without an error message', async () => {
    const fixture = consumerFixture()

    const outcome = await fixture.consumer.consume(
      eventStream([{
        id: '1',
        event: 'aborted',
        data: JSON.stringify({ reason: 'user_requested' }),
      }]),
      fixture.consumer.initialOutcome(),
    )

    expect(buildAgentMessageViews(fixture.messages()).some((view) => view.kind === 'error')).toBe(false)
    expect(outcome).toMatchObject({
      finishedNormally: false,
      streamFailed: false,
      terminalStatus: 'aborted',
      terminalEventReceived: true,
    })
  })

  it('surfaces unknown events instead of silently discarding them', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const fixture = consumerFixture()

    await fixture.consumer.consume(
      eventStream([
        { id: '8', event: 'future_runtime_event', data: '{}' },
        { id: '9', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome(),
    )

    expect(warn).toHaveBeenCalledWith(
      '[interactive-stage] received unknown stream event',
      { event: 'future_runtime_event', id: '8' },
    )
    expect(fixture.setActivity).toHaveBeenCalledWith('storyStage.activity.unsupportedEvent')
    warn.mockRestore()
  })

  it('declares every intentionally display-neutral event', async () => {
    const ignored = Object.entries(INTERACTIVE_STREAM_EVENT_CONTRACT)
      .filter(([, handling]) => handling === 'ignored')
      .map(([event]) => event)
    expect(ignored).toEqual([
      'ask_pending',
      'ask_resolved',
      'context_cleanup',
      'context_normalizer',
      'post_run_verification',
      'run_state',
      'subagent_artifact',
      'subagent_transcript_synchronized',
      'tool_target',
      'verification',
      'workspace_change',
    ])

    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const fixture = consumerFixture()
    await fixture.consumer.consume(
      eventStream([
        ...ignored.map((event, index) => ({ id: String(index + 1), event, data: '{}' })),
        { id: '10', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome(),
    )

    expect(warn).not.toHaveBeenCalled()
    warn.mockRestore()
  })

  it('projects tool execution lifecycle events into activity', async () => {
    const fixture = consumerFixture()
    await fixture.consumer.consume(
      eventStream([
        { id: '1', event: 'tool_started', data: JSON.stringify({ name: 'read' }) },
        { id: '2', event: 'tool_progress', data: JSON.stringify({ name: 'read' }) },
        { id: '3', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome(),
    )

    expect(fixture.setActivity).toHaveBeenCalledWith('storyStage.activity.processingTool')
  })

  it('projects one durable terminal status for a settled SubAgent', async () => {
    const fixture = consumerFixture()
    const settled = {
      status: 'failed',
      reason: 'review failed',
      run_id: 'parent-run',
      subagent: true,
      subagent_session_id: 'child-session',
      agent_name: 'reviewer',
    }

    await fixture.consumer.consume(
      eventStream([
        { id: '1', event: 'subagent_settled', data: JSON.stringify(settled) },
        { id: '2', event: 'subagent_settled', data: JSON.stringify(settled) },
        { id: '3', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome(),
    )

    const statuses = buildAgentMessageViews(fixture.messages()).filter((view) => view.kind === 'subagent-status')
    expect(statuses).toHaveLength(1)
    expect(statuses[0]).toMatchObject({
      data: { status: 'failed', reason: 'review failed' },
      metadata: { run_id: 'parent-run', subagent_session_id: 'child-session', agent_name: 'reviewer' },
    })
  })

  it('rejects one malformed payload without aborting later stream events', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const fixture = consumerFixture()

    const outcome = await fixture.consumer.consume(
      eventStream([
        { id: '1', event: 'thinking', data: '{broken' },
        { id: '2', event: 'thinking', data: JSON.stringify({ content: '继续处理' }) },
        { id: '3', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome(),
    )

    expect(warn).toHaveBeenCalledWith(
      '[interactive-stage] rejected malformed stream event',
      expect.objectContaining({ event: 'thinking', id: '1' }),
    )
    expect(fixture.setActivity).toHaveBeenCalledWith('storyStage.activity.invalidEvent')
    expect(fixture.liveAccumulator.appendThinking).toHaveBeenCalledWith('继续处理', expect.any(Object))
    expect(outcome).toMatchObject({ finishedNormally: true, streamFailed: false, terminalEventReceived: true })
    warn.mockRestore()
  })

  it('shows the server error and request ID from a Game stream failure', async () => {
    const fixture = consumerFixture()

    const outcome = await fixture.consumer.consume(
      eventStream([{
        id: '1',
        event: 'error',
        data: JSON.stringify({
          error: 'Agent transcript source revision conflict',
          request_id: '019ffb1f-0171-7436-828c-1d8f45095fe4',
        }),
      }]),
      fixture.consumer.initialOutcome(),
    )

    const error = buildAgentMessageViews(fixture.messages()).find((view) => view.kind === 'error')
    expect(error?.content).toContain('Agent transcript source revision conflict')
    expect(error?.content).toContain('019ffb1f-0171-7436-828c-1d8f45095fe4')
    expect(outcome).toMatchObject({ streamFailed: true, terminalStatus: 'error', terminalEventReceived: true })
  })

  it('localizes a truncated model response in Game mode', async () => {
    const fixture = consumerFixture()

    await fixture.consumer.consume(
      eventStream([{
        id: '1',
        event: 'error',
        data: JSON.stringify({ code: 'agent_runtime.model_output_truncated' }),
      }]),
      fixture.consumer.initialOutcome(),
    )

    const error = buildAgentMessageViews(fixture.messages()).find((view) => view.kind === 'error')
    expect(error?.content).toBe('common.modelOutputTruncated')
  })

  it('shows a Goal evaluation failure without failing the completed primary turn', async () => {
    const fixture = consumerFixture()
    const outcome = await fixture.consumer.consume(
      eventStream([
        {
          id: '1',
          event: 'goal_evaluation_failed',
          data: JSON.stringify({ code: 'agent_runtime.goal_evaluation_failed', detail: 'invalid JSON' }),
        },
        { id: '2', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome(),
    )

    const warning = buildAgentMessageViews(fixture.messages()).find((view) => view.kind === 'error')
    expect(warning?.content).toBe('storyStage.activity.goalEvaluationFailed')
    expect(outcome).toMatchObject({ streamFailed: false, finishedNormally: true, terminalEventReceived: true })
  })
})

describe('story stage display checkpoint recovery', () => {
  it('keeps root narrative Run identity while preserving trace segment metadata', async () => {
    const fixture = consumerFixture()
    await fixture.consumer.consume(
      eventStream([
        {
          id: '0',
          event: 'thinking',
          data: JSON.stringify({
            content: '正在判断门后的动静。',
            run_id: 'run-game',
            display_segment_id: 'thinking-segment',
          }),
        },
        {
          id: '1',
          event: 'chunk',
          data: JSON.stringify({
            content: '门后亮起一盏灯。',
            run_id: 'run-game',
            agent_kind: 'interactive_story',
            display_segment_id: 'narrative-segment',
            display_phase: 'candidate',
          }),
        },
        { id: '2', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome(),
    )

    expect(fixture.liveAccumulator.appendThinking).toHaveBeenCalledWith(
      '正在判断门后的动静。',
      expect.objectContaining({
        run_id: 'run-game',
        display_segment_id: 'thinking-segment',
      }),
    )
    expect(fixture.liveAccumulator.appendAssistant).toHaveBeenCalledWith(
      '门后亮起一盏灯。',
      'live-turn',
      expect.objectContaining({
        run_id: 'run-game',
        agent_kind: 'interactive_story',
        display_segment_id: undefined,
        display_phase: 'candidate',
      }),
    )
  })

  it('rebuilds a complete checkpoint before advancing to its committed cursor', async () => {
    const fixture = consumerFixture([createAgentReasoningMessage({ text: 'stale' })])
    const outcome = await fixture.consumer.consume(
      eventStream([
        {
          id: '0',
          event: 'task_checkpoint',
          data: JSON.stringify({ version: 1, cursor: 9, complete: true }),
        },
        {
          id: '0',
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'command-1',
            delivery: 'start_turn',
            message: '推开石门',
            operation_id: 'operation-1',
            cycle: 1,
          }),
        },
        {
          id: '0',
          event: 'thinking',
          data: JSON.stringify({ content: '完整思考' }),
        },
        {
          id: '9',
          event: 'task_checkpoint_committed',
          data: JSON.stringify({ version: 1, cursor: 9 }),
        },
        {
          id: '10',
          event: 'interactive_turn_persisted',
          data: JSON.stringify({ story_id: 'story-1', branch_id: 'main', turn_count: 1, turn: { id: 'turn-1' } }),
        },
        { id: '11', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome('17'),
    )

    expect(fixture.liveAccumulator.resetForCheckpoint).toHaveBeenCalledOnce()
    expect(fixture.liveAccumulator.prepareTurn).toHaveBeenCalledWith('推开石门', 'live-turn', 'replace')
    expect(fixture.liveAccumulator.appendThinking).toHaveBeenCalledWith('完整思考', expect.any(Object))
    expect(outcome).toMatchObject({
      finishedNormally: true,
      streamFailed: false,
      terminalEventReceived: true,
      streamEventCursor: '11',
    })
  })

  it('requests canonical rehydrate without terminalizing an active Task', async () => {
    const fixture = consumerFixture([createAgentReasoningMessage({ text: 'stale' })])
    const outcome = await fixture.consumer.consume(
      eventStream([
        {
          id: '0',
          event: 'task_rehydrate_required',
          data: JSON.stringify({
            code: 'agent_stream.rehydrate_required',
            task_id: 'task-1',
            cursor: 41,
            settled: false,
          }),
        },
        {
          id: '42',
          event: 'thinking',
          data: JSON.stringify({ content: 'must-not-be-consumed' }),
        },
      ]),
      fixture.consumer.initialOutcome('17'),
    )

    expect(fixture.liveAccumulator.resetForCheckpoint).toHaveBeenCalledOnce()
    expect(fixture.liveAccumulator.appendThinking).not.toHaveBeenCalled()
    expect(fixture.messages()).toEqual([])
    expect(outcome).toMatchObject({
      finishedNormally: false,
      streamFailed: false,
      terminalEventReceived: false,
      streamEventCursor: '17',
      displayRehydrate: { taskId: 'task-1', cursor: 41, settled: false },
    })
  })

  it('does not manufacture a persistence barrier for a structural checkpoint', async () => {
    const fixture = consumerFixture()
    const outcome = await fixture.consumer.consume(
      eventStream([
        {
          event: 'task_checkpoint',
          data: JSON.stringify({ version: 1, cursor: 5, complete: true }),
        },
        {
          event: 'context_compaction',
          data: JSON.stringify({ id: 'compact-1', status: 'completed' }),
        },
        {
          id: '5',
          event: 'task_checkpoint_committed',
          data: JSON.stringify({ version: 1, cursor: 5 }),
        },
        { id: '6', event: 'done', data: '{}' },
      ]),
      fixture.consumer.initialOutcome('2'),
    )

    expect(outcome).toMatchObject({
      finishedNormally: true,
      persistenceRequired: false,
      streamFailed: false,
      terminalEventReceived: true,
      streamEventCursor: '6',
    })
    expect(buildAgentMessageViews(fixture.messages()).some((view) => view.kind === 'error')).toBe(false)
  })
})
