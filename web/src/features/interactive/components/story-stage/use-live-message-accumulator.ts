import { useCallback, useEffect, useMemo, useRef } from 'react'
import { buildContextCompactionMessage, createContextCompactionMessageId, upsertContextCompactionMessage } from '@/components/Chat/context-compaction-message'
import type { ChatMessage } from '@/lib/api'
import { createRafUpdateBatcher, STREAMING_RENDER_INTERVAL_MS } from '@/lib/streaming/raf-update-batcher'
import {
  appendBufferedLiveMessage,
  bindLiveToolEventKeys,
  findMappedLiveToolId,
  findToolMessageIndexForPayload,
  liveToolEventKeys,
  promoteMessageTarget,
  promoteMessageTargets,
  streamMetadataFromPayload,
  type BufferedLiveMessage,
} from './live-stream-messages'
import { publicRuleRollFromToolOutput } from './rule-roll'
import { createLiveTurnRenderKeys, type LiveTurnRenderKeys } from './utils'

type LiveMessageUpdater = (updater: ChatMessage[] | ((current: ChatMessage[]) => ChatMessage[])) => void

interface UseLiveMessageAccumulatorOptions {
  publicRuleRollVisible: boolean
  setMessages: LiveMessageUpdater
}

// Owns the animation-frame buffer and correlation state for one live story stream.
// Persisted history projection only receives stable render-key lookup methods, so
// display recovery state cannot leak into the model/session context boundary.
export function useLiveMessageAccumulator({ publicRuleRollVisible, setMessages }: UseLiveMessageAccumulatorOptions) {
  const toolKeyToMessageIdRef = useRef<Record<string, string>>({})
  const nonNarrativeStreamingRef = useRef(false)
  const stageKeyRef = useRef('')
  const currentTurnRenderKeysRef = useRef<LiveTurnRenderKeys | null>(null)
  const turnRenderKeysRef = useRef<Record<string, LiveTurnRenderKeys>>({})
  const currentCompactionMessageIdRef = useRef<string | null>(null)
  const compactionIdCounterRef = useRef(0)

  // Text, thinking and tool-argument deltas share one ordered commit lane. The
  // staged target is promoted once per batch so the presentation layer can
  // identify prose immediately without adding another render per provider delta.
  const updateBatcher = useMemo(
    () => createRafUpdateBatcher<ChatMessage[]>(
      (update) => setMessages((current) => promoteMessageTargets(update(current))),
      { minIntervalMs: STREAMING_RENDER_INTERVAL_MS },
    ),
    [setMessages],
  )

  const flushMessageBuffer = useCallback(() => {
    updateBatcher.flush()
  }, [updateBatcher])

  const flush = useCallback(() => {
    flushMessageBuffer()
  }, [flushMessageBuffer])

  const queue = useCallback((message: BufferedLiveMessage) => {
    updateBatcher.enqueue((current) => appendBufferedLiveMessage(current, message))
  }, [updateBatcher])

  const appendAssistant = useCallback((content: string, navigationAnchorId: string, metadata: Partial<ChatMessage> = {}) => {
    if (!content) return
    const renderKey = metadata.render_key || (metadata.subagent ? undefined : currentTurnRenderKeysRef.current?.assistant)
    const navigationMetadata = metadata.subagent ? metadata : { ...metadata, navigation_turn_id: navigationAnchorId }
    queue({
      role: 'assistant',
      content,
      metadata: renderKey ? { ...navigationMetadata, render_key: renderKey } : navigationMetadata,
    })
  }, [queue])

  const resetAssistant = useCallback(() => {
    flush()
    setMessages((current) => {
      const last = current[current.length - 1]
      return last?.role === 'assistant' && last.streaming ? current.slice(0, -1) : current
    })
  }, [flush, setMessages])

  const appendThinking = useCallback((content: string, metadata: Partial<ChatMessage> = {}) => {
    if (!content) return
    nonNarrativeStreamingRef.current = true
    queue({ role: 'thinking', content, metadata })
  }, [queue])

  const appendToolCall = useCallback((payload: Record<string, unknown> & { id?: string; name?: string; args?: string }) => {
    flush()
    const toolKeys = liveToolEventKeys(payload)
    const mappedId = findMappedLiveToolId(toolKeys, toolKeyToMessageIdRef.current)
    const id = payload.id || mappedId || `tool-${Date.now()}-${Math.random().toString(16).slice(2)}`
    const name = payload.name || 'unknown_tool'
    if (toolKeys.length > 0) {
      toolKeyToMessageIdRef.current = bindLiveToolEventKeys(toolKeys, toolKeyToMessageIdRef.current, id)
    }
    nonNarrativeStreamingRef.current = true
    setMessages((current) => [
      ...current,
      {
        id,
        role: 'tool_call',
        content: name,
        name,
        args: payload.args || '',
        status: 'running',
        streaming: true,
        ...streamMetadataFromPayload(payload),
      },
    ])
  }, [flush, setMessages])

  const appendToolArgs = useCallback((payload: Record<string, unknown> & { id?: string; name?: string; args?: string; delta?: string }) => {
    if (!payload.id && !payload.name && liveToolEventKeys(payload).length === 0) return
    updateBatcher.enqueue((current) => {
      const targetIndex = findToolMessageIndexForPayload(current, payload, toolKeyToMessageIdRef.current)
      if (targetIndex < 0) return current
      const matchedId = current[targetIndex].id
      if (matchedId) {
        toolKeyToMessageIdRef.current = bindLiveToolEventKeys(liveToolEventKeys(payload), toolKeyToMessageIdRef.current, matchedId)
      }
      return current.map((message, index) => index === targetIndex
        ? { ...message, args: payload.args !== undefined ? payload.args : `${message.args || ''}${payload.delta || ''}` }
        : message)
    })
  }, [updateBatcher])

  const completeToolCall = useCallback((payload: Record<string, unknown> & { id?: string; name?: string }, result = '') => {
    updateBatcher.flush()
    setMessages((current) => {
      const targetIndex = findToolMessageIndexForPayload(current, payload, toolKeyToMessageIdRef.current)
      if (targetIndex < 0) return current
      const matchedId = current[targetIndex].id
      if (matchedId) {
        toolKeyToMessageIdRef.current = bindLiveToolEventKeys(liveToolEventKeys(payload), toolKeyToMessageIdRef.current, matchedId)
      }
      return current.map((message, index) => index === targetIndex
        ? { ...message, status: 'success' as const, result, streaming: false }
        : message)
    })
  }, [setMessages, updateBatcher])

  const appendRuleRoll = useCallback((payload: Record<string, unknown> & { name?: string; content?: string }) => {
    if (!publicRuleRollVisible || payload.name !== 'prepare_interactive_turn') return
    flush()
    const ruleRoll = publicRuleRollFromToolOutput(payload.content || '')
    if (!ruleRoll) return
    setMessages((current) => {
      const id = ruleRoll.resolution_id ? `live-rule-roll-${ruleRoll.resolution_id}` : `live-rule-roll-${Date.now()}`
      if (current.some((message) => message.role === 'rule_roll' && message.id === id)) return current
      return [...current, { id, role: 'rule_roll', rule_roll: ruleRoll, streaming: false }]
    })
  }, [flush, publicRuleRollVisible, setMessages])

  const appendContextCompaction = useCallback((data: Record<string, unknown>) => {
    flush()
    const id = currentCompactionMessageIdRef.current || createContextCompactionMessageId(compactionIdCounterRef)
    currentCompactionMessageIdRef.current = id
    nonNarrativeStreamingRef.current = true
    setMessages((current) => upsertContextCompactionMessage(current, buildContextCompactionMessage(data, id)))
  }, [flush, setMessages])

  const collapseNonNarrative = useCallback(() => {
    if (!nonNarrativeStreamingRef.current) return
    nonNarrativeStreamingRef.current = false
    updateBatcher.enqueue((current) => current.map((message) => {
      if (message.role === 'thinking') {
        return { ...promoteMessageTarget(message), streaming: false }
      }
      if (message.role === 'tool_call' || message.role === 'context_compaction') {
        return {
          ...message,
          streaming: false,
          status: message.status === 'running' ? 'success' : message.status,
        }
      }
      return message
    }))
  }, [updateBatcher])

  const finishMessages = useCallback(() => {
    flush()
    nonNarrativeStreamingRef.current = false
    setMessages((current) => current.map((message) =>
      message.role === 'assistant' || message.role === 'thinking' || message.role === 'tool_call' || message.role === 'context_compaction'
        ? {
            ...promoteMessageTarget(message),
            streaming: false,
            status: message.role === 'tool_call' || message.role === 'context_compaction'
              ? (message.status === 'running' ? 'success' : message.status)
              : message.status,
          }
        : message))
  }, [flush, setMessages])

  const resetForCheckpoint = useCallback(() => {
    updateBatcher.discard()
    toolKeyToMessageIdRef.current = {}
    nonNarrativeStreamingRef.current = false
    currentTurnRenderKeysRef.current = null
    currentCompactionMessageIdRef.current = null
    setMessages([])
  }, [setMessages, updateBatcher])

  const prepareTurn = useCallback((message: string, navigationAnchorId: string, mode: 'replace' | 'append') => {
    flush()
    toolKeyToMessageIdRef.current = {}
    nonNarrativeStreamingRef.current = false
    const renderKeys = createLiveTurnRenderKeys()
    currentTurnRenderKeysRef.current = renderKeys
    currentCompactionMessageIdRef.current = null
    const userMessage: ChatMessage = {
      role: 'user',
      content: message,
      render_key: renderKeys.user,
      navigation_turn_id: navigationAnchorId,
    }
    setMessages((current) => mode === 'replace' ? [userMessage] : [...current, userMessage])
  }, [flush, setMessages])

  const bindPersistedTurn = useCallback((turnId: string) => {
    if (turnId && currentTurnRenderKeysRef.current) {
      turnRenderKeysRef.current[turnId] = currentTurnRenderKeysRef.current
    }
  }, [])

  const renderKeyFor = useCallback((turnId: string, role: 'user' | 'assistant') => {
    return turnRenderKeysRef.current[turnId]?.[role]
  }, [])

  const beginStage = useCallback((stageKey: string) => {
    stageKeyRef.current = stageKey
  }, [])
  const belongsToStage = useCallback((stageKey: string) => stageKeyRef.current === stageKey, [])
  const resetCompaction = useCallback(() => {
    currentCompactionMessageIdRef.current = null
  }, [])
  const endRun = useCallback(() => {
    toolKeyToMessageIdRef.current = {}
    currentCompactionMessageIdRef.current = null
    currentTurnRenderKeysRef.current = null
  }, [])

  useEffect(() => () => {
    updateBatcher.discard()
  }, [updateBatcher])

  return {
    appendAssistant,
    appendContextCompaction,
    appendRuleRoll,
    appendThinking,
    appendToolArgs,
    appendToolCall,
    beginStage,
    bindPersistedTurn,
    belongsToStage,
    collapseNonNarrative,
    completeToolCall,
    endRun,
    finishMessages,
    flush,
    prepareTurn,
    renderKeyFor,
    resetAssistant,
    resetForCheckpoint,
    resetCompaction,
  }
}

export type LiveMessageAccumulator = ReturnType<typeof useLiveMessageAccumulator>
