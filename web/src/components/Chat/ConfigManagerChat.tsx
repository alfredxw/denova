import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { InputArea } from './InputArea'
import { MessageList } from './MessageList'
import { clearConfigManagerSession, getConfigManagerMessages, runConfigManagerStream } from '@/lib/api'
import type { ChatMessage, ConfigManagerRunRequest, SSEEvent } from '@/lib/api'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { appendConfigManagerMessage, parseConfigManagerPayload, reduceConfigManagerMessages, type ConfigManagerToolPayload } from './config-manager-events'

interface ConfigManagerChatProps {
  workspace?: string
  origin: string
  resourceId?: string
  storyId?: string
  branchId?: string
  context?: Record<string, string>
  onMutated?: () => void
  className?: string
}

export function ConfigManagerChat({ workspace = '', origin, resourceId, storyId, branchId, context, onMutated, className = '' }: ConfigManagerChatProps) {
  const { t } = useTranslation()
  const activeKeyRef = useRef('')
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [inputAreaHeight, setInputAreaHeight] = useState(0)
  const skills = useSkillCommands({ agentKey: 'config_manager', workspace, fallbackEnabled: true })
  const scope = useMemo(() => ({
    origin,
    resource_id: resourceId,
    story_id: storyId,
    branch_id: branchId,
  }), [branchId, origin, resourceId, storyId])
  const chatKey = useMemo(() => [
    'config-manager',
    workspace,
    origin,
    resourceId || '',
    storyId || '',
    branchId || '',
  ].join(':'), [branchId, origin, resourceId, storyId, workspace])
  const tokenUsageMessages = useMemo(
    () => messages.filter((message) => message.role === 'token_usage'),
    [messages],
  )
  const messageListBottomPadding = inputAreaHeight > 0 ? inputAreaHeight + 20 : undefined

  const loadMessages = useCallback(() => {
    if (!workspace) {
      setMessages([])
      return
    }
    getConfigManagerMessages(scope)
      .then(setMessages)
      .catch((err) => setError(err instanceof Error ? err.message : t('configManager.historyLoadFailed')))
  }, [scope, t, workspace])

  useEffect(() => {
    activeKeyRef.current = chatKey
    setRunning(false)
    setError(null)
    loadMessages()
  }, [chatKey, loadMessages])

  const appendMessage = (message: ChatMessage) => {
    setMessages((current) => appendConfigManagerMessage(current, message, 'config-manager'))
  }

  const handleEvent = (event: SSEEvent) => {
    setMessages((current) => reduceConfigManagerMessages(current, event, {
      idPrefix: 'config-manager',
      toolLabel: t('configManager.tool'),
      failureMessage: t('configManager.runFailed'),
    }))
    if (event.event === 'tool_result' && parseConfigManagerPayload<ConfigManagerToolPayload>(event.data)) onMutated?.()
  }

  const send = async (message: string) => {
    const instruction = message.trim()
    if (!instruction || running) return
    if (instruction === '/clear') {
      setRunning(true)
      try {
        await clearConfigManagerSession(scope)
        setMessages([{ id: `clear-${Date.now()}`, type: 'clear', created_at: new Date().toISOString() }])
      } catch (err) {
        appendMessage({ role: 'error', content: err instanceof Error ? err.message : t('configManager.clearFailed') })
      } finally {
        setRunning(false)
      }
      return
    }
    appendMessage({ role: 'user', content: instruction })
    setRunning(true)
    setError(null)
    const activeChatKey = chatKey
    try {
      const req: ConfigManagerRunRequest = {
        instruction,
        ...scope,
        context,
      }
      const stream = await runConfigManagerStream(req)
      const reader = stream.getReader()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        if (activeKeyRef.current !== activeChatKey) break
        handleEvent(value)
      }
    } catch (err) {
      if (activeKeyRef.current === activeChatKey) appendMessage({ role: 'error', content: err instanceof Error ? err.message : t('configManager.runFailed') })
    } finally {
      if (activeKeyRef.current === activeChatKey) setRunning(false)
    }
  }

  return (
    <div className={`relative flex h-full min-h-0 flex-col overflow-hidden ${className}`}>
      {error && <div className="border-b border-[var(--nova-border)] px-3 py-2 text-xs text-red-400">{error}</div>}
      <MessageList
        messages={messages}
        isStreaming={running}
        activityContent=""
        scrollResetKey={chatKey}
        bottomPaddingClassName="pb-36"
        bottomPaddingPx={messageListBottomPadding}
      />
      <InputArea
        onSend={(value) => void send(value)}
        disabled={running}
        draftKey={chatKey}
        skills={skills}
        commandScope="all"
        builtinCommands={['/clear']}
        placeholder={t('configManager.placeholder')}
        disabledPlaceholder={t('configManager.executing')}
        tokenUsageMessages={tokenUsageMessages}
        agentKey="config_manager"
        workspace={workspace}
        floating
        onHeightChange={setInputAreaHeight}
      />
    </div>
  )
}
