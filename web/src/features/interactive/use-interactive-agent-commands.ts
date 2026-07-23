import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createAgentCommandID } from '@/lib/api'
import type { AgentCommandDelivery, AgentRuntimeRecoveryAction } from '@/lib/api'
import { agentCommandErrorMessage, agentCommandRetryKey, isKnownAgentCommandOutcome, rememberAgentCommandID } from '@/lib/agent-command'
import {
  recoverInteractiveAgentRuntime,
  submitInteractiveAgentCommand,
  type ActiveInteractiveChat,
} from './api'
import type { StoryStageRuntimeState } from './stores/interactive-store'

export type StoryStageRuntimeUpdater = StoryStageRuntimeState | ((current: StoryStageRuntimeState) => StoryStageRuntimeState)

interface InteractiveAgentCommandOptions {
  storyId: string
  branchId: string
  readRuntime: () => StoryStageRuntimeState
  onRuntimeChange: (runtime: StoryStageRuntimeUpdater) => void
}

/** Owns operation-targeted game commands; the stage remains a display adapter. */
export function useInteractiveAgentCommands({ storyId, branchId, readRuntime, onRuntimeChange }: InteractiveAgentCommandOptions) {
  const { t } = useTranslation()
  const [delivery, setDelivery] = useState<AgentCommandDelivery>('follow_up')
  const retryCommandIDsRef = useRef(new Map<string, string>())
  const recoveryAbortActionRef = useRef<AgentRuntimeRecoveryAction | null>(null)

  const project = useCallback((active: ActiveInteractiveChat, connection: StoryStageRuntimeState['connection'] = 'connected') => {
    recoveryAbortActionRef.current = active.recovery_actions?.find((action) => action.kind === 'abort') || null
    onRuntimeChange((current) => {
      const projected = runtimeFromActiveInteractiveChat(active, connection)
      const sameOperation = Boolean(projected.operationId && projected.operationId === current.operationId)
      return {
        ...projected,
        streamEventCursor: sameOperation ? current.streamEventCursor : '',
        abortPending: sameOperation ? current.abortPending : false,
      }
    })
  }, [onRuntimeChange])

  const requireProjectedOperation = useCallback(() => {
    const runtime = readRuntime()
    if (runtime.phase !== 'running' || !runtime.operationId) {
      throw new Error(t('chat.runtime.operationChanged'))
    }
    return runtime
  }, [readRuntime, t])

  const recover = useCallback(async (active: ActiveInteractiveChat) => {
    const recoveryAbort = active.recovery_actions?.find((action) => action.kind === 'abort') || null
    recoveryAbortActionRef.current = recoveryAbort
    onRuntimeChange((current) => ({
      ...current,
      recoveryAbortAvailable: Boolean(recoveryAbort),
    }))
    const actions = interactiveRecoveryActionsToSubmit(active)
    if (!actions.length) return active
    let taskId = active.task_id?.trim() || ''
    for (const action of actions) {
      const receipt = await recoverInteractiveAgentRuntime({ storyId, branchId, action })
      if (receipt.recovery_action.kind !== action.kind ||
        receipt.recovery_action.command_id !== action.command_id ||
        receipt.recovery_action.operation_id !== action.operation_id) {
        throw new Error(t('chat.runtime.operationChanged'))
      }
      taskId = receipt.task_id?.trim() || taskId
    }
    if (!taskId) throw new Error(t('chat.runtime.operationUnavailable'))
    const executionResumed = actions.some((action) => action.kind !== 'start_turn')
    if (executionResumed) recoveryAbortActionRef.current = null
    return {
      ...active,
      active: true,
      status: 'running' as const,
      task_id: taskId,
      // start_turn only restores the display stream; keep the server-derived
      // abort capability while the durable operation remains recovery-paused.
      recovery_paused: !executionResumed,
      runtime_recoverable: !executionResumed,
      stream_attached: true,
      recovery_actions: executionResumed ? [] : recoveryAbort ? [recoveryAbort] : [],
    }
  }, [branchId, onRuntimeChange, storyId, t])

  const enqueue = useCallback(async (message: string, styleScenes: string[]) => {
    const runtime = requireProjectedOperation()
    if (runtime.abortPending) throw new Error(t('chat.runtime.abortPending'))
    if (runtime.queue.some((item) => item.delivery === delivery)) throw new Error(t('chat.runtime.queueConflict'))
    const retryKey = agentCommandRetryKey(runtime.operationId, delivery, { message, styleScenes })
    const commandId = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
    try {
      const receipt = await submitInteractiveAgentCommand({
        type: delivery,
        commandId,
        targetOperationId: runtime.operationId,
        storyId,
        branchId,
        message,
        styleScenes,
      })
      retryCommandIDsRef.current.delete(retryKey)
      recoveryAbortActionRef.current = null
      onRuntimeChange((current) => current.operationId !== runtime.operationId
        ? current
        : {
            ...current,
            cursor: Math.max(current.cursor, receipt.cursor),
            operationId: receipt.operation_id,
            recoveryPaused: false,
            recoveryAbortAvailable: false,
            queue: [
              ...current.queue.filter((item) => item.command_id !== commandId),
              {
                command_id: commandId,
                operation_id: receipt.operation_id,
                delivery,
                message,
              },
            ],
          })
      return receipt
    } catch (error) {
      if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
      throw new Error(agentCommandErrorMessage(error, t))
    }
  }, [branchId, delivery, onRuntimeChange, requireProjectedOperation, storyId, t])

  const abort = useCallback(async () => {
    const recoveryAction = recoveryAbortActionRef.current
    if (recoveryAction) {
      try {
        const receipt = await recoverInteractiveAgentRuntime({ storyId, branchId, action: recoveryAction })
        if (receipt.recovery_action.kind !== recoveryAction.kind ||
          receipt.recovery_action.command_id !== recoveryAction.command_id ||
          receipt.recovery_action.operation_id !== recoveryAction.operation_id) {
          throw new Error(t('chat.runtime.operationChanged'))
        }
        recoveryAbortActionRef.current = null
        onRuntimeChange((current) => ({
          ...current,
          cursor: Math.max(current.cursor, receipt.cursor),
          recoveryPaused: false,
          recoveryAbortAvailable: false,
          abortPending: true,
        }))
        return { targeted: true as const, receipt }
      } catch (error) {
        if (isKnownAgentCommandOutcome(error)) {
          recoveryAbortActionRef.current = null
          onRuntimeChange((current) => ({ ...current, recoveryAbortAvailable: false }))
        }
        throw new Error(agentCommandErrorMessage(error, t))
      }
    }
    const runtime = requireProjectedOperation()
    if (runtime.abortPending) return { targeted: true as const }
    const retryKey = agentCommandRetryKey(runtime.operationId, 'abort', { reason: 'user_requested' })
    const commandId = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
    try {
      const receipt = await submitInteractiveAgentCommand({
        type: 'abort',
        commandId,
        targetOperationId: runtime.operationId,
        storyId,
        branchId,
        reason: 'user_requested',
      })
      retryCommandIDsRef.current.delete(retryKey)
      onRuntimeChange((current) => current.operationId !== runtime.operationId
        ? current
        : {
            ...current,
            cursor: Math.max(current.cursor, receipt.cursor),
            operationId: receipt.operation_id,
            abortPending: true,
          })
      return { targeted: true as const, receipt }
    } catch (error) {
      if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
      throw new Error(agentCommandErrorMessage(error, t))
    }
  }, [branchId, onRuntimeChange, requireProjectedOperation, storyId, t])

  return { abort, delivery, enqueue, project, recover, setDelivery }
}

function interactiveRecoveryActionsToSubmit(active: ActiveInteractiveChat) {
  if (!active.runtime_recoverable) return []
  if (active.stream_attached && !active.recovery_paused) return []
  const projected = active.recovery_actions || []
  const attach = active.stream_attached
    ? undefined
    : projected.find((action) => action.kind === 'start_turn')
  const stateChange = projected.find((action) => action.kind !== 'start_turn' && action.kind !== 'abort')
  return [attach, stateChange].filter((action): action is AgentRuntimeRecoveryAction => Boolean(action))
}

export function runtimeFromActiveInteractiveChat(
  active: ActiveInteractiveChat,
  connection: StoryStageRuntimeState['connection'] = 'connected',
): StoryStageRuntimeState {
  return {
    cursor: active.cursor || 0,
    phase: active.phase || (active.active ? 'running' : 'idle'),
    recoveryPaused: Boolean(active.recovery_paused),
    recoveryAbortAvailable: Boolean(active.recovery_actions?.some((action) => action.kind === 'abort')),
    operationId: active.active_operation_id || '',
    cycle: active.active_cycle || 0,
    activeOutput: active.active_output,
    queue: active.queue || [],
    openTools: active.open_tools || [],
    connection,
    streamEventCursor: '',
    abortPending: false,
  }
}
