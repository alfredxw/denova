import type { TFunction } from 'i18next'
import { describe, expect, it, vi } from 'vitest'
import type { ChatMessage } from '@/lib/api'
import type { InteractiveSSEEvent } from '../../types'
import { createStoryStageStreamConsumer } from './story-stage-stream-consumer'
import type { LiveMessageAccumulator } from './use-live-message-accumulator'

function eventStream(events: InteractiveSSEEvent[]) {
  return new ReadableStream<InteractiveSSEEvent>({
    start(controller) {
      for (const event of events) controller.enqueue(event)
      controller.close()
    },
  })
}

function consumerFixture(initialMessages: ChatMessage[] = []) {
  let messages = initialMessages
  const setMessages = vi.fn((updater: ChatMessage[] | ((current: ChatMessage[]) => ChatMessage[])) => {
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
  const consumer = createStoryStageStreamConsumer({
    liveAccumulator,
    liveTurnNavigationAnchorId: 'live-turn',
    onRuntimeRecoveryRequired: vi.fn().mockResolvedValue(undefined),
    onTurnPersisted: vi.fn(),
    setActivity: vi.fn(),
    setMessages,
    setStageRuntime: vi.fn(),
    t: ((key: string) => key) as unknown as TFunction,
    updateStageRun: vi.fn(),
  })
  return { consumer, liveAccumulator, messages: () => messages }
}

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
    const fixture = consumerFixture([{ role: 'thinking', content: 'stale' }])
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
          data: JSON.stringify({ turn: { id: 'turn-1' } }),
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
    const fixture = consumerFixture([{ role: 'thinking', content: 'stale' }])
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
    expect(fixture.messages()).not.toEqual([{ role: 'error', content: 'storyStage.activity.persistenceMissing' }])
  })
})
