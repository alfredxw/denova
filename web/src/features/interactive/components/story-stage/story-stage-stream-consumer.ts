import type { TFunction } from 'i18next'
import type { AgentMessageMetadata, AgentUIMessage } from '@/lib/agent-ui'
import { withErrorLogID } from '@/lib/api-client'
import { agentMessageHasDataPart, createAgentDataMessage } from '@/lib/agent-ui-message'
import { createInteractiveNarrativeFilter } from '../../stream-parser'
import type { StoryStageRunState } from '../../stores/interactive-store'
import type { InteractiveSSEEvent, InteractiveTurnPersistedEvent, Snapshot } from '../../types'
import type { StoryStageRuntimeUpdater } from '../../use-interactive-agent-commands'
import { streamMetadataFromPayload } from './live-stream-messages'
import { decodeInteractiveStreamEvent, type DecodedHandledStreamEvent } from './story-stage-stream-decoder'
import { buildTokenUsageMessage, upsertTokenUsageMessage } from './token-usage'
import type { LiveMessageAccumulator } from './use-live-message-accumulator'

export type StoryStageStreamOutcome = {
  finishedNormally: boolean
  /** True only after the latest observed Agent cycle was durably persisted. */
  receivedPersistedTurn: boolean
  /**
   * Tracks whether the latest observed Agent cycle still needs its persistence
   * barrier. Structural operations never set this flag, so compact/remove may
   * settle with `done` without manufacturing an interactive turn.
   */
  persistenceRequired: boolean
  persistedSnapshot?: Snapshot
  terminalReason?: string
  terminalStatus?: TaskCheckpointStatus
  terminalEventReceived: boolean
  streamFailed: boolean
  streamEventCursor: string
  streamHandoffTaskId?: string
  displayRehydrate?: {
    taskId: string
    cursor: number
    settled: boolean
  }
}

type InteractiveAgentCycleStarted = Extract<DecodedHandledStreamEvent, { type: 'agent_cycle_started' }>['data']

type TaskCheckpointStatus = 'running' | 'done' | 'aborted' | 'error'

interface StoryStageStreamConsumerOptions {
  liveAccumulator: LiveMessageAccumulator
  liveTurnNavigationAnchorId: string
  onRuntimeRecoveryRequired: () => Promise<{ handoffTaskId?: string } | void>
  onTurnPersisted: (event: InteractiveTurnPersistedEvent) => Snapshot | void
  setActivity: (content: string) => void
  setMessages: (updater: AgentUIMessage[] | ((current: AgentUIMessage[]) => AgentUIMessage[])) => void
  setStageRuntime: (runtime: StoryStageRuntimeUpdater) => void
  t: TFunction
  updateStageRun: (updater: Partial<StoryStageRunState> | ((current: StoryStageRunState) => StoryStageRunState)) => void
}

/**
 * Projects one resumable game SSE stream into display-only stage state. The
 * returned outcome carries terminal and persistence barriers across reconnects;
 * command admission and transport retry remain outside this boundary.
 */
export function createStoryStageStreamConsumer({
  liveAccumulator,
  liveTurnNavigationAnchorId,
  onRuntimeRecoveryRequired,
  onTurnPersisted,
  setActivity,
  setMessages,
  setStageRuntime,
  t,
  updateStageRun,
}: StoryStageStreamConsumerOptions) {
  function initialOutcome(streamEventCursor = ''): StoryStageStreamOutcome {
    return {
      finishedNormally: false,
      receivedPersistedTurn: false,
      persistenceRequired: false,
      terminalEventReceived: false,
      streamFailed: false,
      streamEventCursor,
    }
  }

  function settleInactiveProjection(previous: StoryStageStreamOutcome): StoryStageStreamOutcome {
    const next = { ...previous, terminalEventReceived: true }
    if (next.persistenceRequired) {
      setMessages([errorMessage(t('storyStage.activity.persistenceMissing'))])
      next.finishedNormally = false
      next.streamFailed = true
      return next
    }
    if (next.streamFailed) return next
    switch (next.terminalStatus) {
      case 'error':
        setMessages([errorMessage(next.terminalReason || t('storyStage.activity.unknownError'))])
        next.streamFailed = true
        return next
      case 'aborted':
        if (!isUserRequestedAbort(next.terminalReason)) {
          setMessages((current) => [...current, errorMessage(next.terminalReason || t('storyStage.activity.aborted'))])
        }
        next.finishedNormally = false
        return next
      case 'running':
      case 'done':
      case undefined:
        next.finishedNormally = true
        return next
    }
  }

  async function consume(stream: ReadableStream<InteractiveSSEEvent>, previous: StoryStageStreamOutcome): Promise<StoryStageStreamOutcome> {
    let narrativeFilter = createInteractiveNarrativeFilter()
    let rootNarrativeMetadata: AgentMessageMetadata = {}
    let {
      finishedNormally,
      persistenceRequired,
      persistedSnapshot,
      receivedPersistedTurn,
      streamEventCursor,
      streamFailed,
      terminalReason,
      terminalStatus,
    } = previous
    let terminalEventReceived = false
    let streamHandoffTaskId: string | undefined
    let displayRehydrate: StoryStageStreamOutcome['displayRehydrate']
    let connectionEstablished = false
    let checkpointReplay = false
    const reader = stream.getReader()
    streamEvents: while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (!connectionEstablished) {
        connectionEstablished = true
        setStageRuntime((current) => ({ ...current, connection: 'connected' }))
      }
      if (value.id && value.id !== '0') {
        streamEventCursor = value.id
        updateStageRun((current) => ({
          ...current,
          runtime: {
            ...current.runtime,
            streamEventCursor: value.id || current.runtime.streamEventCursor,
          },
        }))
      }
      const decoded = decodeInteractiveStreamEvent(value)
      if (decoded.kind === 'unknown') {
        console.warn('[interactive-stage] received unknown stream event', { event: decoded.type, id: value.id })
        setActivity(t('storyStage.activity.unsupportedEvent', { event: decoded.type || 'unknown' }))
        continue
      }
      if (decoded.kind === 'invalid') {
        console.warn('[interactive-stage] rejected malformed stream event', {
          event: decoded.type,
          id: value.id,
          issue: decoded.issue,
        })
        setActivity(t('storyStage.activity.invalidEvent', { event: decoded.type }))
        continue
      }
      if (decoded.kind === 'ignored') continue

      const event = decoded.event
      switch (event.type) {
        case 'task_checkpoint': {
          const data = event.data
          if (data.complete === false) {
            console.warn('[interactive-stage] display checkpoint is explicitly incomplete', { cursor: data.cursor })
          }
          liveAccumulator.resetForCheckpoint()
          narrativeFilter = createInteractiveNarrativeFilter()
          rootNarrativeMetadata = {}
          finishedNormally = false
          persistenceRequired = false
          persistedSnapshot = undefined
          receivedPersistedTurn = false
          streamFailed = false
          terminalStatus = data.status
          terminalReason = data.terminal_reason?.trim() || undefined
          terminalEventReceived = false
          streamHandoffTaskId = undefined
          checkpointReplay = true
          setActivity(t('storyStage.activity.recovering'))
          break
        }
        case 'task_checkpoint_committed': {
          checkpointReplay = false
          break
        }
        case 'task_rehydrate_required': {
          const data = event.data
          const taskId = data.task_id
          const cursor = data.cursor
          liveAccumulator.resetForCheckpoint()
          narrativeFilter = createInteractiveNarrativeFilter()
          rootNarrativeMetadata = {}
          if (typeof data.persistence_required === 'boolean') {
            persistenceRequired = data.persistence_required
          }
          persistedSnapshot = undefined
          receivedPersistedTurn = false
          finishedNormally = false
          streamFailed = false
          terminalStatus = data.status
          terminalReason = data.terminal_reason?.trim() || undefined
          terminalEventReceived = false
          setActivity(t('storyStage.activity.recovering'))
          // Cursor zero on the envelope deliberately does not certify the
          // omitted range. The runtime first reloads canonical story state,
          // then reconnects this exact Task after its server-issued cursor.
          displayRehydrate = {
            taskId,
            cursor,
            settled: data.settled === true,
          }
          await reader.cancel()
          break streamEvents
        }
        case 'agent_cycle_started': {
          const data = event.data
          const { text, reset } = narrativeFilter.flush()
          if (reset) liveAccumulator.resetAssistant()
          liveAccumulator.collapseNonNarrative()
          if (text) liveAccumulator.appendAssistant(text, liveTurnNavigationAnchorId, rootNarrativeMetadata)
          liveAccumulator.finishMessages()
          narrativeFilter = createInteractiveNarrativeFilter()
          rootNarrativeMetadata = {}
          receivedPersistedTurn = false
          persistenceRequired = true
          beginAgentCycle(data, checkpointReplay)
          break
        }
        case 'chunk': {
          const data = event.data
          if (data.subagent) {
            liveAccumulator.appendAssistant(data.content || '', liveTurnNavigationAnchorId, streamMetadataFromPayload(data))
            setActivity('')
            break
          }
          // The per-turn render key is the durable Game narrative identity.
          // Keep Run/phase provenance, but do not replace it with a display-only
          // segment ID that the persisted domain turn cannot reproduce.
          rootNarrativeMetadata = { ...streamMetadataFromPayload(data), display_segment_id: undefined }
          const { text, reset } = narrativeFilter.push(data.content || '')
          if (reset) liveAccumulator.resetAssistant()
          if (text) {
            liveAccumulator.collapseNonNarrative()
            liveAccumulator.appendAssistant(text, liveTurnNavigationAnchorId, rootNarrativeMetadata)
          }
          setActivity('')
          break
        }
        case 'subagent_settled': {
          const data = event.data
          liveAccumulator.flush()
          const metadata = streamMetadataFromPayload(data)
          const source = metadata.subagent_session_id || metadata.parent_call_id || metadata.agent_name || 'subagent'
          const messageID = `subagent-settled:${metadata.run_id || 'run'}:${source}`
          const message = createAgentDataMessage({
            id: messageID,
            partId: messageID,
            type: 'agent-activity',
            data: { ...data, event: 'subagent_settled' },
            metadata,
          })
          setMessages((current) => current.some((candidate) => candidate.id === messageID) ? current : [...current, message])
          setActivity('')
          break
        }
        case 'thinking': {
          const data = event.data
          liveAccumulator.appendThinking(data.content || '', streamMetadataFromPayload(data))
          setActivity(t('storyStage.activity.thinking'))
          break
        }
        case 'interactive_content_reclassified': {
          const data = event.data
          liveAccumulator.resetAssistant()
          liveAccumulator.appendThinking(data.content || '', streamMetadataFromPayload(data))
          setActivity(t('storyStage.activity.thinking'))
          break
        }
        case 'tool_call': {
          const data = event.data
          liveAccumulator.flush()
          liveAccumulator.appendToolCall(data)
          setActivity(
            t('storyStage.activity.processingTool', {
              name: data.name || t('storyStage.activity.toolCall'),
            }),
          )
          break
        }
        case 'tool_args_delta': {
          const data = event.data
          liveAccumulator.appendToolArgs(data)
          break
        }
        case 'tool_started':
        case 'tool_progress': {
          const data = event.data
          setActivity(t('storyStage.activity.processingTool', {
            name: typeof data.name === 'string' && data.name.trim()
              ? data.name
              : t('storyStage.activity.toolCall'),
          }))
          break
        }
        case 'tool_result': {
          const data = event.data
          liveAccumulator.flush()
          liveAccumulator.completeToolCall(data, data.content || '')
          liveAccumulator.appendRuleRoll(data)
          setActivity('')
          break
        }
        case 'context_compaction': {
          const data = event.data
          liveAccumulator.flush()
          liveAccumulator.appendContextCompaction(data)
          setActivity('')
          if (data.status === 'completed' || data.status === 'failed') liveAccumulator.resetCompaction()
          break
        }
        case 'token_usage': {
          const data = event.data
          liveAccumulator.flush()
          setMessages((current) => upsertTokenUsageMessage(current, buildTokenUsageMessage(data)))
          break
        }
        case 'interactive_turn_persisted': {
          const data = event.data
          liveAccumulator.flush()
          receivedPersistedTurn = true
          persistenceRequired = false
          if (data.turn?.id) liveAccumulator.bindPersistedTurn(data.turn.id)
          const appliedSnapshot = onTurnPersisted(data)
          persistedSnapshot = appliedSnapshot || persistedSnapshot
          if (appliedSnapshot) {
            liveAccumulator.finishMessages()
            setMessages((current) => current.filter((message) => agentMessageHasDataPart(message, 'agent-token-usage')))
          }
          setActivity('')
          break
        }
        case 'runtime_recovery_required': {
          setActivity(t('storyStage.activity.recovering'))
          const recovery = await onRuntimeRecoveryRequired()
          if (recovery?.handoffTaskId) {
            streamHandoffTaskId = recovery.handoffTaskId
            await reader.cancel()
            break streamEvents
          }
          setActivity(t('storyStage.activity.thinking'))
          break
        }
        case 'goal_evaluation_failed': {
          liveAccumulator.flush()
          setMessages((current) => [
            ...current,
            errorMessage(t('storyStage.activity.goalEvaluationFailed')),
          ])
          break
        }
        case 'error': {
          const data = event.data
          if (data.code === 'agent_runtime.recovery_required') {
            setActivity(t('storyStage.activity.recovering'))
            const recovery = await onRuntimeRecoveryRequired()
            if (recovery?.handoffTaskId) {
              streamHandoffTaskId = recovery.handoffTaskId
              await reader.cancel()
              break streamEvents
            }
            setActivity(t('storyStage.activity.thinking'))
            break
          }
          liveAccumulator.flush()
          liveAccumulator.finishMessages()
          setActivity('')
          streamFailed = true
          terminalStatus = 'error'
          const message = withErrorLogID(data.message || data.error || t('storyStage.activity.unknownError'), data)
          terminalReason = message
          terminalEventReceived = true
          setMessages((current) => [
            ...current,
            errorMessage(message),
          ])
          break
        }
        case 'done':
        case 'aborted': {
          const { text, reset } = narrativeFilter.flush()
          if (reset) liveAccumulator.resetAssistant()
          liveAccumulator.collapseNonNarrative()
          if (text) liveAccumulator.appendAssistant(text, liveTurnNavigationAnchorId, rootNarrativeMetadata)
          liveAccumulator.finishMessages(event.type === 'aborted' ? 'cancelled' : 'success')
          if (event.type === 'aborted') {
            const data = event.data
            terminalStatus = 'aborted'
            terminalReason =
              typeof data.reason === 'string' ? data.reason.trim() : typeof data.message === 'string' ? data.message.trim() : undefined
            if (!isUserRequestedAbort(terminalReason)) {
              setMessages((current) => [...current, errorMessage(terminalReason || t('storyStage.activity.aborted'))])
            }
          } else if (persistenceRequired && !streamFailed) {
            terminalStatus = 'done'
            streamFailed = true
            setMessages([errorMessage(t('storyStage.activity.persistenceMissing'))])
          } else if (!streamFailed) {
            terminalStatus = 'done'
            finishedNormally = true
          }
          setActivity('')
          terminalEventReceived = true
          break
        }
        default:
          assertNever(event)
      }
    }
    return {
      finishedNormally,
      persistenceRequired,
      persistedSnapshot,
      receivedPersistedTurn,
      streamEventCursor,
      streamFailed,
      terminalReason,
      terminalStatus,
      terminalEventReceived,
      streamHandoffTaskId,
      displayRehydrate,
    }
  }

  function beginAgentCycle(data: InteractiveAgentCycleStarted, fromCheckpoint: boolean) {
    updateStageRun((current) => ({
      ...current,
      runtime: {
        ...current.runtime,
        phase: 'running',
        recoveryPaused: false,
        recoveryAbortAvailable: false,
        operationId: data.operation_id || current.runtime.operationId,
        cycle: data.cycle || current.runtime.cycle,
        queue: current.runtime.queue.filter((item) => item.command_id !== data.command_id),
        connection: 'connected',
      },
    }))
    const message = data.message?.trim()
    if (data.delivery === 'start_turn') {
      if (fromCheckpoint && message) {
        liveAccumulator.prepareTurn(message, liveTurnNavigationAnchorId, 'replace')
        updateStageRun({ retryMessage: message, rewindTurnId: undefined })
        setActivity(t('storyStage.activity.thinking'))
      }
      return
    }
    if (!message) return
    liveAccumulator.prepareTurn(message, liveTurnNavigationAnchorId, 'append')
    updateStageRun({ retryMessage: message, rewindTurnId: undefined })
    setActivity(t('storyStage.activity.thinking'))
  }

  return { consume, initialOutcome, settleInactiveProjection }
}

function isUserRequestedAbort(reason?: string) {
  return reason?.trim() === 'user_requested'
}

function assertNever(value: never): never {
  throw new Error(`Unhandled interactive stream event: ${value}`)
}

function errorMessage(content: string) {
  return createAgentDataMessage({ type: 'agent-error', data: { role: 'error', content } })
}
