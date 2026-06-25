import { useEffect, useMemo, useState } from 'react'
import { Activity, Bot, Check, FileText, Loader2, PenLine, Plus, SearchCheck, Sparkles, WandSparkles, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { fetchSettings, updateWorkspaceSettings } from '@/features/settings/api'
import type { Teller } from '@/features/interactive/types'
import { removeChatContextCompaction } from '@/lib/api'
import type { ChapterSummary, ChatMessage, ContextAnalysis, SessionSummary, TextSelection } from '@/lib/api'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { BUILTIN_WRITING_SKILLS, DEFAULT_WRITING_SKILL, useWritingSkillOptions, type WritingSkillOption } from '@/hooks/useWritingSkillOptions'
import { MessageList } from './MessageList'
import { InputArea } from './InputArea'
import { SessionManagementPanel } from './SessionManagementPanel'
import { AgentTracePanel } from './AgentTracePanel'
import { SubAgentSessionPanel } from './SubAgentSessionPanel'
import { CONTEXT_ANALYSIS_SIMULATED_MESSAGE, ContextAnalysisDialog } from './ContextAnalysisDialog'
import { subAgentSessionKey } from './subagent-session'
import type { ReferencePickerItem } from './FileReferencePicker'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from '@/components/ui/dropdown-menu'

type AgentPanelView = 'chat' | 'sessions' | 'traces'

const WRITING_AGENT_INIT_EVENT = 'nova:writing-agent-init'

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
  styleScenes: string[]
  textSelections: TextSelection[]
  fileSuggestions: string[]
  onCreateSession: (title?: string) => void | Promise<void>
  onSwitchSession: (id: string) => void | Promise<void>
  onRenameSession: (id: string, title: string) => void | Promise<void>
  onDeleteSession: (id: string) => void | Promise<void>
  onSend: (message: string, options?: { writingSkill?: string }) => void
  onAnalyzeContext: (message: string, options?: { writingSkill?: string }) => Promise<ContextAnalysis>
  onStop: () => void
  onReferenceRemove: (path: string) => void
  onLoreReferenceAdd: (id: string) => void
  onLoreReferenceRemove: (id: string) => void
  onStyleSceneAdd: (scene: string) => void
  onStyleSceneRemove: (scene: string) => void
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
  styleScenes,
  textSelections,
  fileSuggestions,
  onCreateSession,
  onSwitchSession,
  onRenameSession,
  onDeleteSession,
  onSend,
  onAnalyzeContext,
  onStop,
  onReferenceRemove,
  onLoreReferenceAdd,
  onLoreReferenceRemove,
  onStyleSceneAdd,
  onStyleSceneRemove,
  onTextSelectionRemove,
  onClose,
}: AgentPanelProps) {
  const { t } = useTranslation()
  const [view, setView] = useState<AgentPanelView>('chat')
  const [inputPrefill, setInputPrefill] = useState<{ prompt: string; nonce: number } | null>(null)
  const [contextAnalysisOpen, setContextAnalysisOpen] = useState(false)
  const [contextAnalysisLoading, setContextAnalysisLoading] = useState(false)
  const [contextAnalysisError, setContextAnalysisError] = useState<string | null>(null)
  const [contextAnalysis, setContextAnalysis] = useState<ContextAnalysis | null>(null)
  const [activeSubAgentSessionKey, setActiveSubAgentSessionKey] = useState('')
  const [ideTellerId, setIdeTellerId] = useState('classic')
  const [writingSkill, setWritingSkill] = useState(DEFAULT_WRITING_SKILL)
  const skillCommands = useSkillCommands({ agentKey: 'ide', workspace, fallbackEnabled: true })
  const writingSkillOptions = useWritingSkillOptions(workspace)
  const activeSession = sessions.find((session) => session.id === activeSessionId) ||
    sessions.find((session) => session.active) ||
    sessions[0]
  const tokenUsageMessages = useMemo(
    () => messages.filter((message) => message.role === 'token_usage'),
    [messages],
  )
  const styleSceneSuggestions = useMemo(() => {
    const teller = tellers.find((item) => item.id === ideTellerId) || tellers.find((item) => item.id === 'classic') || tellers[0]
    return Array.from(new Set((teller?.style_rules || []).map((rule) => rule.scene.trim()).filter(Boolean)))
  }, [ideTellerId, tellers])

  useEffect(() => {
    const handleWritingInitRequest = (event: Event) => {
      const detail = (event as CustomEvent<{ prompt?: string }>).detail
      const prompt = detail?.prompt || t('writingAgent.initPrompt')
      setView('chat')
      setInputPrefill((current) => ({ prompt, nonce: (current?.nonce || 0) + 1 }))
    }
    window.addEventListener(WRITING_AGENT_INIT_EVENT, handleWritingInitRequest)
    return () => window.removeEventListener(WRITING_AGENT_INIT_EVENT, handleWritingInitRequest)
  }, [t])

  useEffect(() => {
    let cancelled = false
    if (!workspace) {
      setWritingSkill(DEFAULT_WRITING_SKILL)
      return () => { cancelled = true }
    }
    fetchSettings()
      .then((settings) => {
        if (!cancelled) setWritingSkill(settings.effective.writing_skill_default || DEFAULT_WRITING_SKILL)
      })
      .catch(() => {
        if (!cancelled) setWritingSkill(DEFAULT_WRITING_SKILL)
      })
    return () => { cancelled = true }
  }, [workspace])

  const handleAnalyzeContext = async (message: string) => {
    setContextAnalysisLoading(true)
    setContextAnalysisError(null)
    setContextAnalysis(null)
    try {
      setContextAnalysis(await onAnalyzeContext(message, { writingSkill }))
    } catch (e) {
      setContextAnalysis(null)
      setContextAnalysisError((e as Error).message)
    } finally {
      setContextAnalysisLoading(false)
    }
  }

  const openContextAnalysis = () => {
    setContextAnalysisOpen(true)
    void handleAnalyzeContext(CONTEXT_ANALYSIS_SIMULATED_MESSAGE)
  }

  const openSubAgentSession = (message: ChatMessage) => {
    const key = subAgentSessionKey(message)
    if (key) setActiveSubAgentSessionKey(key)
  }

  const removeContextCompaction = async () => {
    await removeChatContextCompaction()
    await handleAnalyzeContext(CONTEXT_ANALYSIS_SIMULATED_MESSAGE)
  }

  const sendWithWritingSkill = (message: string) => {
    onSend(message, { writingSkill })
  }

  return (
    <aside className="nova-sidebar relative flex h-full min-h-0 flex-col overflow-hidden">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
        <div className="flex min-w-0 shrink-0 items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
          <Bot className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          {t('chat.agent')}
        </div>
        <div className="flex h-7 min-w-0 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('chat.panelSwitch')}>
          <button
            type="button"
            onClick={() => setView('chat')}
            className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'chat' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            {t('chat.view.chat')}
          </button>
          <button
            type="button"
            onClick={() => setView('sessions')}
            className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'sessions' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            {t('chat.view.sessions')}
          </button>
          <button
            type="button"
            onClick={() => setView('traces')}
            className={`rounded-[6px] px-1.5 py-0.5 text-[11px] transition-colors ${view === 'traces' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
            aria-label={t('chat.view.traces')}
            title={t('chat.view.traces')}
          >
            <Activity className="h-3 w-3" />
          </button>
        </div>
        <div className="min-w-0 flex-1" />
        <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{isStreaming ? t('chat.status.streaming') : t('chat.status.idle')}</span>
        <button
          type="button"
          onClick={onClose}
          className="nova-nav-item rounded p-1"
          aria-label={t('chat.closeAgent')}
          title={t('common.close')}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      {view === 'chat' ? (
        <>
          <div className="flex min-h-[42px] shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3">
            <IdeTellerSelector workspace={workspace} tellers={tellers} onValueChange={setIdeTellerId} />
            <div className="flex min-w-0 flex-1 items-center text-[11px] text-[var(--nova-text-faint)]" title={activeSession ? `${activeSession.title} · ${t('common.messages', { count: activeSession.message_count })}` : t('chat.noSession')}>
              <span className="shrink-0 text-[var(--nova-text-muted)]">{t('chat.current')}</span>
              <span className="min-w-0 truncate">{activeSession?.title || t('chat.noSession')}</span>
              {activeSession && <span className="shrink-0"> · {activeSession.message_count}</span>}
            </div>
            <button
              type="button"
              disabled={isStreaming}
              onClick={() => void onCreateSession()}
              className="nova-nav-item flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] disabled:cursor-not-allowed disabled:opacity-45"
              aria-label={t('chat.newSession')}
              title={t('chat.newSession')}
            >
              <Plus className="h-3.5 w-3.5" />
              {t('chat.new')}
            </button>
          </div>
          <div className="relative flex min-h-0 flex-1">
            <div className="relative flex min-w-0 flex-1 flex-col">
              {messages.length === 0 && !isStreaming && (
                <AgentQuickActions
                  chapter={currentChapter}
                  selectedFile={selectedFile}
                  onSend={sendWithWritingSkill}
                />
              )}
              <MessageList
                messages={messages}
                isStreaming={isStreaming}
                activityContent={activityContent}
                scrollResetKey={`${workspace || 'none'}:${activeSessionId || 'current'}`}
                bottomPaddingClassName="pb-36"
                onOpenSubAgentSession={openSubAgentSession}
                activeSubAgentSessionKey={activeSubAgentSessionKey}
              />
              <InputArea
                onSend={sendWithWritingSkill}
                onStop={onStop}
                disabled={isStreaming}
                draftKey={`ide-agent:${workspace || 'global'}`}
                inputPrefill={inputPrefill}
                onInputPrefillConsumed={() => setInputPrefill(null)}
                referencedFiles={references}
                onReferenceRemove={onReferenceRemove}
                fileSuggestions={fileSuggestions}
                loreReferences={loreReferences}
                loreReferenceLabels={loreReferenceLabels}
                onLoreReferenceAdd={onLoreReferenceAdd}
                onLoreReferenceRemove={onLoreReferenceRemove}
                loreSuggestions={loreSuggestions}
                styleScenes={styleScenes}
                onStyleSceneAdd={onStyleSceneAdd}
                onStyleSceneRemove={onStyleSceneRemove}
                styleSceneSuggestions={styleSceneSuggestions}
                textSelections={textSelections}
                onTextSelectionRemove={onTextSelectionRemove}
                skills={skillCommands}
                onContextAnalyze={openContextAnalysis}
                tokenUsageMessages={tokenUsageMessages}
                agentKey="ide"
                workspace={workspace}
                writingSkillControl={<WritingSkillSelector workspace={workspace} value={writingSkill} options={writingSkillOptions} onValueChange={setWritingSkill} />}
                floating
              />
            </div>
            {activeSubAgentSessionKey && (
              <>
                <div className="hidden min-w-[280px] flex-[0_0_42%] md:block">
                  <SubAgentSessionPanel
                    messages={messages}
                    sessionKey={activeSubAgentSessionKey}
                    onClose={() => setActiveSubAgentSessionKey('')}
                  />
                </div>
                <div className="absolute inset-0 z-30 md:hidden">
                  <SubAgentSessionPanel
                    messages={messages}
                    sessionKey={activeSubAgentSessionKey}
                    onClose={() => setActiveSubAgentSessionKey('')}
                  />
                </div>
              </>
            )}
          </div>
          <ContextAnalysisDialog
            open={contextAnalysisOpen}
            loading={contextAnalysisLoading}
            error={contextAnalysisError}
            analysis={contextAnalysis}
            onOpenChange={setContextAnalysisOpen}
            onRemoveCompaction={removeContextCompaction}
          />
        </>
      ) : view === 'sessions' ? (
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
      ) : (
        <AgentTracePanel disabled={isStreaming} />
      )}
    </aside>
  )
}

function IdeTellerSelector({ workspace, tellers, onValueChange }: { workspace: string; tellers: Teller[]; onValueChange?: (value: string) => void }) {
  const { t } = useTranslation()
  const [value, setValue] = useState('classic')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let cancelled = false
    if (!workspace) {
      setValue('classic')
      onValueChange?.('classic')
      return () => { cancelled = true }
    }
    fetchSettings()
      .then((settings) => {
        const next = settings.effective.ide_story_teller_id || 'classic'
        if (!cancelled) {
          setValue(next)
          onValueChange?.(next)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setValue('classic')
          onValueChange?.('classic')
        }
      })
    return () => { cancelled = true }
  }, [onValueChange, workspace])

  const handleChange = async (next: string) => {
    if (!workspace || next === value) return
    const previous = value
    setValue(next)
    onValueChange?.(next)
    setSaving(true)
    try {
      const settings = await fetchSettings()
      await updateWorkspaceSettings({ ...settings.workspace, ide_story_teller_id: next })
      window.dispatchEvent(new CustomEvent('nova:settings-updated'))
    } catch (e) {
      console.warn('保存 IDE 默认导演失败', e)
      setValue(previous)
      onValueChange?.(previous)
    } finally {
      setSaving(false)
    }
  }

  if (tellers.length === 0) return null

  return (
    <label className="flex min-w-[126px] max-w-[170px] shrink-0 items-center gap-1.5 text-[11px] text-[var(--nova-text-faint)]" title={t('chat.tellerTitle')}>
      <span className="shrink-0">{t('chat.teller')}</span>
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

function WritingSkillSelector({ workspace, value, options, onValueChange }: { workspace: string; value: string; options: WritingSkillOption[]; onValueChange: (value: string) => void }) {
  const { t } = useTranslation()
  const [savingSkill, setSavingSkill] = useState<string | null>(null)

  const normalizedOptions = useMemo(() => {
    if (options.some((option) => option.name === value)) return options
    return [fallbackWritingSkillOption(value || DEFAULT_WRITING_SKILL), ...options]
  }, [options, value])
  const selected = normalizedOptions.find((option) => option.name === value) || normalizedOptions.find((option) => option.name === DEFAULT_WRITING_SKILL)
  const selectedSource = selected?.scope || 'builtin'
  const currentLabel = selected ? `${writingSkillLabel(selected, t)} · ${t(`chat.writingSkill.source.${selectedSource}`)}` : value

  const handleChange = async (next: string) => {
    if (!workspace || next === value || savingSkill) return
    const previous = value
    onValueChange(next)
    setSavingSkill(next)
    try {
      const settings = await fetchSettings()
      await updateWorkspaceSettings({ ...settings.workspace, writing_skill_default: next })
      window.dispatchEvent(new CustomEvent('nova:settings-updated'))
    } catch (e) {
      console.warn('保存写作 Skill 失败', e)
      onValueChange(previous)
    } finally {
      setSavingSkill(null)
    }
  }

  return (
    <>
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          disabled={!workspace || normalizedOptions.length === 0}
          className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
          title={selected?.path || t('chat.writingSkillTitle')}
          aria-label={t('chat.writingSkill')}
        >
          <Sparkles className="h-3.5 w-3.5" />
          <span className="min-w-0 flex-1">{t('chat.writingSkill')}</span>
          <span className="max-w-36 truncate text-[10px] text-[var(--nova-text-faint)]">{currentLabel}</span>
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent className="w-72 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
          {normalizedOptions.map((option) => {
            const label = writingSkillLabel(option, t)
            const selectedOption = option.name === selected?.name
            return (
              <DropdownMenuItem
                key={`${option.scope}:${option.name}`}
                disabled={Boolean(savingSkill)}
                onSelect={(event) => {
                  event.preventDefault()
                  void handleChange(option.name)
                }}
                onClick={() => void handleChange(option.name)}
                className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
              >
                {savingSkill === option.name ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className={`h-3.5 w-3.5 ${selectedOption ? 'opacity-100' : 'opacity-0'}`} />}
                <span className="min-w-0 flex-1 truncate">{label}</span>
                <span className="text-[10px] text-[var(--nova-text-faint)]">{t(`chat.writingSkill.source.${option.scope}`)}</span>
              </DropdownMenuItem>
            )
          })}
          {normalizedOptions.length === 0 && (
            <DropdownMenuItem disabled className="text-xs">
              {t('chat.writingSkill.empty')}
            </DropdownMenuItem>
          )}
        </DropdownMenuSubContent>
      </DropdownMenuSub>
      <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
    </>
  )
}

function fallbackWritingSkillOption(name: string): WritingSkillOption {
  const scope = BUILTIN_WRITING_SKILLS.includes(name as typeof BUILTIN_WRITING_SKILLS[number]) ? 'builtin' : 'workspace'
  return { name, description: '', scope, path: '', active: true, agent: 'ide' }
}

function writingSkillLabel(option: Pick<WritingSkillOption, 'name'>, t: ReturnType<typeof useTranslation>['t']) {
  switch (option.name) {
    case 'novel-lite':
      return t('chat.writingSkill.preset.lite')
    case 'novel-standard':
      return t('chat.writingSkill.preset.standard')
    case 'novel-heavy':
      return t('chat.writingSkill.preset.heavy')
    default:
      return option.name
  }
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
  const { t } = useTranslation()
  const target = chapter ? t('chat.quick.targetChapter', { title: chapter.display_title }) : (selectedFile ? t('chat.quick.targetFile', { file: selectedFile }) : t('chat.quick.targetWork'))
  const actions = useMemo(() => [
    { label: t('chat.quick.nextGroup'), icon: FileText, prompt: '请基于当前大纲、已定稿章节、progress.md、character-states.md 和资料库长期设定，生成接下来一个短期情节单元的章节组细纲。只规划下一组，不要批量生成很多组；细纲要短而可维护，方便阅读、评论和后续更新，每章只写关键点，不写长篇背景解释；如实际定稿已经偏离大纲，请先指出偏差并让我确认是调整大纲还是拉回主线。' },
    { label: t('chat.quick.writeNextChapter'), icon: PenLine, prompt: '请读取当前章节组细纲、长期大纲、progress.md、character-states.md、资料库长期设定和前面至少两章定稿正文，按细纲安排创作下一章。写作前请先按长期大纲的卷章安排和已有章节路径判断下一章所属分卷；若属于某一卷，请写入 chapters/<分卷名>/ 下符合章节文件名模板的文件。若草稿流程未启用且我没有明确要求草稿，请直接写入 chapters/ 对应分卷目录作为定稿候选。' },
    { label: t('chat.quick.continueParagraph'), icon: PenLine, prompt: `请基于${target}的上下文，续写下一段正文，保持原有叙事节奏和人物状态。` },
    { label: t('chat.quick.polishChapter'), icon: WandSparkles, prompt: `请检查并润色${target}，重点优化语句节奏、动作描写和情绪推进，不改变核心剧情。` },
    { label: t('chat.quick.finalizeState'), icon: FileText, prompt: `请将${target}视为章节定稿，检查其与前后文和当前章节组细纲的连续性，然后同步更新 progress.md 和 character-states.md；只有角色身份、人设、长期关系、能力体系或世界规则等稳定设定发生明确变化时，才更新资料库。除非我明确要求，不要修改长期大纲。` },
    { label: t('chat.quick.consistencyCheck'), icon: SearchCheck, prompt: `请对${target}做一致性检查，重点关注人物动机、时间线、道具、地点和前后文冲突。` },
  ], [target, t])

  return (
    <div className="border-b border-[var(--nova-border)] bg-[var(--nova-bg)] p-3">
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-[var(--nova-text-muted)]">
        <Sparkles className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        {t('chat.quickActions')}
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
