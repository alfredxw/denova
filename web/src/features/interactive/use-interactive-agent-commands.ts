import { useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { createAgentCommandID } from '@/lib/api'
import type { AgentQueuedCommandAction, AgentRuntimeQueuedCommand, AgentRuntimeRecoveryAction } from '@/lib/api'
import { agentCommandErrorMessage, agentCommandRetryKey, isKnownAgentCommandOutcome, mergeProjectedAgentQueue, rememberAgentCommandID } from '@/lib/agent-command'
import {
  recoverInteractiveAgentRuntime,
  submitInteractiveAgentCommand,
  type ActiveInteractiveChat,
} from './api'
import type { StoryStageRuntimeState } from './stores/interactive-store'
import { attachmentUploadsRetryIdentity, type ChatAttachmentUpload } from '@/lib/chat-attachments'

export type StoryStageRuntimeUpdater = StoryStageRuntimeState | ((current: StoryStageRuntimeState) => StoryStageRuntimeState)

interface InteractiveAgentCommandOptions {
  storyId: string
  branchId: string
  readRuntime: () => StoryStageRuntimeState
  onRuntimeChange: (runtime: StoryStageRuntimeUpdater) => void
}

/** Owns targeted game commands and server-authorized recovery; the stage remains a display adapter. */
export function useInteractiveAgentCommands({ storyId, branchId, readRuntime, onRuntimeChange }: InteractiveAgentCommandOptions) {
  const { t } = useTranslation()
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
      if (receipt.recovery_action.action_id !== action.action_id ||
        receipt.recovery_action.kind !== action.kind ||
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

  const abort = useCallback(async () => {
    const recoveryAction = recoveryAbortActionRef.current
    if (recoveryAction) {
      try {
        const receipt = await recoverInteractiveAgentRuntime({ storyId, branchId, action: recoveryAction })
        if (receipt.recovery_action.action_id !== recoveryAction.action_id ||
          receipt.recovery_action.kind !== recoveryAction.kind ||
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

  const followUp = useCallback(async (input: { message: string; styleScenes: string[]; attachments?: ChatAttachmentUpload[] }) => {
    const runtime = requireProjectedOperation()
    const payload = {
      message: input.message.trim(),
      style_scenes: Array.from(new Set(input.styleScenes.map((scene) => scene.trim()).filter(Boolean))),
      attachments: input.attachments || [],
    }
    const retryKey = agentCommandRetryKey(runtime.operationId, 'follow_up', {
      ...payload,
      attachments: attachmentUploadsRetryIdentity(payload.attachments),
    })
    const commandId = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
    try {
      const receipt = await submitInteractiveAgentCommand({
        type: 'follow_up',
        commandId,
        targetOperationId: runtime.operationId,
        storyId,
        branchId,
        input: {
          message: payload.message,
          styleScenes: payload.style_scenes,
          ...(payload.attachments.length ? { attachments: payload.attachments } : {}),
        },
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
            queue: mergeProjectedAgentQueue(current.queue, {
              command_id: commandId,
              operation_id: receipt.operation_id,
              delivery: 'follow_up',
              message: payload.message,
            }),
          })
      return { commandId, receipt }
    } catch (error) {
      if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
      throw new Error(agentCommandErrorMessage(error, t))
    }
  }, [branchId, onRuntimeChange, requireProjectedOperation, storyId, t])

  const submitQueuedControl = useCallback(async (
    item: AgentRuntimeQueuedCommand,
    action: AgentQueuedCommandAction,
    reason?: string,
  ) => {
    const runtime = requireProjectedOperation()
    if (item.operation_id !== runtime.operationId || !runtime.queue.some((candidate) => candidate.command_id === item.command_id)) {
      throw new Error(t('chat.runtime.invalidCommand'))
    }
    const payload = { target_command_id: item.command_id, ...(reason ? { reason } : {}) }
    const retryKey = agentCommandRetryKey(runtime.operationId, action, payload)
    const commandId = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
    try {
      const receipt = await submitInteractiveAgentCommand({
        type: action,
        commandId,
        targetOperationId: runtime.operationId,
        targetCommandId: item.command_id,
        storyId,
        branchId,
        ...(reason ? { reason } : {}),
      })
      retryCommandIDsRef.current.delete(retryKey)
      if (action === 'steer_queued') recoveryAbortActionRef.current = null
      onRuntimeChange((current) => current.operationId !== runtime.operationId
        ? current
        : {
            ...current,
            cursor: Math.max(current.cursor, receipt.cursor),
            operationId: receipt.operation_id,
            ...(action === 'steer_queued' ? {
              recoveryPaused: false,
              recoveryAbortAvailable: false,
            } : {}),
            queue: action === 'cancel_queued'
              ? current.queue.filter((candidate) => candidate.command_id !== item.command_id)
              : current.queue.map((candidate) => candidate.command_id === item.command_id
                ? { ...candidate, steer_requested: true }
                : candidate),
          })
      return receipt
    } catch (error) {
      if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
      throw new Error(agentCommandErrorMessage(error, t))
    }
  }, [branchId, onRuntimeChange, requireProjectedOperation, storyId, t])

  const steerQueued = useCallback(
    (item: AgentRuntimeQueuedCommand) => submitQueuedControl(item, 'steer_queued'),
    [submitQueuedControl],
  )
  const cancelQueued = useCallback(
    (item: AgentRuntimeQueuedCommand, reason = 'user_deleted') => submitQueuedControl(item, 'cancel_queued', reason),
    [submitQueuedControl],
  )

  return { abort, cancelQueued, followUp, project, recover, steerQueued }
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
    pendingInterruptionId: active.pending_interruption_id?.trim() || '',
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
