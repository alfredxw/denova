import { useCallback, useRef, useState } from 'react'
import type { SessionSummary } from '@/lib/api'
import { normalizeAgentUIMessages, type AgentUIMessage } from '@/lib/agent-ui'
import { filterInternalPlanUIMessages } from './agent-chat-state'
import { writingAgentChatClient, type AgentChatClient } from './agent-chat-client'

interface WritingAgentHistoryOptions {
  setMessages: (messages: AgentUIMessage[] | ((messages: AgentUIMessage[]) => AgentUIMessage[])) => void
  client?: AgentChatClient
}

/**
 * Owns writing-session history loading and its request ordering guarantees.
 * Authoritative reloads replace provisional stream state; pagination only
 * prepends a page when no newer authoritative reload has superseded it.
 */
export function useWritingAgentHistory({ setMessages, client = writingAgentChatClient }: WritingAgentHistoryOptions) {
  const fixedSessionId = client.fixedSessionId || ''
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [activeSessionId, setActiveSessionId] = useState(fixedSessionId)
  const [hasEarlierMessages, setHasEarlierMessages] = useState(false)
  const [isLoadingEarlierHistory, setIsLoadingEarlierHistory] = useState(false)
  const historyRequestGenerationRef = useRef(0)
  const sessionsRequestGenerationRef = useRef(0)
  const earlierHistoryRequestRef = useRef(0)
  const earlierHistoryLoadingRef = useRef(false)
  const historyPageRef = useRef<{
    sessionId?: string
    nextBefore: string
    hasMore: boolean
  }>({
    nextBefore: '0',
    hasMore: false,
  })

  const loadSessions = useCallback(async () => {
    const generation = sessionsRequestGenerationRef.current + 1
    sessionsRequestGenerationRef.current = generation
    try {
      const list = await client.getSessions()
      if (generation !== sessionsRequestGenerationRef.current) return list
      setSessions(list)
      setActiveSessionId(fixedSessionId || list.find((item) => item.active)?.id || list[0]?.id || '')
      return list
    } catch (error) {
      console.error('加载会话列表失败', error)
      return []
    }
  }, [client, fixedSessionId])

  const loadHistoryAuthoritative = useCallback(
    async (sessionId?: string) => {
      const generation = historyRequestGenerationRef.current + 1
      historyRequestGenerationRef.current = generation
      earlierHistoryRequestRef.current += 1
      earlierHistoryLoadingRef.current = false
      setIsLoadingEarlierHistory(false)

      const targetSessionId = fixedSessionId || sessionId
      const page = await client.getMessagesPage(targetSessionId || undefined)
      if (generation !== historyRequestGenerationRef.current) {
        throw new Error('Writing history reload was superseded before it could become authoritative')
      }

      historyPageRef.current = {
        sessionId: targetSessionId || undefined,
        nextBefore: page.nextBefore,
        hasMore: page.hasMore,
      }
      setHasEarlierMessages(page.hasMore)
      setMessages(filterInternalPlanUIMessages(page.messages))
    },
    [client, fixedSessionId, setMessages],
  )

  const loadHistory = useCallback(
    async (sessionId?: string) => {
      try {
        await loadHistoryAuthoritative(sessionId)
      } catch (error) {
        console.error('加载历史失败', error)
      }
    },
    [loadHistoryAuthoritative],
  )

  const loadEarlierHistory = useCallback(async () => {
    const currentPage = historyPageRef.current
    if (!currentPage.hasMore || earlierHistoryLoadingRef.current) return

    const historyGeneration = historyRequestGenerationRef.current
    const requestID = earlierHistoryRequestRef.current + 1
    earlierHistoryRequestRef.current = requestID
    earlierHistoryLoadingRef.current = true
    setIsLoadingEarlierHistory(true)

    try {
      const page = await client.getMessagesPage(currentPage.sessionId, {
        before: currentPage.nextBefore,
      })
      if (historyGeneration !== historyRequestGenerationRef.current || requestID !== earlierHistoryRequestRef.current) return

      const earlierMessages = filterInternalPlanUIMessages(page.messages)
      historyPageRef.current = {
        ...currentPage,
        nextBefore: page.nextBefore,
        hasMore: page.hasMore,
      }
      setHasEarlierMessages(page.hasMore)
      setMessages((messages) => normalizeAgentUIMessages([...earlierMessages, ...messages]))
    } catch (error) {
      if (historyGeneration === historyRequestGenerationRef.current && requestID === earlierHistoryRequestRef.current) {
        console.error('加载更早历史失败', error)
      }
    } finally {
      if (requestID === earlierHistoryRequestRef.current) {
        earlierHistoryLoadingRef.current = false
        setIsLoadingEarlierHistory(false)
      }
    }
  }, [client, setMessages])

  return {
    activeSessionId,
    hasEarlierMessages,
    isLoadingEarlierHistory,
    loadEarlierHistory,
    loadHistory,
    loadHistoryAuthoritative,
    loadSessions,
    sessions,
    setActiveSessionId,
  }
}
