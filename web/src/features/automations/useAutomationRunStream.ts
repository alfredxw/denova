import { useCallback, useState } from 'react'
import {
  abortAutomationRun,
  getAutomationRunMessages,
  streamAutomationRun,
  streamAutomationRunByID,
  streamAutomationRunMessage,
  type AutomationRunRecord,
  type SSEEvent,
} from '@/lib/api'
import { normalizeRepeatedMessages, useAgentEventStream } from '@/hooks/useAgentEventStream'

export function useAutomationRunStream(options: { onFinished?: () => void | Promise<void> } = {}) {
  const { onFinished } = options
  const [activeRun, setActiveRun] = useState<AutomationRunRecord | null>(null)

  const handleStreamEvent = useCallback((event: SSEEvent, data: Record<string, unknown>) => {
    if (event.event === 'automation_run') {
      setActiveRun(data as unknown as AutomationRunRecord)
    }
  }, [])

  const {
    messages,
    setMessages,
    isStreaming,
    activityContent,
    consumeAgentStream,
    resetStreamingState,
    setAbortController,
  } = useAgentEventStream({ onEvent: handleStreamEvent })

  const reset = useCallback(() => {
    resetStreamingState()
    setMessages([])
    setActiveRun(null)
  }, [resetStreamingState, setMessages])

  const consumeRunStream = useCallback(async (stream: ReadableStream<SSEEvent>) => {
    await consumeAgentStream(stream)
    await onFinished?.()
  }, [consumeAgentStream, onFinished])

  const start = useCallback(async (taskId: string, userMessage: string) => {
    reset()
    setMessages(userMessage ? [{ role: 'user', content: userMessage }] : [])
    const abortController = new AbortController()
    setAbortController(abortController)
    const stream = await streamAutomationRun(taskId, abortController.signal)
    await consumeRunStream(stream)
  }, [consumeRunStream, reset, setAbortController, setMessages])

  const resume = useCallback(async (run: AutomationRunRecord, intro?: string) => {
    reset()
    setActiveRun(run)
    setMessages(intro ? [{ role: 'system', content: intro }] : [])
    const abortController = new AbortController()
    setAbortController(abortController)
    const stream = await streamAutomationRunByID(run.id, abortController.signal)
    await consumeRunStream(stream)
  }, [consumeRunStream, reset, setAbortController, setMessages])

  const loadHistory = useCallback(async (run: AutomationRunRecord) => {
    reset()
    setActiveRun(run)
    const history = await getAutomationRunMessages(run.id)
    setMessages(normalizeRepeatedMessages(history))
  }, [reset, setMessages])

  const send = useCallback(async (message: string) => {
    const trimmed = message.trim()
    const runId = activeRun?.id
    if (!trimmed || !runId || isStreaming) return
    setMessages(prev => [...prev, { role: 'user', content: trimmed }])
    const abortController = new AbortController()
    setAbortController(abortController)
    const stream = await streamAutomationRunMessage(runId, trimmed, abortController.signal)
    await consumeRunStream(stream)
  }, [activeRun?.id, consumeRunStream, isStreaming, setAbortController, setMessages])

  const stop = useCallback(() => {
    const runId = activeRun?.id
    if (runId) void abortAutomationRun(runId)
  }, [activeRun?.id])

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
