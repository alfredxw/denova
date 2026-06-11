import { useEffect, useRef, useState, type ChangeEvent, type KeyboardEvent } from 'react'
import { AtSign, Bot, Loader2, Send, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { type ChatMessage, type SSEEvent } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { MessageList } from '@/components/Chat/MessageList'
import { clearInteractiveTellerAgentSession, getInteractiveTellerAgentMessages, runInteractiveTellerAgentStream } from '../api'
import type { Teller, TellerAgentResult } from '../types'

const actionButtonClassName = 'nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const selectClassName = 'nova-field h-8 text-xs focus:ring-0'

interface LoreToolPayload {
  id?: string
  name?: string
  args?: string
  delta?: string
  content?: string
}

type TellerAgentChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'thinking' | 'tool_call' | 'error' | 'clear'
  content: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error'
  toolResult?: string
  targetTeller?: Teller
  tellerReferences?: Teller[]
  result?: TellerAgentResult
}
export function TellerAgentChat({
  workspace,
  tellers,
  targetTellerId,
  onTargetTellerIdChange,
  onResult,
}: {
  workspace: string
  tellers: Teller[]
  targetTellerId: string
  onTargetTellerIdChange: (id: string) => void
  onResult: (result: TellerAgentResult) => void
}) {
  const { t } = useTranslation()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const historyWorkspaceRef = useRef<string | null>(null)
  const workspaceRef = useRef(workspace)
  const [value, setValue] = useState('')
  const [referenceIds, setReferenceIds] = useState<string[]>([])
  const [referenceQuery, setReferenceQuery] = useState<string | null>(null)
  const [messages, setMessages] = useState<TellerAgentChatMessage[]>([])
  const [running, setRunning] = useState(false)
  const targetTeller = tellers.find((teller) => teller.id === targetTellerId) || null
  const normalizedQuery = (referenceQuery || '').trim().toLowerCase()
  const referencedTellers = referenceIds
    .map((id) => tellers.find((teller) => teller.id === id))
    .filter((teller): teller is Teller => Boolean(teller))
  const visibleTellers = tellers
    .filter((teller) => {
      if (referenceIds.includes(teller.id)) return false
      if (!normalizedQuery) return true
      const haystack = `${teller.name}\n${teller.id}\n${teller.description}\n${(teller.tags || []).join('\n')}`.toLowerCase()
      return haystack.includes(normalizedQuery)
    })
    .slice(0, 30)

  useEffect(() => {
    setReferenceIds((current) => current.filter((id) => tellers.some((teller) => teller.id === id)))
  }, [tellers])

  useEffect(() => {
    workspaceRef.current = workspace
    if (historyWorkspaceRef.current === workspace) return
    historyWorkspaceRef.current = workspace || null
    setValue('')
    setReferenceIds([])
    setReferenceQuery(null)
    setMessages([])
    setRunning(false)
    if (!workspace) return
    let cancelled = false
    getInteractiveTellerAgentMessages()
      .then((history) => {
        if (cancelled) return
        setMessages(history.map((message, index) => tellerHistoryMessageToChat(message, index, tellers)))
      })
      .catch((error) => {
        if (!cancelled) {
          setMessages([{ id: 'load-error', role: 'error', content: error instanceof Error ? error.message : t('settingPanel.tellerAgent.historyLoadFailed') }])
        }
      })
    return () => { cancelled = true }
  }, [workspace])

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    const nextValue = event.target.value
    setValue(nextValue)
    const atMatch = nextValue.match(/(?:^|\s)@([^\s@]*)$/)
    setReferenceQuery(atMatch ? atMatch[1] : null)
  }

  const selectReference = (teller: Teller) => {
    const nextValue = value.replace(/(?:^|\s)@([^\s@]*)$/, (match) => {
      const prefix = match.startsWith(' ') ? ' ' : ''
      return `${prefix}@${teller.name} `
    })
    setValue(nextValue === value ? `${value.trimEnd()} @${teller.name} ` : nextValue)
    setReferenceIds((current) => current.includes(teller.id) ? current : [...current, teller.id])
    setReferenceQuery(null)
    textareaRef.current?.focus()
  }

  const removeReference = (id: string) => {
    setReferenceIds((current) => current.filter((entry) => entry !== id))
  }

  const appendMessage = (message: Omit<TellerAgentChatMessage, 'id'>) => {
    setMessages((current) => [...current, { ...message, id: `${Date.now()}-${current.length}` }])
  }

  const appendStreamingMessage = (role: 'assistant' | 'thinking', content: string) => {
    if (!content) return
    setMessages((current) => {
      const last = current[current.length - 1]
      if (last?.role === role && !last.result) {
        return [...current.slice(0, -1), { ...last, content: `${last.content}${content}` }]
      }
      return [...current, { id: `${Date.now()}-${current.length}`, role, content }]
    })
  }

  const upsertToolCall = (payload: LoreToolPayload) => {
    const id = payload.id || `tool-${Date.now()}`
    const name = payload.name || t('settingPanel.tellerAgent.tool')
    setMessages((current) => {
      const existing = current.findIndex((message) => message.id === id)
      const nextMessage: TellerAgentChatMessage = {
        id,
        role: 'tool_call',
        content: name,
        name,
        args: payload.args || '',
        status: 'running',
      }
      if (existing >= 0) {
        return current.map((message, index) => index === existing ? { ...message, ...nextMessage, args: message.args || nextMessage.args } : message)
      }
      return [...current, nextMessage]
    })
  }

  const appendToolArgs = (payload: LoreToolPayload) => {
    if (!payload.id || !payload.delta) return
    setMessages((current) => current.map((message) => (
      message.id === payload.id && message.role === 'tool_call'
        ? { ...message, args: `${message.args || ''}${payload.delta}` }
        : message
    )))
  }

  const finishToolCall = (payload: LoreToolPayload) => {
    const id = payload.id
    if (!id) return
    setMessages((current) => current.map((message) => (
      message.id === id && message.role === 'tool_call'
        ? { ...message, status: 'success', toolResult: payload.content || '' }
        : message
    )))
  }

  const send = async () => {
    const instruction = value.trim()
    if (!instruction || running) return
    const activeWorkspace = workspace
    if (instruction === '/clear') {
      setRunning(true)
      try {
        await clearInteractiveTellerAgentSession()
        if (workspaceRef.current !== activeWorkspace) return
        appendMessage({ role: 'clear', content: t('settingPanel.tellerAgent.clearDone') })
        setValue('')
        setReferenceIds([])
        setReferenceQuery(null)
      } catch (error) {
        if (workspaceRef.current !== activeWorkspace) return
        appendMessage({ role: 'error', content: error instanceof Error ? error.message : t('settingPanel.tellerAgent.clearFailed') })
      } finally {
        if (workspaceRef.current === activeWorkspace) setRunning(false)
      }
      return
    }
    const refs = [...referenceIds]
    const userReferences = refs
      .map((id) => tellers.find((teller) => teller.id === id))
      .filter((teller): teller is Teller => Boolean(teller))
    appendMessage({ role: 'user', content: instruction, targetTeller: targetTeller || undefined, tellerReferences: userReferences })
    setValue('')
    setReferenceIds([])
    setReferenceQuery(null)
    setRunning(true)
    try {
      const stream = await runInteractiveTellerAgentStream(instruction, targetTeller?.id || '', refs)
      const reader = stream.getReader()
      while (true) {
        const { done, value: event } = await reader.read()
        if (done) break
        if (workspaceRef.current !== activeWorkspace) break
        handleTellerAgentEvent(event)
      }
    } catch (error) {
      if (workspaceRef.current !== activeWorkspace) return
      appendMessage({ role: 'error', content: error instanceof Error ? error.message : t('settingPanel.tellerAgent.runFailed') })
    } finally {
      if (workspaceRef.current === activeWorkspace) {
        setRunning(false)
        textareaRef.current?.focus()
      }
    }
  }

  const handleTellerAgentEvent = (event: SSEEvent) => {
    if (event.event === 'thinking') {
      const payload = parseLoreEventData<{ content?: string }>(event.data)
      appendStreamingMessage('thinking', payload?.content || '')
      return
    }
    if (event.event === 'chunk') {
      const payload = parseLoreEventData<{ content?: string }>(event.data)
      appendStreamingMessage('assistant', payload?.content || '')
      return
    }
    if (event.event === 'tool_call') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) upsertToolCall(payload)
      return
    }
    if (event.event === 'tool_args_delta') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) appendToolArgs(payload)
      return
    }
    if (event.event === 'tool_result') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) finishToolCall(payload)
      return
    }
    if (event.event === 'teller_result') {
      const result = parseLoreEventData<TellerAgentResult>(event.data)
      if (!result) {
        appendMessage({ role: 'error', content: t('settingPanel.tellerAgent.badResult') })
        return
      }
      onResult(result)
      appendMessage({ role: 'assistant', content: tellerAgentResultSummary(result, t), result })
      return
    }
    if (event.event === 'error') {
      const payload = parseLoreEventData<{ message?: string }>(event.data)
      appendMessage({ role: 'error', content: payload?.message || t('settingPanel.tellerAgent.runFailed') })
    }
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      void send()
      return
    }
    if (event.key === 'Escape') {
      setReferenceQuery(null)
    }
  }
  const chatMessages = messages.map((message) => tellerAgentMessageToChatMessage(message, t))

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
      <div className="flex h-10 shrink-0 items-center justify-between border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4">
        <div className="text-xs text-[var(--nova-text-faint)]">{t('settingPanel.tellerAgent.persistHint')}</div>
      </div>

      <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3">
        <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_220px]">
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs text-[var(--nova-text-muted)]">
            {targetTeller ? t('settingPanel.tellerAgent.referenceCurrent', { name: targetTeller.name }) : t('settingPanel.tellerAgent.noReferenceCurrent')}
          </div>
          <Select value={targetTellerId || 'none'} onValueChange={(value) => onTargetTellerIdChange(value === 'none' ? '' : value)}>
            <SelectTrigger size="sm" className={selectClassName}>
              <SelectValue placeholder={t('settingPanel.tellerAgent.selectReference')} />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectItem value="none">{t('settingPanel.tellerAgent.noneReference')}</SelectItem>
              {tellers.map((teller) => (
                <SelectItem key={teller.id} value={teller.id}>{teller.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {messages.length === 0 ? (
          <div className="h-full p-4">
            <EmptyState title={t('settingPanel.tellerAgent.title')} description={t('settingPanel.tellerAgent.emptyDesc')} />
          </div>
        ) : (
          <MessageList
            messages={chatMessages}
            isStreaming={running}
            activityContent={running ? t('settingPanel.loreAgent.running') : ''}
            collapseTraceBeforeAssistant
            scrollResetKey={`teller-agent:${workspace || 'none'}`}
            bottomPaddingClassName="pb-6"
          />
        )}
      </div>

      <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
        <div className="mx-auto max-w-4xl">
          <div className="nova-field flex min-w-0 items-end gap-2 rounded-[var(--nova-radius)] px-3 py-2">
            <Bot className="mb-2 h-4 w-4 shrink-0 text-[var(--nova-text-faint)]" />
            <div className="relative min-w-0 flex-1">
              <Popover open={referenceQuery !== null && visibleTellers.length > 0}>
                <PopoverTrigger asChild>
                  <span className="absolute bottom-full left-0 h-0 w-0" />
                </PopoverTrigger>
                <PopoverContent
                  align="start"
                  side="top"
                  className="mb-2 w-[360px] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)]"
                  onOpenAutoFocus={(event) => event.preventDefault()}
                >
                  <Command shouldFilter={false} className="bg-transparent">
                    <CommandInput value={referenceQuery || ''} readOnly placeholder={t('settingPanel.tellerAgent.searchTeller')} />
                    <CommandList>
                      <CommandEmpty>{t('settingPanel.tellerAgent.noTeller')}</CommandEmpty>
                      <CommandGroup heading={t('settingPanel.tellerAgent.referenceTeller')}>
                        {visibleTellers.map((teller) => (
                          <CommandItem
                            key={teller.id}
                            value={teller.id}
                            onSelect={() => selectReference(teller)}
                            className="cursor-pointer"
                          >
                            <span className="min-w-0 flex-1 truncate">@{teller.name}</span>
                            <span className="text-[11px] text-[var(--nova-text-faint)]">{teller.id}</span>
                          </CommandItem>
                        ))}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
              <Textarea
                ref={textareaRef}
                autoResize
                className="min-h-10 w-full resize-none border-0 bg-transparent p-0 text-sm leading-5 text-[var(--nova-text)] shadow-none outline-none placeholder:text-[var(--nova-text-faint)] focus-visible:border-transparent focus-visible:ring-0 disabled:opacity-60"
                value={value}
                onChange={handleChange}
                onKeyDown={handleKeyDown}
                placeholder={running ? t('settingPanel.tellerAgent.executing') : t('settingPanel.tellerAgent.placeholder')}
                rows={1}
                disabled={running}
              />
            </div>
            {referencedTellers.length > 0 && (
              <div className="flex max-w-[220px] flex-wrap justify-end gap-1.5">
                {referencedTellers.map((teller) => (
                  <span
                    key={teller.id}
                    className="inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-xs text-[var(--nova-text-muted)]"
                  >
                    <AtSign className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" />
                    <span className="truncate">{teller.name}</span>
                    <button
                      type="button"
                      className="rounded text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
                      onClick={() => removeReference(teller.id)}
                      aria-label={t('settingPanel.removeReference', { name: teller.name })}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
              </div>
            )}
            <Button className={actionButtonClassName} variant="outline" size="sm" disabled={running || !value.trim()} onClick={() => void send()}>
              {running ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
              {running ? t('settingPanel.executing') : t('settingPanel.send')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function tellerAgentMessageToChatMessage(message: TellerAgentChatMessage, t: (key: string, options?: Record<string, unknown>) => string): ChatMessage {
  const referenceNames = (message.tellerReferences || []).map((teller) => teller.name).filter(Boolean)
  const targetName = message.targetTeller ? t('settingPanel.result.reference', { name: message.targetTeller.name }) : ''
  const content = appendReferenceSummary(message.content, [targetName, ...referenceNames])
  return agentMessageToChatMessage(message, content, t)
}
function tellerHistoryMessageToChat(message: ChatMessage, index: number, tellers: Teller[]): TellerAgentChatMessage {
  if (message.type === 'clear') {
    return {
      id: `history-clear-${index}`,
      role: 'clear',
      content: '',
    }
  }
  const role = message.role === 'user' ? 'user' : message.role === 'error' ? 'error' : 'assistant'
  return {
    id: `history-${index}`,
    role,
    content: message.content || '',
    tellerReferences: role === 'user' ? tellerReferencesFromContent(message.content || '', tellers) : undefined,
  }
}
function tellerReferencesFromContent(content: string, tellers: Teller[]) {
  return tellers.filter((teller) => teller.name && content.includes(`@${teller.name}`))
}
function tellerAgentResultSummary(result: TellerAgentResult, t: (key: string) => string) {
  const action = result.action === 'update' ? t('settingPanel.result.updated') : t('settingPanel.result.created')
  return `${result.message || t('settingPanel.tellerAgent.done')}（${action}：${result.teller.name}）`
}


function agentMessageToChatMessage(
  message: Pick<TellerAgentChatMessage, 'id' | 'role' | 'content' | 'name' | 'args' | 'status' | 'toolResult'>,
  content: string,
  t: (key: string, options?: Record<string, unknown>) => string,
): ChatMessage {
  if (message.role === 'clear') {
    return {
      id: message.id,
      type: 'clear',
      role: 'system',
      content: content || t('settingPanel.contextCleared'),
    }
  }
  if (message.role === 'tool_call') {
    return {
      id: message.id,
      role: 'tool_call',
      content: message.name || content,
      name: message.name || content,
      args: message.args || '',
      status: message.status,
      result: message.toolResult || '',
      streaming: message.status === 'running',
    }
  }
  return {
    id: message.id,
    role: message.role,
    content,
    streaming: message.role === 'thinking' && message.status === 'running',
  }
}

function appendReferenceSummary(content: string, names: string[]) {
  const visibleNames = names.map((name) => name.trim()).filter(Boolean)
  if (visibleNames.length === 0) return content
  return `${content}

${visibleNames.map((name) => `@${name}`).join(' ')}`
}

function parseLoreEventData<T>(data: string): T | null {
  try {
    return JSON.parse(data) as T
  } catch {
    return null
  }
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 text-center">
        <div className="text-sm font-medium text-[var(--nova-text)]">{title}</div>
        <div className="mt-1 text-xs text-[var(--nova-text-faint)]">{description}</div>
      </div>
    </div>
  )
}
