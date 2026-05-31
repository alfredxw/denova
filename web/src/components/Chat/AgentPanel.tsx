import { useEffect, useMemo, useState } from 'react'
import { Bot, FileText, MessageSquareText, PenLine, Plus, SearchCheck, Sparkles, WandSparkles, X } from 'lucide-react'
import { fetchSettings, updateWorkspaceSettings } from '@/features/settings/api'
import type { Teller } from '@/features/interactive/types'
import type { ChapterSummary, ChatMessage, SessionSummary, TextSelection } from '@/lib/api'
import { MessageList } from './MessageList'
import { InputArea } from './InputArea'
import { SessionManagementPanel } from './SessionManagementPanel'
import type { ReferencePickerItem } from './FileReferencePicker'

type AgentPanelView = 'chat' | 'sessions'

interface AgentPanelProps {
  workspace: string
  currentChapter?: ChapterSummary
  selectedFile: string | null
  tellers: Teller[]
  messages: ChatMessage[]
  sessions: SessionSummary[]
  activeSessionId: string
  isStreaming: boolean
  activityContent: string
  references: string[]
  loreReferences: string[]
  loreReferenceLabels: Record<string, string>
  loreSuggestions: ReferencePickerItem[]
  styleReferences: string[]
  textSelections: TextSelection[]
  fileSuggestions: string[]
  styleSuggestions: string[]
  onCreateSession: (title?: string) => void | Promise<void>
  onSwitchSession: (id: string) => void | Promise<void>
  onRenameSession: (id: string, title: string) => void | Promise<void>
  onDeleteSession: (id: string) => void | Promise<void>
  onSend: (message: string) => void
  onStop: () => void
  onReferenceRemove: (path: string) => void
  onLoreReferenceAdd: (id: string) => void
  onLoreReferenceRemove: (id: string) => void
  onStyleReferenceAdd: (path: string) => void
  onStyleReferenceRemove: (path: string) => void
  onTextSelectionRemove: (index: number) => void
  onClose: () => void
}

/** IDE 右侧创作 Agent 面板，内部支持在对话与完整会话管理之间切换。 */
export function AgentPanel({
  workspace,
  currentChapter,
  selectedFile,
  tellers,
  messages,
  sessions,
  activeSessionId,
  isStreaming,
  activityContent,
  references,
  loreReferences,
  loreReferenceLabels,
  loreSuggestions,
  styleReferences,
  textSelections,
  fileSuggestions,
  styleSuggestions,
  onCreateSession,
  onSwitchSession,
  onRenameSession,
  onDeleteSession,
  onSend,
  onStop,
  onReferenceRemove,
  onLoreReferenceAdd,
  onLoreReferenceRemove,
  onStyleReferenceAdd,
  onStyleReferenceRemove,
  onTextSelectionRemove,
  onClose,
}: AgentPanelProps) {
  const [view, setView] = useState<AgentPanelView>('chat')
  const activeSession = sessions.find((session) => session.id === activeSessionId) ||
    sessions.find((session) => session.active) ||
    sessions[0]

  return (
    <aside className="nova-sidebar flex h-full min-h-0 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
        <div className="flex min-w-0 shrink-0 items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
          <Bot className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          创作Agent
        </div>
        <div className="flex h-7 min-w-0 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label="Agent 面板切换">
          <button
            type="button"
            onClick={() => setView('chat')}
            className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'chat' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            对话
          </button>
          <button
            type="button"
            onClick={() => setView('sessions')}
            className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'sessions' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            会话
          </button>
        </div>
        <div className="min-w-0 flex-1" />
        <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{isStreaming ? '创作中…' : '等待'}</span>
        <button
          type="button"
          onClick={onClose}
          className="nova-nav-item rounded p-1"
          aria-label="关闭创作 Agent"
          title="关闭"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      {view === 'chat' ? (
        <>
          <div className="flex min-h-[42px] shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3">
            <IdeTellerSelector workspace={workspace} tellers={tellers} />
            <div className="flex min-w-0 flex-1 items-center text-[11px] text-[var(--nova-text-faint)]" title={activeSession ? `${activeSession.title} · ${activeSession.message_count} 条消息` : '暂无会话'}>
              <span className="shrink-0 text-[var(--nova-text-muted)]">当前：</span>
              <span className="min-w-0 truncate">{activeSession?.title || '暂无会话'}</span>
              {activeSession && <span className="shrink-0"> · {activeSession.message_count} 条</span>}
            </div>
            <button
              type="button"
              disabled={isStreaming}
              onClick={() => void onCreateSession()}
              className="nova-nav-item flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] disabled:cursor-not-allowed disabled:opacity-45"
              aria-label="新建会话"
              title="新建会话"
            >
              <Plus className="h-3.5 w-3.5" />
              新建
            </button>
            <button
              type="button"
              onClick={() => setView('sessions')}
              className="nova-nav-item flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px]"
            >
              <MessageSquareText className="h-3.5 w-3.5" />
              管理
            </button>
          </div>
          {messages.length === 0 && !isStreaming && (
            <AgentQuickActions
              chapter={currentChapter}
              selectedFile={selectedFile}
              onSend={onSend}
            />
          )}
          <MessageList
            messages={messages}
            isStreaming={isStreaming}
            activityContent={activityContent}
            scrollResetKey={`${workspace || 'none'}:${activeSessionId || 'current'}`}
          />
          <InputArea
            onSend={onSend}
            onStop={onStop}
            disabled={isStreaming}
            referencedFiles={references}
            onReferenceRemove={onReferenceRemove}
            fileSuggestions={fileSuggestions}
            loreReferences={loreReferences}
            loreReferenceLabels={loreReferenceLabels}
            onLoreReferenceAdd={onLoreReferenceAdd}
            onLoreReferenceRemove={onLoreReferenceRemove}
            loreSuggestions={loreSuggestions}
            styleReferences={styleReferences}
            onStyleReferenceAdd={onStyleReferenceAdd}
            onStyleReferenceRemove={onStyleReferenceRemove}
            styleSuggestions={styleSuggestions}
            textSelections={textSelections}
            onTextSelectionRemove={onTextSelectionRemove}
          />
        </>
      ) : (
        <SessionManagementPanel
          sessions={sessions}
          activeSessionId={activeSessionId}
          disabled={isStreaming}
          onCreate={onCreateSession}
          onSwitch={onSwitchSession}
          onRename={onRenameSession}
          onDelete={onDeleteSession}
          onEnterChat={() => setView('chat')}
        />
      )}
    </aside>
  )
}

function IdeTellerSelector({ workspace, tellers }: { workspace: string; tellers: Teller[] }) {
  const [value, setValue] = useState('classic')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let cancelled = false
    if (!workspace) {
      setValue('classic')
      return () => { cancelled = true }
    }
    fetchSettings()
      .then((settings) => {
        if (!cancelled) setValue(settings.effective.ide_story_teller_id || 'classic')
      })
      .catch(() => {
        if (!cancelled) setValue('classic')
      })
    return () => { cancelled = true }
  }, [workspace])

  const handleChange = async (next: string) => {
    if (!workspace || next === value) return
    const previous = value
    setValue(next)
    setSaving(true)
    try {
      const settings = await fetchSettings()
      await updateWorkspaceSettings({ ...settings.workspace, ide_story_teller_id: next })
      window.dispatchEvent(new CustomEvent('nova:settings-updated'))
    } catch (e) {
      console.warn('保存 IDE 默认讲述者失败', e)
      setValue(previous)
    } finally {
      setSaving(false)
    }
  }

  if (tellers.length === 0) return null

  return (
    <label className="flex min-w-[126px] max-w-[170px] shrink-0 items-center gap-1.5 text-[11px] text-[var(--nova-text-faint)]" title="IDE 创作 Agent 下一轮使用的默认讲述者">
      <span className="shrink-0">讲述者</span>
      <select
        value={tellers.some((teller) => teller.id === value) ? value : 'classic'}
        disabled={saving}
        onChange={(event) => void handleChange(event.target.value)}
        className="nova-field h-7 min-w-0 flex-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] text-[var(--nova-text-muted)] outline-none"
      >
        {tellers.map((teller) => (
          <option key={teller.id} value={teller.id}>{teller.name}</option>
        ))}
      </select>
    </label>
  )
}

function AgentQuickActions({
  chapter,
  selectedFile,
  onSend,
}: {
  chapter?: ChapterSummary
  selectedFile: string | null
  onSend: (message: string) => void
}) {
  const target = chapter ? `当前章节《${chapter.display_title}》` : (selectedFile ? `当前文件 ${selectedFile}` : '当前作品')
  const actions = useMemo(() => [
    { label: '下一组细纲', icon: FileText, prompt: '请基于当前大纲、已定稿章节、progress.md 和角色状态，生成接下来一个短期情节单元的章节组细纲。只规划下一组，不要批量生成很多组；如实际定稿已经偏离大纲，请先指出偏差并让我确认是调整大纲还是拉回主线。' },
    { label: '按细纲写下一章', icon: PenLine, prompt: '请读取当前章节组细纲、长期大纲、progress.md、角色状态和前面至少两章定稿正文，按细纲安排创作下一章。若草稿流程未启用且我没有明确要求草稿，请直接写入 chapters/ 作为定稿候选。' },
    { label: '续写下一段', icon: PenLine, prompt: `请基于${target}的上下文，续写下一段正文，保持原有叙事节奏和人物状态。` },
    { label: '润色当前章', icon: WandSparkles, prompt: `请检查并润色${target}，重点优化语句节奏、动作描写和情绪推进，不改变核心剧情。` },
    { label: '定稿并同步状态', icon: FileText, prompt: `请将${target}视为章节定稿，检查其与前后文和当前章节组细纲的连续性，然后同步更新 progress.md 和 characters.md；除非我明确要求，不要修改长期大纲。` },
    { label: '一致性检查', icon: SearchCheck, prompt: `请对${target}做一致性检查，重点关注人物动机、时间线、道具、地点和前后文冲突。` },
  ], [target])

  return (
    <div className="border-b border-[var(--nova-border)] bg-[var(--nova-bg)] p-3">
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-[var(--nova-text-muted)]">
        <Sparkles className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        快捷创作
      </div>
      <div className="grid grid-cols-2 gap-2">
        {actions.map((action) => {
          const Icon = action.icon
          return (
            <button
              key={action.label}
              type="button"
              className="nova-nav-item flex items-center gap-2 border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-left text-xs"
              onClick={() => onSend(action.prompt)}
            >
              <Icon className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
              <span className="truncate">{action.label}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
