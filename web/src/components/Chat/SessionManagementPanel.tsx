import { useMemo, useState } from 'react'
import { Check, Edit3, LogIn, MessageSquareText, Plus, Search, Trash2, X } from 'lucide-react'
import type { SessionSummary } from '@/lib/api'

interface SessionManagementPanelProps {
  sessions: SessionSummary[]
  activeSessionId: string
  disabled?: boolean
  onCreate: (title?: string) => void | Promise<void>
  onSwitch: (id: string) => void | Promise<void>
  onRename: (id: string, title: string) => void | Promise<void>
  onDelete: (id: string) => void | Promise<void>
  onEnterChat: () => void
}

/** 右侧面板内的完整会话管理视图，承载搜索、切换、重命名和删除。 */
export function SessionManagementPanel({
  sessions,
  activeSessionId,
  disabled = false,
  onCreate,
  onSwitch,
  onRename,
  onDelete,
  onEnterChat,
}: SessionManagementPanelProps) {
  const [query, setQuery] = useState('')
  const [editingId, setEditingId] = useState('')
  const [draftTitle, setDraftTitle] = useState('')

  const filteredSessions = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    const sorted = [...sessions].sort((a, b) => Date.parse(b.updated_at || b.created_at || '') - Date.parse(a.updated_at || a.created_at || ''))
    if (!keyword) return sorted
    return sorted.filter((session) => (session.title || '新会话').toLowerCase().includes(keyword))
  }, [query, sessions])

  const activeSession = sessions.find((session) => session.id === activeSessionId) ||
    sessions.find((session) => session.active) ||
    sessions[0]

  const handleCreate = async () => {
    if (disabled) return
    await onCreate()
    onEnterChat()
  }

  const beginRename = (session: SessionSummary) => {
    setEditingId(session.id)
    setDraftTitle(session.title || '新会话')
  }

  const cancelRename = () => {
    setEditingId('')
    setDraftTitle('')
  }

  const submitRename = async (id: string) => {
    const title = draftTitle.trim()
    if (!title) {
      cancelRename()
      return
    }
    await onRename(id, title)
    cancelRename()
  }

  const handleDelete = async (id: string) => {
    if (disabled || sessions.length <= 1) return
    await onDelete(id)
  }

  const enterSession = async (id: string) => {
    if (disabled) return
    if (id !== activeSessionId) await onSwitch(id)
    onEnterChat()
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-bg)]">
      <div className="border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3">
        <div className="flex items-center gap-2">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              className="nova-field h-8 w-full rounded border pl-7 pr-2 text-xs outline-none"
              placeholder="搜索会话标题"
              aria-label="搜索会话"
            />
          </div>
          <button
            type="button"
            disabled={disabled}
            onClick={() => void handleCreate()}
            className="nova-nav-item flex h-8 shrink-0 items-center gap-1.5 border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 text-xs disabled:cursor-not-allowed disabled:opacity-45"
          >
            <Plus className="h-3.5 w-3.5" />
            新建
          </button>
        </div>
        <div className="mt-2 flex items-center justify-between text-[11px] text-[var(--nova-text-faint)]">
          <span>{filteredSessions.length} / {sessions.length} 个会话</span>
          <span className="truncate">当前：{activeSession?.title || '暂无会话'}</span>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {filteredSessions.length === 0 ? (
          <div className="flex h-full items-center justify-center px-4 text-center text-xs text-[var(--nova-text-faint)]">
            没有匹配的会话
          </div>
        ) : (
          <div className="space-y-1.5">
            {filteredSessions.map((session) => {
              const active = session.id === activeSessionId || session.active
              const editing = editingId === session.id
              return (
                <div
                  key={session.id}
                  className={`rounded-[var(--nova-radius)] border px-2.5 py-2 ${
                    active
                      ? 'border-[var(--nova-border)] bg-[var(--nova-active)]'
                      : 'border-transparent bg-[var(--nova-surface)] hover:border-[var(--nova-border)]'
                  }`}
                >
                  <div className="flex min-w-0 items-start gap-2">
                    <MessageSquareText className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${active ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
                    <div className="min-w-0 flex-1">
                      {editing ? (
                        <input
                          autoFocus
                          value={draftTitle}
                          onChange={(event) => setDraftTitle(event.target.value)}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter') void submitRename(session.id)
                            if (event.key === 'Escape') cancelRename()
                          }}
                          className="nova-field h-7 w-full rounded border px-2 text-xs outline-none"
                          aria-label="会话标题"
                        />
                      ) : (
                        <button
                          type="button"
                          disabled={disabled}
                          onClick={() => void onSwitch(session.id)}
                          className="block max-w-full truncate text-left text-xs font-medium text-[var(--nova-text)] disabled:cursor-not-allowed"
                          title={session.title || '新会话'}
                        >
                          {session.title || '新会话'}
                        </button>
                      )}
                      <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-[var(--nova-text-faint)]">
                        <span>{session.message_count} 条消息</span>
                        <span>{formatSessionTime(session.updated_at || session.created_at)}</span>
                        {active && <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[var(--nova-text-muted)]">当前</span>}
                      </div>
                    </div>

                    <div className="flex shrink-0 items-center gap-0.5">
                      {editing ? (
                        <>
                          <button
                            type="button"
                            onClick={() => void submitRename(session.id)}
                            className="nova-nav-item rounded p-1"
                            aria-label={`保存会话 ${session.title}`}
                            title="保存"
                          >
                            <Check className="h-3.5 w-3.5" />
                          </button>
                          <button
                            type="button"
                            onClick={cancelRename}
                            className="nova-nav-item rounded p-1"
                            aria-label="取消重命名"
                            title="取消"
                          >
                            <X className="h-3.5 w-3.5" />
                          </button>
                        </>
                      ) : (
                        <>
                          <button
                            type="button"
                            disabled={disabled}
                            onClick={() => void enterSession(session.id)}
                            className="nova-nav-item rounded p-1 disabled:cursor-not-allowed disabled:opacity-40"
                            aria-label={`进入会话 ${session.title}`}
                            title="进入对话"
                          >
                            <LogIn className="h-3.5 w-3.5" />
                          </button>
                          <button
                            type="button"
                            disabled={disabled}
                            onClick={() => beginRename(session)}
                            className="nova-nav-item rounded p-1 disabled:cursor-not-allowed disabled:opacity-40"
                            aria-label={`重命名会话 ${session.title}`}
                            title="重命名"
                          >
                            <Edit3 className="h-3.5 w-3.5" />
                          </button>
                          <button
                            type="button"
                            disabled={disabled || sessions.length <= 1}
                            onClick={() => void handleDelete(session.id)}
                            className="nova-nav-item rounded p-1 hover:bg-[#4a2b2b] hover:text-[#ff8a8a] disabled:cursor-not-allowed disabled:opacity-30"
                            aria-label={`删除会话 ${session.title}`}
                            title={sessions.length <= 1 ? '至少保留一个会话' : '删除'}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

function formatSessionTime(value: string) {
  if (!value) return '时间未知'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未知'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}
