import { useCallback, useRef, useState } from 'react'
import {
  abortAutomationRun,
  getAutomationRunMessages,
  streamAutomationRun,
  streamAutomationRunByID,
  streamAutomationRunMessage,
  type AutomationRunRecord,
  type AutomationTriggerEvidence,
  createAgentCommandID,
} from '@/lib/api'
import type { AgentMessageView } from '@/lib/agent-message-view'
import { createAgentTextMessage, useAgentUIMessageStream } from '@/hooks/useAgentUIMessageStream'
import { agentCommandRetryKey, isKnownAgentCommandOutcome, rememberAgentCommandID } from '@/lib/agent-command'

export function useAutomationRunStream(options: { onFinished?: () => void | Promise<void> } = {}) {
  const { onFinished } = options
  const [activeRun, setActiveRun] = useState<AutomationRunRecord | null>(null)
  const retryCommandIDsRef = useRef(new Map<string, string>())

  const handleStreamView = useCallback((view: AgentMessageView) => {
    if (view.kind !== 'activity' || view.data.event !== 'automation_run') return
    setActiveRun(view.data as unknown as AutomationRunRecord)
  }, [])

  const {
    messages,
    setMessages,
    isStreaming,
    activityContent,
    consumeAgentUIStream,
    resetStreamingState,
    setAbortController,
  } = useAgentUIMessageStream({ onView: handleStreamView })

  const reset = useCallback(() => {
    resetStreamingState()
    setMessages([])
    setActiveRun(null)
  }, [resetStreamingState, setMessages])

  const consumeRunStream = useCallback(async (stream: Awaited<ReturnType<typeof streamAutomationRun>>) => {
    await consumeAgentUIStream(stream)
    await onFinished?.()
  }, [consumeAgentUIStream, onFinished])

  const start = useCallback(async (taskId: string, userMessage: string, triggerEvidence: AutomationTriggerEvidence[] = []) => {
    reset()
    setMessages(userMessage ? [createAgentTextMessage('user', userMessage)] : [])
    const abortController = new AbortController()
    setAbortController(abortController)
    const retryKey = agentCommandRetryKey(taskId, 'automation_run', triggerEvidence)
    const commandId = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
    try {
      const stream = await streamAutomationRun(taskId, commandId, abortController.signal, triggerEvidence)
      retryCommandIDsRef.current.delete(retryKey)
      await consumeRunStream(stream)
    } catch (error) {
      if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
      throw error
    }
  }, [consumeRunStream, reset, setAbortController, setMessages])

  const resume = useCallback(async (run: AutomationRunRecord, intro?: string) => {
    reset()
    setActiveRun(run)
    setMessages(intro ? [createAgentTextMessage('system', intro)] : [])
    const abortController = new AbortController()
    setAbortController(abortController)
    const stream = await streamAutomationRunByID(run.id, abortController.signal)
    await consumeRunStream(stream)
  }, [consumeRunStream, reset, setAbortController, setMessages])

  const loadHistory = useCallback(async (run: AutomationRunRecord) => {
    reset()
    setActiveRun(run)
    const history = await getAutomationRunMessages(run.id)
    setMessages(history)
  }, [reset, setMessages])

  const send = useCallback(async (message: string) => {
    const trimmed = message.trim()
    const runId = activeRun?.id
    if (!trimmed || !runId || isStreaming) return
    setMessages(prev => [...prev, createAgentTextMessage('user', trimmed)])
    const abortController = new AbortController()
    setAbortController(abortController)
    const retryKey = agentCommandRetryKey(runId, 'automation_follow_up', trimmed)
    const commandId = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
    try {
      const stream = await streamAutomationRunMessage(runId, commandId, trimmed, abortController.signal)
      retryCommandIDsRef.current.delete(retryKey)
      await consumeRunStream(stream)
    } catch (error) {
      if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
      throw error
    }
  }, [activeRun?.id, consumeRunStream, isStreaming, setAbortController, setMessages])

  const stop = useCallback(() => {
    const runId = activeRun?.id
    const operationId = activeRun?.runtime_operation_id?.trim()
    if (!runId || !operationId) return
    const retryKey = agentCommandRetryKey(operationId, 'abort', { reason: 'user_requested' })
    const commandId = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
    void abortAutomationRun(runId, commandId, operationId).then(() => {
      retryCommandIDsRef.current.delete(retryKey)
    }).catch((error) => {
      if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
    })
  }, [activeRun?.id, activeRun?.runtime_operation_id])

  return {
    messages,
    isStreaming,
    activityContent,
    activeRun,
    start,
    resume,
    loadHistory,
    send,
    stop,
    reset,
  }
}
