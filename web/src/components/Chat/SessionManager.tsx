import { useState } from 'react'
import { Edit3, Plus, Trash2 } from 'lucide-react'
import type { SessionSummary } from '@/lib/api'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface SessionManagerProps {
  sessions: SessionSummary[]
  activeSessionId: string
  disabled?: boolean
  onCreate: (title?: string) => void | Promise<void>
  onSwitch: (id: string) => void | Promise<void>
  onRename: (id: string, title: string) => void | Promise<void>
  onDelete: (id: string) => void | Promise<void>
}

/** 会话管理面板，提供创建、切换、重命名和删除当前 workspace 内的会话。 */
export function SessionManager({
  sessions,
  activeSessionId,
  disabled = false,
  onCreate,
  onSwitch,
  onRename,
  onDelete,
}: SessionManagerProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [draftTitle, setDraftTitle] = useState('')
  const activeSession = sessions.find(session => session.id === activeSessionId) ||
    sessions.find(session => session.active) ||
    sessions[0]

  const beginRename = () => {
    if (!activeSession) return
    setIsEditing(true)
    setDraftTitle(activeSession.title)
  }

  const submitRename = async () => {
    const title = draftTitle.trim()
    if (!activeSession || !title) {
      setIsEditing(false)
      return
    }
    await onRename(activeSession.id, title)
    setIsEditing(false)
  }

  const handleDelete = async () => {
    if (!activeSession || sessions.length <= 1) return
    await onDelete(activeSession.id)
  }

  return (
    <div className="flex min-w-0 flex-1 items-center gap-1.5">
      <span className="shrink-0 text-[11px] font-medium text-[#858b96]">会话</span>
      {isEditing && activeSession ? (
        <input
          autoFocus
          value={draftTitle}
          onChange={(event) => setDraftTitle(event.target.value)}
          onBlur={() => void submitRename()}
          onKeyDown={(event) => {
            if (event.key === 'Enter') void submitRename()
            if (event.key === 'Escape') setIsEditing(false)
          }}
          className="min-w-0 flex-1 rounded border border-[#4b5563] bg-[#1b1c1f] px-2 py-0.5 text-xs text-[#d7dbe2] outline-none"
          aria-label="会话标题"
        />
      ) : (
        <Select
          value={activeSession?.id || ''}
          disabled={disabled || sessions.length === 0}
          onValueChange={(id) => void onSwitch(id)}
        >
          <SelectTrigger
            size="sm"
            className="min-w-0 flex-1 border-[#303238] bg-[#25262a] px-2 py-0.5 text-xs text-[#d7dbe2] outline-none hover:bg-[#303238] focus:ring-0"
            aria-label="选择会话"
            title={activeSession ? `${activeSession.title} · ${activeSession.message_count} 条消息` : '暂无会话'}
          >
            <SelectValue placeholder="暂无会话" />
          </SelectTrigger>
          <SelectContent className="border-[#303238] bg-[#25262a] text-[#d7dbe2]">
            {sessions.length === 0 ? (
              <SelectItem value="empty" disabled>暂无会话</SelectItem>
            ) : sessions.map(session => (
              <SelectItem key={session.id} value={session.id}>
                {session.title || '新会话'} · {session.message_count} 条
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      <div className="flex shrink-0 items-center gap-0.5">
        <button
          type="button"
          disabled={disabled}
          onClick={() => void onCreate()}
          className="inline-flex items-center gap-0.5 rounded px-1 py-0.5 text-[11px] text-[#aeb4bf] hover:bg-[#303238] disabled:cursor-not-allowed disabled:opacity-50"
          aria-label="新建会话"
        >
          <Plus className="h-3 w-3" />
        </button>
        <button
          type="button"
          disabled={disabled || !activeSession}
          onClick={beginRename}
          className="rounded p-0.5 text-[#858b96] hover:bg-[#3a3d45] hover:text-[#d7dbe2] disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={activeSession ? `重命名会话 ${activeSession.title}` : '重命名会话'}
        >
          <Edit3 className="h-3 w-3" />
        </button>
        <button
          type="button"
          disabled={disabled || !activeSession || sessions.length <= 1}
          onClick={() => void handleDelete()}
          className="rounded p-0.5 text-[#858b96] hover:bg-[#4a2b2b] hover:text-[#ff8a8a] disabled:cursor-not-allowed disabled:opacity-30"
          aria-label={activeSession ? `删除会话 ${activeSession.title}` : '删除会话'}
        >
          <Trash2 className="h-3 w-3" />
        </button>
      </div>
    </div>
  )
}
