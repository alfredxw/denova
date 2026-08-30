import { useCallback, useEffect, useRef, useState } from 'react'
import type { ActiveChatTask, AgentRuntimeRecoveryAction, AgentRuntimeRecoveryReceipt } from '@/lib/api'
import type { AgentChatTransport } from '@/lib/agent-ui'
import { isAbortError } from './agent-chat-state'
import { writingAgentChatClient, type AgentChatClient } from './agent-chat-client'

export interface WritingDisplayRehydrateRequest {
  signal: number
  taskID: string
  cursor: number
  settled: boolean
  status?: WritingTaskStatus
  terminalReason?: string
  terminalReasonTruncated?: boolean
}

export type WritingTaskStatus = 'running' | 'done' | 'aborted' | 'error'

interface WritingAgentRuntimeRecoveryOptions {
  activeSessionId: string
  displayRehydrateRequest: WritingDisplayRehydrateRequest | null
  loadHistoryAuthoritative: (sessionId?: string) => Promise<void>
  onDisplayRehydrated: () => void
  onDisplayTerminalRestored: (request: WritingDisplayRehydrateRequest) => void
  onSettled: () => void
  runtimeRecoverySignal: number
  resumeStream: () => Promise<void>
  transport: AgentChatTransport
  transportError?: Error
  transportStatus: 'ready' | 'submitted' | 'streaming' | 'error'
  transportStreaming: boolean
  client?: AgentChatClient
}

/**
 * Owns writing-mode display attachment and cold-runtime recovery. Durable input
 * stays server-side: the browser only echoes identities from `/api/chat/active`.
 */
export function useWritingAgentRuntimeRecovery({
  activeSessionId,
  displayRehydrateRequest,
  loadHistoryAuthoritative,
  onDisplayRehydrated,
  onDisplayTerminalRestored,
  onSettled,
  runtimeRecoverySignal,
  resumeStream,
  transport,
  transportError,
  transportStatus,
  transportStreaming,
  client = writingAgentChatClient,
}: WritingAgentRuntimeRecoveryOptions) {
  const [runtimeProjection, setRuntimeProjection] = useState<ActiveChatTask | null>(null)
  const [recoveryPending, setRecoveryPending] = useState(false)
  const [recoveryAttempt, setRecoveryAttempt] = useState(0)
  const runtimeProjectionRef = useRef<ActiveChatTask | null>(null)
  const streamCycleProjectionVersionRef = useRef(0)
  const recoveryActionInFlightRef = useRef(false)
  const checkInFlightRef = useRef<{ sessionID: string; promise: Promise<void> } | null>(null)
  const attachedRecoveryRetryNeededRef = useRef(false)
  const retryNeededRef = useRef(false)
  const immediateRetryProjectionRef = useRef('')
  const displayRehydrateRef = useRef<WritingDisplayRehydrateRequest | null>(null)
  const displayRehydrateCompletedSignalRef = useRef(0)
  const displayRehydrateInFlightRef = useRef(false)
  const displayOmissionActiveRef = useRef(false)
  const wasStreamingRef = useRef(false)
  const transportResponseStreaming = transportStatus === 'streaming'
  const transportErrorRef = useRef<Error | undefined>(transportError)
  const transportStatusRef = useRef(transportStatus)
  const onSettledRef = useRef(onSettled)
  const activeSessionIdRef = useRef(activeSessionId)
  const explicitlyResumedSessionIdRef = useRef('')
  activeSessionIdRef.current = activeSessionId
  transportStatusRef.current = transportStatus
  transportErrorRef.current = transportError
  runtimeProjectionRef.current = runtimeProjection
  onSettledRef.current = onSettled
  if (displayRehydrateRequest && displayRehydrateRequest.signal > displayRehydrateCompletedSignalRef.current) {
    displayRehydrateRef.current = displayRehydrateRequest
  }

  const projectStreamCycle = useCallback((operationID: string, cycle?: number) => {
    const normalizedOperationID = operationID.trim()
    if (!normalizedOperationID) return
    streamCycleProjectionVersionRef.current += 1
    const current = runtimeProjectionRef.current
    const projection: ActiveChatTask = {
      ...(current || {}),
      active: true,
      status: 'running',
      phase: 'running',
      recovery_paused: false,
      runtime_recoverable: false,
      stream_attached: true,
      recovery_actions: [],
      active_operation_id: normalizedOperationID,
      ...(Number.isSafeInteger(cycle) && (cycle || 0) > 0 ? { active_cycle: cycle } : {}),
    }
    runtimeProjectionRef.current = projection
    setRuntimeProjection(projection)
  }, [])

  const markRecoveryRetry = useCallback((retryImmediately: boolean, projectionFingerprint?: string) => {
    retryNeededRef.current = true
    setRecoveryPending(true)
    if (!retryImmediately) return
    const fingerprint = projectionFingerprint || writingRecoveryProjectionFingerprint(runtimeProjectionRef.current)
    if (!fingerprint || immediateRetryProjectionRef.current === fingerprint) return
    immediateRetryProjectionRef.current = fingerprint
    setRecoveryAttempt((current) => current + 1)
  }, [])

  const attachDisplayStream = useCallback(
    async (failureContext: string, sessionID: string, beforeResume?: () => void): Promise<boolean> => {
      const statusAtStart = transportStatusRef.current
      const errorAtStart = transportErrorRef.current
      const projectionAtStart = writingRecoveryProjectionFingerprint(runtimeProjectionRef.current)
      try {
        // Every explicit reconnect replaces provisional text/reasoning/tool args
        // with authoritative history first. AI SDK reconnect streams append new
        // parts; without this reset, partial `hel` followed by replayed `hello`
        // renders as `helhello` and tool arguments can be duplicated as well.
        await loadHistoryAuthoritative(sessionID)
        if (activeSessionIdRef.current !== sessionID) return false
        if (displayOmissionActiveRef.current) onDisplayRehydrated()
        beforeResume?.()
        await Promise.resolve(resumeStream())
        // AI SDK resolves resumeStream on an HTTP/reconnect failure and reports
        // the failure through ChatStatus. Preserve the durable projection and
        // wait for an explicit retry opportunity instead of spinning here.
        if (transportStatusRef.current !== 'error') return true
        if (statusAtStart === 'error' && transportErrorRef.current === errorAtStart) return false
        markRecoveryRetry(true, projectionAtStart)
        return false
      } catch (error) {
        if (activeSessionIdRef.current !== sessionID) return false
        if (isAbortError(error)) return false
        console.warn(`[use-agent-runtime-recovery.ts] ${failureContext}`, error)
        markRecoveryRetry(false)
        if (isRehydrateRequired(error)) setRecoveryAttempt((current) => current + 1)
        return false
      }
    },
    [loadHistoryAuthoritative, markRecoveryRetry, onDisplayRehydrated, resumeStream],
  )

  const inspectAndAttach = useCallback(
    async (canonicalizeWhenIdle: boolean, attachStream = true, requestedSessionID = activeSessionIdRef.current) => {
      const sessionID = requestedSessionID.trim()
      while (checkInFlightRef.current?.sessionID === sessionID) await checkInFlightRef.current.promise
      if (activeSessionIdRef.current !== sessionID) return
      const inspection = (async () => {
        if (attachStream) setRecoveryPending(true)
        try {
          let projection = await client.getActiveChatTask(sessionID)
          if (activeSessionIdRef.current !== sessionID) return
          runtimeProjectionRef.current = projection
          setRuntimeProjection(projection)
          const shouldObserve = Boolean(projection.active || projection.task_id?.trim() || projection.runtime_recoverable)
          if (shouldObserve) wasStreamingRef.current = true

          let failedProjection = ''
          let recovered: Awaited<ReturnType<typeof recoverWritingProjection>>
          while (true) {
            const projectionFingerprint = writingRecoveryProjectionFingerprint(projection)
            if (!attachStream && projectionFingerprint === failedProjection) {
              attachedRecoveryRetryNeededRef.current = false
              setRecoveryPending(false)
              return
            }
            try {
              recovered = await recoverWritingProjection(projection, (action) => client.recoverChatAgentRuntime(action, sessionID))
              break
            } catch (error) {
              if (attachStream || !isRecoveryProjectionRefreshError(error)) throw error
              failedProjection = projectionFingerprint
              projection = await client.getActiveChatTask(sessionID)
              if (activeSessionIdRef.current !== sessionID) return
              runtimeProjectionRef.current = projection
              setRuntimeProjection(projection)
            }
          }
          runtimeProjectionRef.current = recovered.projection
          setRuntimeProjection(recovered.projection)
          if (recovered.taskID) {
            retryNeededRef.current = false
            if (attachStream) {
              transport.setActiveStreamTarget(recovered.taskID, undefined, { session_id: sessionID })
              void attachDisplayStream('failed to attach writing Agent stream; preserving recovery state', sessionID)
            } else {
              attachedRecoveryRetryNeededRef.current = false
              setRecoveryPending(false)
            }
            return
          }
          if (recovered.projection.runtime_recoverable) {
            retryNeededRef.current = false
            attachedRecoveryRetryNeededRef.current = false
            setRecoveryPending(attachStream)
            return
          }

          retryNeededRef.current = false
          wasStreamingRef.current = false
          if (attachStream) transport.clearActiveStreamTarget()
          onSettledRef.current()
          if (canonicalizeWhenIdle) {
            await loadHistoryAuthoritative(sessionID)
            if (activeSessionIdRef.current !== sessionID) return
            displayOmissionActiveRef.current = false
          }
          setRecoveryPending(false)
        } catch (error) {
          if (activeSessionIdRef.current !== sessionID) return
          if (!isAbortError(error)) {
            console.warn(
              '[use-agent-runtime-recovery.ts] failed to inspect or recover writing Agent runtime; waiting for a safe retry opportunity',
              error,
            )
            retryNeededRef.current = true
            attachedRecoveryRetryNeededRef.current = !attachStream
            wasStreamingRef.current = true
            setRecoveryPending(true)
          }
        }
      })()
      const trackedInspection = { sessionID, promise: inspection }
      checkInFlightRef.current = trackedInspection
      try {
        await inspection
      } finally {
        if (checkInFlightRef.current === trackedInspection) checkInFlightRef.current = null
      }
    },
    [attachDisplayStream, client, loadHistoryAuthoritative, transport],
  )

  useEffect(() => {
    const request = displayRehydrateRef.current
    if (!request || transportStreaming || displayRehydrateInFlightRef.current) return
    displayRehydrateInFlightRef.current = true
    wasStreamingRef.current = true
    retryNeededRef.current = false
    setRecoveryPending(true)
    if (!request.settled) displayOmissionActiveRef.current = true

    const finish = (completed: boolean) => {
      displayRehydrateInFlightRef.current = false
      if (!completed) {
        retryNeededRef.current = true
        setRecoveryPending(true)
        return
      }
      displayRehydrateCompletedSignalRef.current = request.signal
      if (displayRehydrateRef.current?.signal === request.signal) displayRehydrateRef.current = null
    }

    if (request.settled) {
      const sessionID = activeSessionIdRef.current.trim()
      void loadHistoryAuthoritative(sessionID)
        .then(() => {
          if (activeSessionIdRef.current !== sessionID) {
            finish(false)
            return
          }
          onDisplayTerminalRestored(request)
          transport.clearActiveStreamTarget()
          displayOmissionActiveRef.current = false
          retryNeededRef.current = false
          wasStreamingRef.current = false
          onSettledRef.current()
          setRecoveryPending(false)
          finish(true)
        })
        .catch((error) => {
          if (!isAbortError(error)) {
            console.warn('[use-agent-runtime-recovery.ts] failed to canonically rehydrate a settled Writing display', error)
            markRecoveryRetry(false)
          }
          finish(false)
        })
      return
    }

    const sessionID = activeSessionIdRef.current.trim()
    void attachDisplayStream('failed to canonically rehydrate and resume the same Writing Task', sessionID, () => {
      transport.setActiveStreamTarget(request.taskID, request.cursor, { session_id: sessionID })
    }).then(finish)
  }, [
    activeSessionId,
    attachDisplayStream,
    displayRehydrateRequest,
    loadHistoryAuthoritative,
    markRecoveryRetry,
    onDisplayRehydrated,
    onDisplayTerminalRestored,
    recoveryAttempt,
    transport,
    transportStreaming,
  ])

  useEffect(() => {
    // Omission belongs to one Writing session. A manual session switch must
    // never carry the previous run's warning into another conversation.
    displayOmissionActiveRef.current = false
    if (explicitlyResumedSessionIdRef.current === activeSessionId) {
      explicitlyResumedSessionIdRef.current = ''
      return
    }
    displayRehydrateInFlightRef.current = false
    retryNeededRef.current = false
    attachedRecoveryRetryNeededRef.current = false
    streamCycleProjectionVersionRef.current += 1
    runtimeProjectionRef.current = null
    setRuntimeProjection(null)
    setRecoveryPending(false)
    transport.clearActiveStreamTarget()
  }, [activeSessionId, transport])

  useEffect(() => {
    if (transportStatus !== 'error' || !wasStreamingRef.current) return
    if (!activeSessionIdRef.current.trim()) {
      retryNeededRef.current = false
      wasStreamingRef.current = false
      setRecoveryPending(false)
      transport.clearActiveStreamTarget()
      onSettledRef.current()
      return
    }
    if (transportError) {
      console.warn(
        '[use-agent-runtime-recovery.ts] writing Agent stream entered the AI SDK error state; waiting for a safe retry opportunity',
        transportError,
      )
    }
    // The status transition below performs the first canonical inspection.
    // Remember its projection so a late resumeStream resolution cannot queue
    // the same inspection a second time.
    immediateRetryProjectionRef.current = writingRecoveryProjectionFingerprint(runtimeProjectionRef.current)
    markRecoveryRetry(false)
  }, [markRecoveryRetry, transport, transportError, transportStatus])

  // `submitted` only means the browser started the POST. Do not treat it as
  // durable acceptance: a deterministic 4xx (for example context_changed)
  // must return to the composer instead of entering runtime recovery. Once the
  // response stream starts, the server has accepted the Run and `/active` can
  // bind operation-scoped controls to its exact identity.
  useEffect(() => {
    if (displayRehydrateRef.current) return
    if (transportStatus === 'submitted') {
      immediateRetryProjectionRef.current = ''
      retryNeededRef.current = false
      wasStreamingRef.current = false
      setRecoveryPending(false)
      return
    }
    if (transportResponseStreaming) {
      immediateRetryProjectionRef.current = ''
      const sessionID = activeSessionIdRef.current.trim()
      if (!sessionID) {
        retryNeededRef.current = false
        wasStreamingRef.current = false
        setRecoveryPending(false)
        transport.clearActiveStreamTarget()
        return
      }
      wasStreamingRef.current = true
      retryNeededRef.current = false
      setRecoveryPending(false)
      let cancelled = false
      const streamCycleProjectionVersion = streamCycleProjectionVersionRef.current
      client.getActiveChatTask(sessionID)
        .then((projection) => {
          if (cancelled || activeSessionIdRef.current !== sessionID) return
          // `agent_cycle_started` is emitted after durable Run acceptance. An
          // older `/active` request may still return the pre-registration idle
          // projection and must not revoke that newer exact Run identity.
          if (streamCycleProjectionVersionRef.current !== streamCycleProjectionVersion) return
          runtimeProjectionRef.current = projection
          setRuntimeProjection(projection)
          const taskID = projection.task_id?.trim()
          if (taskID) transport.setActiveStreamTarget(taskID, undefined, { session_id: sessionID })
        })
        .catch((error) => {
          if (!cancelled && !isAbortError(error))
            console.warn('[use-agent-runtime-recovery.ts] failed to refresh writing Agent projection', error)
        })
      return () => {
        cancelled = true
      }
    }
    if (!wasStreamingRef.current) return
    void inspectAndAttach(true)
  }, [activeSessionId, client, inspectAndAttach, recoveryAttempt, transport, transportResponseStreaming, transportStatus])

  useEffect(() => {
    if (runtimeRecoverySignal <= 0) return
    // The current AI SDK reader remains attached. Only advance the exact
    // server-projected head action; never open another display stream.
    void inspectAndAttach(false, false)
  }, [inspectAndAttach, runtimeRecoverySignal])

  useEffect(() => {
    if (!recoveryPending) return
    const retry = () => {
      if (displayRehydrateRef.current) {
        if (!displayRehydrateInFlightRef.current && !transportStreaming) {
          setRecoveryAttempt((current) => current + 1)
        }
        return
      }
      if (attachedRecoveryRetryNeededRef.current) {
        void inspectAndAttach(false, false)
        return
      }
      if (transportStreaming) return
      if (!retryNeededRef.current) return
      setRecoveryAttempt((current) => current + 1)
    }
    const retryWhenVisible = () => {
      if (document.visibilityState === 'visible') retry()
    }
    window.addEventListener('online', retry)
    window.addEventListener('focus', retry)
    document.addEventListener('visibilitychange', retryWhenVisible)
    return () => {
      window.removeEventListener('online', retry)
      window.removeEventListener('focus', retry)
      document.removeEventListener('visibilitychange', retryWhenVisible)
    }
  }, [inspectAndAttach, recoveryPending, transportStreaming])

  const resumeActiveChat = useCallback(async (sessionID = activeSessionIdRef.current) => {
    if (transportStreaming) return
    const targetSessionID = sessionID.trim()
    // Explicit callers use the Session identity just confirmed by the server.
    // Advance the async fence immediately instead of waiting for React to
    // commit the matching state render; this also invalidates late work from A.
    explicitlyResumedSessionIdRef.current = targetSessionID
    if (activeSessionIdRef.current !== targetSessionID) {
      displayOmissionActiveRef.current = false
      displayRehydrateInFlightRef.current = false
      retryNeededRef.current = false
      attachedRecoveryRetryNeededRef.current = false
      runtimeProjectionRef.current = null
      setRuntimeProjection(null)
      setRecoveryPending(false)
      transport.clearActiveStreamTarget()
      activeSessionIdRef.current = targetSessionID
    }
    await inspectAndAttach(false, true, targetSessionID)
  }, [inspectAndAttach, transport, transportStreaming])

  const abortRecovery = useCallback(async () => {
    const action = runtimeProjection?.recovery_actions?.find((candidate) => candidate.kind === 'abort')
    if (!action) return false
    if (recoveryActionInFlightRef.current) return true
    recoveryActionInFlightRef.current = true
    try {
      const sessionID = activeSessionIdRef.current.trim()
      const receipt = await client.recoverChatAgentRuntime(action, sessionID)
      if (activeSessionIdRef.current !== sessionID) return false
      if (!sameRecoveryAction(receipt.recovery_action, action)) {
        throw new Error('The writing Agent recovery action changed while aborting')
      }
      const taskID = receipt.task_id?.trim()
      if (!taskID) throw new Error('Recovered writing Agent abort did not return a display task')
      retryNeededRef.current = false
      wasStreamingRef.current = true
      if (!transportStreaming) setRecoveryPending(true)
      setRuntimeProjection((current) =>
        current
          ? {
              ...current,
              active: true,
              status: 'running',
              task_id: taskID,
              recovery_paused: false,
              runtime_recoverable: false,
              stream_attached: true,
              recovery_actions: [],
            }
          : current,
      )
      // A submitted/streaming AI SDK request already owns the display stream;
      // the typed abort receipt will arrive on that stream. Opening a second
      // resume subscription here races the current reader and duplicates UI.
      if (!transportStreaming) {
        transport.setActiveStreamTarget(taskID, undefined, { session_id: sessionID })
        void attachDisplayStream('failed to attach recovered writing Agent abort stream', sessionID)
      }
      return true
    } finally {
      recoveryActionInFlightRef.current = false
    }
  }, [attachDisplayStream, client, runtimeProjection, transport, transportStreaming])

  return {
    abortRecovery,
    projectStreamCycle,
    recoveryPending,
    resumeActiveChat,
    runtimeProjection,
    setRuntimeProjection,
  }
}

async function recoverWritingProjection(
  initial: ActiveChatTask,
  recover: (action: AgentRuntimeRecoveryAction) => Promise<AgentRuntimeRecoveryReceipt>,
): Promise<{
  projection: ActiveChatTask
  taskID: string
}> {
  let taskID = initial.task_id?.trim() || ''
  const actions = recoveryActionsToSubmit(initial)
  for (const action of actions) {
    const receipt = await recover(action)
    if (!sameRecoveryAction(receipt.recovery_action, action)) {
      throw new Error('The writing Agent recovery action changed while it was being resumed')
    }
    taskID = receipt.task_id?.trim() || taskID
  }
  if (!actions.length) {
    // The active endpoint retains the last finished display Task for replay,
    // including `task_id` and `stream_attached`. It is observable only while
    // the binding itself remains active or has an explicit recovery boundary.
    const existingDisplayTask = initial.active || (initial.stream_attached && initial.runtime_recoverable)
    return { projection: initial, taskID: existingDisplayTask ? taskID : '' }
  }
  if (!taskID) throw new Error('Recovered writing Agent runtime did not return a display task')
  const executionResumed = actions.some((action) => action.kind !== 'start_turn')
  const abortActions = (initial.recovery_actions || []).filter((action) => action.kind === 'abort')
  return {
    taskID,
    projection: {
      ...initial,
      active: true,
      status: 'running',
      task_id: taskID,
      // start_turn only restores the display stream. The durable operation is
      // still paused until a queued/structural action resumes it or the user
      // submits the server-projected abort action.
      recovery_paused: !executionResumed,
      runtime_recoverable: !executionResumed,
      stream_attached: true,
      recovery_actions: executionResumed ? [] : abortActions,
    },
  }
}

function recoveryActionsToSubmit(projection: ActiveChatTask) {
  if (!projection.runtime_recoverable) return []
  if (projection.stream_attached && !projection.recovery_paused) return []
  const projected = projection.recovery_actions || []
  // An attached Task is already the display owner for this operation. Reusing
  // its task_id is sufficient; POSTing start_turn again would conflict with
  // that owner. A head-of-line state action may still need to run after the
  // attach succeeded but before the previous browser observed its receipt.
  const attach = projection.stream_attached ? undefined : projected.find((action) => action.kind === 'start_turn')
  const stateChange = projected.find((action) => action.kind !== 'start_turn' && action.kind !== 'abort')
  return [attach, stateChange].filter((action): action is AgentRuntimeRecoveryAction => Boolean(action))
}

function sameRecoveryAction(left: AgentRuntimeRecoveryAction, right: AgentRuntimeRecoveryAction) {
  return left.action_id === right.action_id && left.kind === right.kind && left.command_id === right.command_id && left.operation_id === right.operation_id
}

function writingRecoveryProjectionFingerprint(projection: ActiveChatTask | null) {
  if (!projection) return ''
  return JSON.stringify([
    projection.task_id || '',
    projection.active_operation_id || '',
    projection.cursor || 0,
    projection.recovery_paused === true,
    projection.pending_interruption_id || '',
    projection.recovery_actions || [],
  ])
}

function isRehydrateRequired(error: unknown) {
  return Boolean(error && typeof error === 'object' && (error as { code?: unknown }).code === 'agent_runtime.rehydrate_required')
}

function isRecoveryProjectionRefreshError(error: unknown) {
  if (!error || typeof error !== 'object') return false
  const code = (error as { code?: unknown }).code
  return (
    code === 'agent_runtime.rehydrate_required' || code === 'agent_runtime.recovery_changed' || code === 'agent_runtime.stream_attached'
  )
}
