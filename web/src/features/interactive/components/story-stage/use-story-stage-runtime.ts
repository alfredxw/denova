import { useRef, useState } from 'react'
import type { TFunction } from 'i18next'
import { buildContextCompactionMessage, createContextCompactionMessageId, settleContextCompactionMessages } from '@/components/Chat/context-compaction-message'
import { createAgentCommandID, type AgentRuntimeQueuedCommand } from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { createAgentDataMessage } from '@/lib/agent-ui-message'
import { agentCommandRetryKey, isKnownAgentCommandOutcome, rememberAgentCommandID } from '@/lib/agent-command'
import {
  compactInteractiveContext,
  getActiveInteractiveChat,
  sendInteractiveMessage,
  streamActiveInteractiveChat,
  type ActiveInteractiveChat,
} from '../../api'
import type { InteractiveSSEEvent, InteractiveTurnPersistedEvent, Snapshot } from '../../types'
import type { StoryStageRunState } from '../../stores/interactive-store'
import { useInteractiveStore } from '../../stores/interactive-store'
import type { StoryStageRuntimeUpdater } from '../../use-interactive-agent-commands'
import {
  clearStoryRunAbortController,
  registerStoryRunAbortController,
  useActiveStoryRunRecovery,
  wakeStoryRunRecovery,
} from '../../use-active-story-run'
import type { useInteractiveAgentCommands } from '../../use-interactive-agent-commands'
import { isAbortError, parseInlineStyleScenes } from './utils'
import type { LiveMessageAccumulator } from './use-live-message-accumulator'
import type { StoryImagesController } from './use-story-images'
import { createStoryStageStreamConsumer, type StoryStageStreamOutcome } from './story-stage-stream-consumer'
import {
  isRetryableStoryStageObservationError,
  isStoryStageProjectionRefreshError,
  storyStageProjectionFingerprint,
  waitForStoryStageReconnect,
} from './story-stage-recovery-transport'

type InteractiveAgentCommands = ReturnType<typeof useInteractiveAgentCommands>

interface UseStoryStageRuntimeOptions {
  storyId: string
  branchId: string
  stageKey: string
  input: string
  editingTurnId?: string
  styleScenes: string[]
  streaming: boolean
  branchTerminal: boolean
  blocked: boolean
  stageRun: StoryStageRunState
  liveTurnNavigationAnchorId: string
  t: TFunction
  interactiveAgentCommands: InteractiveAgentCommands
  liveAccumulator: LiveMessageAccumulator
  storyImages: StoryImagesController
  readStageRuntime: () => StoryStageRunState['runtime']
  setStageRuntime: (runtime: StoryStageRuntimeUpdater) => void
  updateStageRun: (updater: Partial<StoryStageRunState> | ((current: StoryStageRunState) => StoryStageRunState)) => void
  setStreaming: (value: boolean) => void
  setActivity: (content: string) => void
  setMessages: (updater: AgentUIMessage[] | ((current: AgentUIMessage[]) => AgentUIMessage[])) => void
  clearComposer: () => void
  onTurnPersisted: (event: InteractiveTurnPersistedEvent) => Snapshot | void
  onDone: (options?: { silent?: boolean }) => void | Promise<Snapshot | void>
}

// Coordinates one durable agent operation with its resumable SSE display stream.
// Root game turns are admitted while idle; active-run input becomes a durable
// Follow Up that can be steered from the projected queue. This hook also owns
// transport recovery, abort, event projection and terminal settlement.
export function useStoryStageRuntime({
  storyId,
  branchId,
  stageKey,
  input,
  editingTurnId,
  styleScenes,
  streaming,
  branchTerminal,
  blocked,
  stageRun,
  liveTurnNavigationAnchorId,
  t,
  interactiveAgentCommands,
  liveAccumulator,
  storyImages,
  readStageRuntime,
  setStageRuntime,
  updateStageRun,
  setStreaming,
  setActivity,
  setMessages,
  clearComposer,
  onTurnPersisted,
  onDone,
}: UseStoryStageRuntimeOptions) {
  const [commandSubmitting, setCommandSubmitting] = useState(false)
  const [queueActionPendingCommandID, setQueueActionPendingCommandID] = useState('')
  const commandSubmittingRef = useRef(false)
  const compactionIdCounterRef = useRef(0)
  const initialStartCommandIDsRef = useRef(new Map<string, string>())
  const streamConsumer = createStoryStageStreamConsumer({
    liveAccumulator,
    liveTurnNavigationAnchorId,
    onRuntimeRecoveryRequired: recoverAttachedRuntime,
    onTurnPersisted,
    setActivity,
    setMessages,
    setStageRuntime,
    t,
    updateStageRun,
  })

  useActiveStoryRunRecovery({
    stageKey,
    storyId,
    branchId,
    isStreaming: () => Boolean(useInteractiveStore.getState().storyStageRuns[stageKey]?.streaming),
    onResume: resumeActiveStoryRun,
    onDetach: () =>
      updateStageRun((current) => ({
        ...current,
        streaming: false,
        activityContent: '',
        runtime: { ...current.runtime, connection: 'disconnected' },
      })),
  })

  async function send(override?: { message?: string; rewindTurnId?: string }) {
    const message = (override?.message ?? input).trim()
    if (!message || !storyId || branchTerminal || blocked || commandSubmittingRef.current) return false
    if (streaming) {
      const runtime = readStageRuntime()
      if (runtime.abortPending || !runtime.operationId || runtime.connection !== 'connected') return false
      return submitFollowUp(message)
    }
    if (message === '/compact') {
      await compactCurrentContext()
      return true
    }
    const rewindTurnId = override?.rewindTurnId ?? editingTurnId
    const inlineStyleScenes = parseInlineStyleScenes(message)
    const mergedStyleScenes = Array.from(new Set([...styleScenes, ...inlineStyleScenes]))
    clearComposer()
    commandSubmittingRef.current = true
    setCommandSubmitting(true)
    prepareLiveRun(message, rewindTurnId)
    const abortController = new AbortController()
    registerStoryRunAbortController(stageKey, abortController)
    const request = {
      mode: 'story' as const,
      story_id: storyId,
      branch: branchId,
      message,
      style_scenes: mergedStyleScenes,
      regenerate_from_turn_id: rewindTurnId || undefined,
    }
    const retryKey = agentCommandRetryKey('', 'start_turn', request)
    const commandID = rememberAgentCommandID(initialStartCommandIDsRef.current, retryKey, createAgentCommandID)
    try {
      const stream = await sendInteractiveMessage({
        ...request,
        command_id: commandID,
        signal: abortController.signal,
      })
      initialStartCommandIDsRef.current.delete(retryKey)
      commandSubmittingRef.current = false
      setCommandSubmitting(false)
      const outcome = await observeStream(stream, abortController)
      if (outcome.terminalEventReceived) {
        await completeStream(outcome)
        finishRun(abortController)
      }
      return true
    } catch (error) {
      if (isKnownAgentCommandOutcome(error)) initialStartCommandIDsRef.current.delete(retryKey)
      handleStreamError(error)
      finishRun(abortController)
      return false
    }
  }

  async function submitFollowUp(message: string) {
    const inlineStyleScenes = parseInlineStyleScenes(message)
    const mergedStyleScenes = Array.from(new Set([...styleScenes, ...inlineStyleScenes]))
    commandSubmittingRef.current = true
    setCommandSubmitting(true)
    try {
      await interactiveAgentCommands.followUp({ message, styleScenes: mergedStyleScenes })
      clearComposer()
      return true
    } catch (error) {
      appendError(error)
      return false
    } finally {
      commandSubmittingRef.current = false
      setCommandSubmitting(false)
    }
  }

  async function stop() {
    if (commandSubmittingRef.current || stageRun.runtime.abortPending) return
    commandSubmittingRef.current = true
    setCommandSubmitting(true)
    setActivity(t('storyStage.activity.aborting'))
    try {
      const outcome = await interactiveAgentCommands.abort()
      const recoveryTaskID = outcome.receipt && 'task_id' in outcome.receipt ? String(outcome.receipt.task_id || '').trim() : ''
      if (recoveryTaskID) wakeStoryRunRecovery(stageKey)
    } catch (error) {
      setActivity('')
      appendError(error)
    } finally {
      commandSubmittingRef.current = false
      setCommandSubmitting(false)
    }
  }

  async function steerQueuedCommand(item: AgentRuntimeQueuedCommand) {
    return submitQueuedControl(item, 'steer')
  }

  async function deleteQueuedCommand(item: AgentRuntimeQueuedCommand) {
    return submitQueuedControl(item, 'cancel')
  }

  async function submitQueuedControl(item: AgentRuntimeQueuedCommand, action: 'steer' | 'cancel') {
    if (commandSubmittingRef.current || stageRun.runtime.abortPending) return false
    commandSubmittingRef.current = true
    setCommandSubmitting(true)
    setQueueActionPendingCommandID(item.command_id)
    try {
      if (action === 'steer') await interactiveAgentCommands.steerQueued(item)
      else await interactiveAgentCommands.cancelQueued(item)
      return true
    } catch (error) {
      appendError(error)
      return false
    } finally {
      commandSubmittingRef.current = false
      setCommandSubmitting(false)
      setQueueActionPendingCommandID('')
    }
  }

  async function compactCurrentContext() {
    if (!storyId || streaming) return
    clearComposer()
    setStreaming(true)
    setActivity('')
    liveAccumulator.resetCompaction()
    setMessages([buildContextCompactionMessage({ status: 'started', phase: 'pre_run' }, createContextCompactionMessageId(compactionIdCounterRef))])
    try {
      await compactInteractiveContext(storyId, branchId)
      setMessages((current) => [
        ...settleContextCompactionMessages(current, 'success'),
        systemMessage(t('storyStage.contextCompaction.done')),
      ])
      await onDone()
    } catch (error) {
      setMessages((current) => [
        ...settleContextCompactionMessages(current, 'error'),
        errorMessage(error instanceof Error ? error.message : t('storyStage.contextCompaction.failed')),
      ])
    } finally {
      setStreaming(false)
      liveAccumulator.resetCompaction()
      setActivity('')
    }
  }

  async function resumeActiveStoryRun(active: ActiveInteractiveChat, abortController: AbortController, isDisposed: () => boolean) {
    const message = active.message?.trim() || ''
    const previousRuntime = readStageRuntime()
    prepareLiveRun(message, active.regenerate_from_turn_id)
    try {
      if (active.runtime_recoverable) setActivity(t('storyStage.activity.recovering'))
      const recovered = await interactiveAgentCommands.recover(active)
      if (isDisposed()) return
      const taskId = recovered.task_id?.trim()
      if (!taskId) throw new Error(t('chat.runtime.operationUnavailable'))
      const sameOperation = Boolean(recovered.active_operation_id && recovered.active_operation_id === previousRuntime.operationId)
      interactiveAgentCommands.project(recovered, 'connecting')
      if (sameOperation) {
        setStageRuntime((current) => ({
          ...current,
          streamEventCursor: previousRuntime.streamEventCursor,
          abortPending: previousRuntime.abortPending,
        }))
      }
      const stream = await streamActiveInteractiveChat({
        storyId,
        branchId,
        taskId,
        after: sameOperation ? previousRuntime.streamEventCursor || undefined : undefined,
        signal: abortController.signal,
      })
      if (isDisposed()) return
      interactiveAgentCommands.project(recovered, 'connected')
      setActivity(t('storyStage.activity.thinking'))
      const outcome = await observeStream(stream, abortController, isDisposed)
      if (outcome.terminalEventReceived) {
        await completeStream(outcome)
        finishRun(abortController)
      }
    } catch (error) {
      if (!isDisposed() && !isAbortError(error)) {
        if (isRetryableStoryStageObservationError(error)) {
          updateStageRun((current) => ({
            ...current,
            streaming: true,
            runtime: { ...current.runtime, connection: 'disconnected' },
          }))
          setActivity(t('storyStage.activity.recovering'))
          throw error
        }
        handleStreamError(error)
        finishRun(abortController)
      }
    }
  }

  async function recoverAttachedRuntime() {
    let failedProjection = ''
    while (true) {
      const projected = await getActiveInteractiveChat(storyId, branchId)
      const projectionFingerprint = storyStageProjectionFingerprint(projected)
      if (!projected.runtime_recoverable || !projected.recovery_paused) {
        interactiveAgentCommands.project(projected, 'connected')
        return
      }
      // A concurrent tab can advance the same Task between GET and POST. Retry
      // immediately only when the authoritative projection changed; observing
      // the same failed identity twice means this reader should keep waiting
      // for a later control event instead of spinning or terminating the run.
      if (projectionFingerprint === failedProjection) {
        interactiveAgentCommands.project(projected, 'connected')
        return
      }
      const taskId = projected.task_id?.trim() || ''
      try {
        const recovered = await interactiveAgentCommands.recover(projected)
        const recoveredTaskId = recovered.task_id?.trim() || ''
        // `stream_attached` can describe a retained, already-finished display
        // Task. Only an actually active Task owns the live recovery observer
        // strongly enough to require a stable identity across this POST.
        const attachedTaskMustRemainStable = Boolean(taskId && projected.active)
        if (attachedTaskMustRemainStable && recoveredTaskId && taskId !== recoveredTaskId) {
          throw new Error(t('chat.runtime.operationChanged'))
        }
        if (recoveredTaskId && recoveredTaskId !== taskId) {
          interactiveAgentCommands.project(recovered, 'connecting')
          return { handoffTaskId: recoveredTaskId }
        }
        interactiveAgentCommands.project(recovered, 'connected')
        return
      } catch (error) {
        if (!isStoryStageProjectionRefreshError(error)) throw error
        failedProjection = projectionFingerprint
      }
    }
  }

  async function observeStream(
    initialStream: ReadableStream<InteractiveSSEEvent>,
    abortController: AbortController,
    isDisposed: () => boolean = () => false,
  ): Promise<StoryStageStreamOutcome> {
    let stream = initialStream
    let progress = streamConsumer.initialOutcome(readStageRuntime().streamEventCursor)
    let lastImmediateReconnectRetry = ''
    while (!abortController.signal.aborted && !isDisposed()) {
      progress = await streamConsumer.consume(stream, progress)
      if (progress.terminalEventReceived) return progress
      if (progress.displayRehydrate) {
        const rehydrate = progress.displayRehydrate
        setActivity(t('storyStage.activity.recovering'))
        while (!abortController.signal.aborted && !isDisposed()) {
          try {
            await onDone({ silent: true })
            break
          } catch (error) {
            if (isAbortError(error) || abortController.signal.aborted || isDisposed()) throw error
            console.warn('[interactive-stage] Failed to restore canonical story history; preserving runtime state for retry', error)
            updateStageRun((current) => ({
              ...current,
              streaming: true,
              runtime: { ...current.runtime, connection: 'disconnected' },
            }))
            await waitForStoryStageReconnect(abortController.signal, isDisposed)
          }
        }
        if (abortController.signal.aborted || isDisposed()) return progress
        setMessages((current) => [
          ...current,
          systemMessage(t('storyStage.activity.rehydrateRequired')),
        ])
        progress = {
          ...progress,
          displayRehydrate: undefined,
          streamEventCursor: String(rehydrate.cursor),
        }
        setStageRuntime((current) => ({
          ...current,
          connection: rehydrate.settled ? 'disconnected' : 'connecting',
          streamEventCursor: String(rehydrate.cursor),
        }))
        if (rehydrate.settled) {
          return streamConsumer.settleInactiveProjection(progress)
        }
        while (!abortController.signal.aborted && !isDisposed()) {
          try {
            stream = await streamActiveInteractiveChat({
              storyId,
              branchId,
              taskId: rehydrate.taskId,
              after: String(rehydrate.cursor),
              signal: abortController.signal,
            })
            setStageRuntime((current) => ({
              ...current,
              connection: 'connected',
            }))
            setActivity(t('storyStage.activity.thinking'))
            break
          } catch (error) {
            if (isAbortError(error) || abortController.signal.aborted || isDisposed()) throw error
            console.warn('[interactive-stage] Failed to restore the display suffix for the same task; preserving runtime state for retry', error)
            setStageRuntime((current) => ({
              ...current,
              connection: 'disconnected',
            }))
            await waitForStoryStageReconnect(abortController.signal, isDisposed)
          }
        }
        continue
      }
      if (progress.streamHandoffTaskId) {
        const taskId = progress.streamHandoffTaskId
        progress = {
          ...progress,
          streamEventCursor: '',
          streamHandoffTaskId: undefined,
        }
        setStageRuntime((current) => ({
          ...current,
          connection: 'connecting',
          streamEventCursor: '',
        }))
        stream = await streamActiveInteractiveChat({
          storyId,
          branchId,
          taskId,
          signal: abortController.signal,
        })
        setStageRuntime((current) => ({ ...current, connection: 'connected' }))
        setActivity(t('storyStage.activity.thinking'))
        continue
      }
      updateStageRun((current) => ({
        ...current,
        streaming: true,
        runtime: { ...current.runtime, connection: 'disconnected' },
      }))
      setActivity(t('storyStage.activity.reconnecting'))
      while (!abortController.signal.aborted && !isDisposed()) {
        let projectionFingerprint = ''
        try {
          const projected = await getActiveInteractiveChat(storyId, branchId)
          projectionFingerprint = storyStageProjectionFingerprint(projected)
          if (projected.runtime_recoverable) setActivity(t('storyStage.activity.recovering'))
          const active = await interactiveAgentCommands.recover(projected)
          if (!active.active || !active.task_id?.trim()) {
            return streamConsumer.settleInactiveProjection(progress)
          }
          interactiveAgentCommands.project(active, 'connecting')
          stream = await streamActiveInteractiveChat({
            storyId,
            branchId,
            taskId: active.task_id,
            after: progress.streamEventCursor || undefined,
            signal: abortController.signal,
          })
          interactiveAgentCommands.project(active, 'connected')
          setActivity(t('storyStage.activity.thinking'))
          break
        } catch (error) {
          if (isAbortError(error) || abortController.signal.aborted || isDisposed()) throw error
          if (isStoryStageProjectionRefreshError(error) && projectionFingerprint && projectionFingerprint !== lastImmediateReconnectRetry) {
            lastImmediateReconnectRetry = projectionFingerprint
            continue
          }
          console.warn('[interactive-stage] Interactive stream reconnection failed; preserving runtime state until the next attempt', error)
          await waitForStoryStageReconnect(abortController.signal, isDisposed)
        }
      }
    }
    return progress
  }

  function prepareLiveRun(message: string, rewindTurnId?: string) {
    setActivity(message ? t('storyStage.activity.thinking') : t('storyStage.activity.recovering'))
    if (message) {
      liveAccumulator.prepareTurn(message, liveTurnNavigationAnchorId, 'replace')
      updateStageRun({
        rewindTurnId: rewindTurnId || undefined,
        retryMessage: message,
      })
    }
    liveAccumulator.beginStage(stageKey)
    updateStageRun((current) => ({
      ...current,
      streaming: true,
      runtime: {
        ...current.runtime,
        streamEventCursor: '',
        phase: 'running',
        recoveryPaused: false,
        recoveryAbortAvailable: false,
        operationId: '',
        cycle: 0,
        queue: [],
        openTools: [],
        connection: 'connected',
        abortPending: false,
      },
    }))
  }

  async function completeStream({ finishedNormally, receivedPersistedTurn, persistedSnapshot }: StoryStageStreamOutcome) {
    let nextSnapshot: Snapshot | void = persistedSnapshot
    if (persistedSnapshot) {
      void Promise.resolve(onDone({ silent: true })).catch((error) => console.warn('[interactive-stage] Silent interactive snapshot refresh failed', error))
    } else {
      nextSnapshot = await onDone(receivedPersistedTurn ? { silent: true } : undefined)
    }
    if (finishedNormally) await storyImages.maybeGenerateAutomatically(nextSnapshot)
  }

  function handleStreamError(error: unknown) {
    liveAccumulator.flush()
    liveAccumulator.finishMessages()
    setActivity('')
    setMessages((current) => [
      ...current,
      errorMessage(isAbortError(error)
        ? t('storyStage.activity.aborted')
        : error instanceof Error
          ? error.message
          : t('storyStage.activity.runFailed')),
    ])
  }

  function finishRun(abortController: AbortController) {
    if (!clearStoryRunAbortController(stageKey, abortController)) return
    updateStageRun((current) => ({
      ...current,
      streaming: false,
      runtime: {
        ...current.runtime,
        phase: 'idle',
        recoveryPaused: false,
        recoveryAbortAvailable: false,
        operationId: '',
        cycle: 0,
        activeOutput: undefined,
        queue: [],
        openTools: [],
        connection: 'disconnected',
        streamEventCursor: '',
        abortPending: false,
      },
    }))
    commandSubmittingRef.current = false
    setCommandSubmitting(false)
    liveAccumulator.endRun()
    setActivity('')
  }

  function appendError(error: unknown) {
    setMessages((current) => [
      ...current,
      errorMessage(error instanceof Error ? error.message : t('storyStage.activity.runFailed')),
    ])
  }

  return { commandSubmitting, deleteQueuedCommand, queueActionPendingCommandID, compactCurrentContext, send, steerQueuedCommand, stop }
}

function systemMessage(content: string) {
  return createAgentDataMessage({ type: 'agent-system', data: { role: 'system', content } })
}

function errorMessage(content: string) {
  return createAgentDataMessage({ type: 'agent-error', data: { role: 'error', content } })
}
