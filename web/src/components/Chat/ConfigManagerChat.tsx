import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { InputArea } from './InputArea'
import { MessageList } from './MessageList'
import { clearConfigManagerSession, getConfigManagerMessages, runConfigManagerStream } from '@/lib/api'
import type { ChatMessage, ConfigManagerRunRequest, SSEEvent } from '@/lib/api'
import { useSkillCommands } from '@/hooks/useSkillCommands'

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
    setMessages((current) => [...current, { ...message, id: message.id || `${Date.now()}-${current.length}` }])
  }

  const appendStreaming = (role: ChatMessage['role'], content: string) => {
    if (!content) return
    setMessages((current) => {
      const last = current[current.length - 1]
      if (last?.role === role && last.status !== 'success') {
        return [...current.slice(0, -1), { ...last, content: `${last.content || ''}${content}` }]
      }
      return [...current, { id: `${Date.now()}-${current.length}`, role, content }]
    })
  }

  const upsertToolCall = (payload: ToolPayload) => {
    const id = payload.id || `tool-${Date.now()}`
    const name = payload.name || t('configManager.tool')
    setMessages((current) => {
      const existing = current.findIndex((message) => message.id === id)
      const next: ChatMessage = { id, role: 'tool_call', content: name, name, args: payload.args || '', status: 'running' }
      if (existing >= 0) return current.map((message, index) => index === existing ? { ...message, ...next, args: message.args || next.args } : message)
      return [...current, next]
    })
  }

  const appendToolArgs = (payload: ToolPayload) => {
    if (!payload.id || !payload.delta) return
    setMessages((current) => current.map((message) => (
      message.id === payload.id && message.role === 'tool_call'
        ? { ...message, args: `${message.args || ''}${payload.delta}` }
        : message
    )))
  }

  const finishToolCall = (payload: ToolPayload) => {
    if (!payload.id) return
    setMessages((current) => current.map((message) => (
      message.id === payload.id && message.role === 'tool_call'
        ? { ...message, status: 'success', result: payload.content || '' }
        : message
    )))
    onMutated?.()
  }

  const handleEvent = (event: SSEEvent) => {
    if (event.event === 'thinking') {
      appendStreaming('thinking', parsePayload<{ content?: string }>(event.data)?.content || '')
      return
    }
    if (event.event === 'chunk') {
      appendStreaming('assistant', parsePayload<{ content?: string }>(event.data)?.content || '')
      return
    }
    if (event.event === 'tool_call') {
      const payload = parsePayload<ToolPayload>(event.data)
      if (payload) upsertToolCall(payload)
      return
    }
    if (event.event === 'tool_args_delta') {
      const payload = parsePayload<ToolPayload>(event.data)
      if (payload) appendToolArgs(payload)
      return
    }
    if (event.event === 'tool_result') {
      const payload = parsePayload<ToolPayload>(event.data)
      if (payload) finishToolCall(payload)
      return
    }
    if (event.event === 'error') {
      appendMessage({ role: 'error', content: parsePayload<{ message?: string }>(event.data)?.message || t('configManager.runFailed') })
    }
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
    <div className={`flex h-full min-h-0 flex-col ${className}`}>
      {error && <div className="border-b border-[var(--nova-border)] px-3 py-2 text-xs text-red-400">{error}</div>}
      <MessageList
        messages={messages}
        isStreaming={running}
        activityContent=""
        scrollResetKey={chatKey}
        bottomPaddingClassName="pb-4"
      />
      <div className="border-t border-[var(--nova-border)] p-3">
        <InputArea
          onSend={(value) => void send(value)}
          disabled={running}
          draftKey={chatKey}
          skills={skills}
          commandScope="skills"
          placeholder={t('configManager.placeholder')}
          disabledPlaceholder={t('configManager.executing')}
        />
      </div>
    </div>
  )
}

interface ToolPayload {
  id?: string
  name?: string
  args?: string
  delta?: string
  content?: string
}

function parsePayload<T>(data: string): T | null {
  try {
    return JSON.parse(data) as T
  } catch {
    return null
  }
}
