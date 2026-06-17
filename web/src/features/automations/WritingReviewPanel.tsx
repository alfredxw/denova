import { useCallback, useEffect, useMemo, useState } from 'react'
import { Check, ClipboardCheck, ExternalLink, Inbox, MessageSquareText, Play, RefreshCw, Settings, Square, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { InputArea } from '@/components/Chat/InputArea'
import { MessageList } from '@/components/Chat/MessageList'
import {
  checkAutomation,
  confirmAutomationInboxItem,
  dismissAutomationInboxItem,
  getActiveAutomationRuns,
  getAutomationInbox,
  getAutomations,
  markAutomationInboxItemRead,
  type AutomationActiveRun,
  type AutomationInboxItem,
  type AutomationRunRecord,
  type AutomationTask,
} from '@/lib/api'
import { useAutomationRunStream } from './useAutomationRunStream'

interface WritingReviewPanelProps {
  workspace: string
  selectedFile: string | null
  fileSuggestions: string[]
  onOpenConfig: () => void
  onOpenFile: (path: string) => void | Promise<void>
  onSendToWritingAgent: (prompt: string) => void
}

export function WritingReviewPanel({
  workspace,
  selectedFile,
  fileSuggestions,
  onOpenConfig,
  onOpenFile,
  onSendToWritingAgent,
}: WritingReviewPanelProps) {
  const { t } = useTranslation()
  const [tasks, setTasks] = useState<AutomationTask[]>([])
  const [inboxItems, setInboxItems] = useState<AutomationInboxItem[]>([])
  const [activeRuns, setActiveRuns] = useState<AutomationActiveRun[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const chapterOptions = useMemo(() => fileSuggestions.filter(isChapterPath), [fileSuggestions])
  const [selectedReviewPaths, setSelectedReviewPaths] = useState<string[]>(() => (
    selectedFile && isChapterPath(selectedFile) ? [selectedFile] : []
  ))
  const runStream = useAutomationRunStream({ onFinished: () => void load() })

  const reviewTasks = useMemo(() => tasks.filter((task) => task.template === 'review'), [tasks])
  const reviewTaskIDs = useMemo(() => new Set(reviewTasks.map((task) => task.id).filter(Boolean) as string[]), [reviewTasks])
  const primaryTask = useMemo(() => (
    reviewTasks.find((task) => task.enabled)
    || reviewTasks.find((task) => task.id === 'workspace-auto-review')
    || reviewTasks[0]
    || null
  ), [reviewTasks])
  const reviewInboxItems = useMemo(() => (
    inboxItems
      .filter((item) => reviewTaskIDs.has(item.task_id))
      .sort((a, b) => Date.parse(b.updated_at || b.created_at) - Date.parse(a.updated_at || a.created_at))
  ), [inboxItems, reviewTaskIDs])
  const activeReviewRuns = useMemo(() => activeRuns.filter((item) => reviewTaskIDs.has(item.task_id)), [activeRuns, reviewTaskIDs])
  const recentRuns = useMemo(() => (
    reviewTasks
      .flatMap((task) => task.recent_runs || [])
      .sort((a, b) => Date.parse(b.started_at) - Date.parse(a.started_at))
  ), [reviewTasks])
  const focusedItem = reviewInboxItems[0] || null

  const load = useCallback(async () => {
    if (!workspace) {
      setTasks([])
      setInboxItems([])
      setActiveRuns([])
      setLoading(false)
      return
    }
    setLoading(true)
    setError('')
    try {
      const [nextTasks, nextInbox, nextActiveRuns] = await Promise.all([
        getAutomations(),
        getAutomationInbox(),
        getActiveAutomationRuns(),
      ])
      setTasks(nextTasks)
      setInboxItems(nextInbox)
      setActiveRuns(nextActiveRuns)
    } catch (e) {
      setError(e instanceof Error ? e.message : t('writingReview.error.load'))
    } finally {
      setLoading(false)
    }
  }, [t, workspace])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!selectedFile || !isChapterPath(selectedFile)) return
    setSelectedReviewPaths((current) => current.length > 0 ? current : [selectedFile])
  }, [selectedFile])

  const runTask = async () => {
    if (!primaryTask?.id) return
    if (selectedReviewPaths.length === 0) {
      setError(t('writingReview.error.noChapters'))
      return
    }
    setError('')
    try {
      const evidence = selectedReviewPaths.map((path) => ({
        source: 'manual_chapter_selection',
        title: chapterTitle(path),
        ref: path,
        snippet: t('writingReview.manualEvidence', { path }),
      }))
      await runStream.start(primaryTask.id, t('automations.run.userMessage', { name: primaryTask.name }), evidence)
    } catch (e) {
      setError(e instanceof Error ? e.message : t('writingReview.error.run'))
    }
  }

  const checkTask = async () => {
    if (!primaryTask?.id) return
    setError('')
    try {
      await checkAutomation(primaryTask.id)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : t('writingReview.error.check'))
    }
  }

  const confirmItem = async (item: AutomationInboxItem) => {
    setError('')
    try {
      const result = await confirmAutomationInboxItem(item.id)
      setInboxItems((current) => current.map((entry) => entry.id === result.item.id ? result.item : entry))
      if (result.run) {
        await runStream.resume(result.run, t('automations.run.attached', { name: taskName(result.run.task_id, tasks) }))
      } else {
        await load()
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : t('writingReview.error.confirm'))
    }
  }

  const dismissItem = async (item: AutomationInboxItem) => {
    setError('')
    try {
      const updated = await dismissAutomationInboxItem(item.id)
      setInboxItems((current) => current.map((entry) => entry.id === updated.id ? updated : entry))
    } catch (e) {
      setError(e instanceof Error ? e.message : t('writingReview.error.dismiss'))
    }
  }

  const readItem = async (item: AutomationInboxItem) => {
    setError('')
    try {
      const updated = await markAutomationInboxItemRead(item.id)
      setInboxItems((current) => current.map((entry) => entry.id === updated.id ? updated : entry))
    } catch (e) {
      setError(e instanceof Error ? e.message : t('writingReview.error.read'))
    }
  }

  const openRun = async (runID: string, item?: AutomationInboxItem) => {
    const run = findRunRecord(runID, activeRuns, recentRuns, item)
    if (!run) {
      setError(t('writingReview.error.missingRun'))
      return
    }
    setError('')
    try {
      if (run.status === 'running') {
        await runStream.resume(run, t('automations.run.attached', { name: taskName(run.task_id, tasks) }))
      } else {
        await runStream.loadHistory(run)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : t('writingReview.error.history'))
    }
  }

  const sendToWritingAgent = (item: AutomationInboxItem, evidence?: AutomationInboxItem['evidence'][number]) => {
    const target = evidence?.ref || selectedFile || ''
    const prompt = [
      t('writingReview.agentPrompt.title', { title: item.title }),
      item.summary,
      target ? t('writingReview.agentPrompt.target', { target }) : '',
      evidence?.snippet ? t('writingReview.agentPrompt.evidence', { evidence: evidence.snippet }) : '',
    ].filter(Boolean).join('\n\n')
    onSendToWritingAgent(prompt)
  }

  const latestRun = runStream.activeRun || activeReviewRuns[0]?.run || recentRuns[0] || null
  const canCheck = Boolean(primaryTask?.id)
  const canRun = Boolean(primaryTask?.id) && !runStream.isStreaming && selectedReviewPaths.length > 0
  const selectableChapterOptions = chapterOptions.filter((path) => !selectedReviewPaths.includes(path))

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      {error && <InlineErrorNotice className="mx-3 mt-2" message={error} title={t('automations.error')} />}

      <div className="flex min-h-0 flex-1 flex-col">
        <div className="shrink-0 border-b border-[var(--nova-border)] px-2 py-2">
          <div className="flex items-center gap-1.5">
            <div className="min-w-0 flex-1">
              <div className="flex min-w-0 items-center gap-1.5 text-xs font-medium text-[var(--nova-text)]">
                <span className="truncate">{primaryTask?.name || t('writingReview.noTaskTitle')}</span>
                {reviewInboxItems.length > 0 && (
                  <span className="shrink-0 rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
                    {t('writingReview.itemCount', { count: reviewInboxItems.length })}
                  </span>
                )}
              </div>
              <div className="mt-0.5 truncate text-[11px] leading-4 text-[var(--nova-text-faint)]">
                {primaryTask
                  ? t(primaryTask.enabled ? 'automations.enabled' : 'automations.disabled')
                  : t('writingReview.status.setup')}
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <button type="button" onClick={onOpenConfig} className="nova-nav-item flex h-7 w-7 items-center justify-center" aria-label={t('writingReview.configure')} title={t('writingReview.configure')}>
                <Settings className="h-3.5 w-3.5" />
              </button>
              <button type="button" onClick={() => void load()} className="nova-nav-item flex h-7 w-7 items-center justify-center" aria-label={t('writingReview.refresh')} title={t('writingReview.refresh')}>
                <RefreshCw className="h-3.5 w-3.5" />
              </button>
              {runStream.isStreaming ? (
                <button type="button" onClick={runStream.stop} className="nova-nav-item flex h-7 w-7 items-center justify-center rounded text-[11px]" aria-label={t('automations.stopRun')} title={t('automations.stopRun')}>
                  <Square className="h-3.5 w-3.5" />
                </button>
              ) : (
                <>
                  <button type="button" disabled={!canCheck} onClick={() => void checkTask()} className="nova-nav-item flex h-7 w-7 items-center justify-center rounded text-[11px] disabled:cursor-not-allowed disabled:opacity-45" aria-label={t('writingReview.checkNow')} title={t('writingReview.checkNow')}>
                    <Inbox className="h-3.5 w-3.5" />
                  </button>
                  <button type="button" disabled={!canRun} onClick={() => void runTask()} className="nova-nav-item flex h-7 w-7 items-center justify-center rounded border border-[var(--nova-border)] bg-[var(--nova-active)] text-[11px] disabled:cursor-not-allowed disabled:opacity-45" aria-label={t('writingReview.runNow')} title={t('writingReview.runNow')}>
                    <Play className="h-3.5 w-3.5" />
                  </button>
                </>
              )}
            </div>
          </div>
          <div className="mt-2 flex min-w-0 items-center gap-1.5">
            <select
              value=""
              onChange={(event) => {
                const path = event.target.value
                if (path) setSelectedReviewPaths((current) => current.includes(path) ? current : [...current, path])
              }}
              className="nova-field h-7 min-w-0 flex-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] text-[var(--nova-text-muted)] outline-none"
              aria-label={t('writingReview.chapterSelect')}
              title={t('writingReview.chapterSelect')}
            >
              <option value="">{selectedReviewPaths.length > 0 ? t('writingReview.addChapter') : t('writingReview.chooseChapter')}</option>
              {selectableChapterOptions.map((path) => (
                <option key={path} value={path}>{path}</option>
              ))}
            </select>
          </div>
          {selectedReviewPaths.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1">
              {selectedReviewPaths.map((path) => (
                <button
                  key={path}
                  type="button"
                  onClick={() => setSelectedReviewPaths((current) => current.filter((item) => item !== path))}
                  className="nova-nav-item max-w-full truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]"
                  title={t('writingReview.removeChapter', { path })}
                  aria-label={t('writingReview.removeChapter', { path })}
                >
                  {chapterTitle(path)} ×
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {loading ? (
            <div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-faint)]">{t('writingReview.loading')}</div>
          ) : runStream.messages.length > 0 || runStream.isStreaming ? (
            <MessageList
              messages={runStream.messages}
              isStreaming={runStream.isStreaming}
              activityContent={runStream.activityContent}
              scrollResetKey={runStream.activeRun?.id || latestRun?.id || 'review'}
              bottomPaddingClassName="pb-4"
            />
          ) : focusedItem ? (
            <ReviewInboxDetail
              item={focusedItem}
              taskName={taskName(focusedItem.task_id, tasks)}
              onRead={readItem}
              onConfirm={confirmItem}
              onDismiss={dismissItem}
              onOpenRun={openRun}
              onOpenFile={onOpenFile}
              onSendToWritingAgent={sendToWritingAgent}
            />
          ) : latestRun ? (
            <ReviewRunSummary run={latestRun} onOpen={() => void openRun(latestRun.id)} />
          ) : (
            <EmptyReviewState hasTask={Boolean(primaryTask)} onOpenConfig={onOpenConfig} />
          )}
        </div>

        <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)]">
          <InputArea
            onSend={(message) => void runStream.send(message)}
            onStop={runStream.stop}
            disabled={runStream.isStreaming || !runStream.activeRun}
            commandsEnabled={false}
            placeholder={t('writingReview.input.placeholder')}
            disabledPlaceholder={runStream.isStreaming ? t('writingReview.input.running') : t('writingReview.input.noRun')}
          />
        </div>
      </div>
    </div>
  )
}

export function WritingReviewTabBadge({
  workspace,
}: {
  workspace: string
}) {
  const [reviewCount, setReviewCount] = useState(0)

  useEffect(() => {
    let cancelled = false
    async function loadStatus() {
      if (!workspace) {
        setReviewCount(0)
        return
      }
      try {
        const [tasks, inbox] = await Promise.all([getAutomations(), getAutomationInbox()])
        const reviewTaskIDs = new Set(tasks.filter((task) => task.template === 'review').map((task) => task.id).filter(Boolean) as string[])
        if (cancelled) return
        setReviewCount(inbox.filter((item) => reviewTaskIDs.has(item.task_id) && (item.status === 'pending' || (item.status === 'auto_run' && !item.read_at))).length)
      } catch {
        if (!cancelled) {
          setReviewCount(0)
        }
      }
    }
    void loadStatus()
    const timer = window.setInterval(loadStatus, 30000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [workspace])

  if (reviewCount === 0) return null
  return <span className="rounded bg-[var(--nova-active)] px-1 text-[10px] text-[var(--nova-text)]">{reviewCount}</span>
}

function ReviewInboxDetail({
  item,
  taskName,
  onRead,
  onConfirm,
  onDismiss,
  onOpenRun,
  onOpenFile,
  onSendToWritingAgent,
}: {
  item: AutomationInboxItem
  taskName: string
  onRead: (item: AutomationInboxItem) => void | Promise<void>
  onConfirm: (item: AutomationInboxItem) => void | Promise<void>
  onDismiss: (item: AutomationInboxItem) => void | Promise<void>
  onOpenRun: (runID: string, item: AutomationInboxItem) => void | Promise<void>
  onOpenFile: (path: string) => void | Promise<void>
  onSendToWritingAgent: (item: AutomationInboxItem, evidence?: AutomationInboxItem['evidence'][number]) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-3 p-3 text-xs">
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
        <div className="flex items-start gap-2">
          <Inbox className="mt-0.5 h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-[var(--nova-text)]">{item.title}</span>
              <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{taskName}</span>
              <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{t(`automations.inbox.status.${item.status}`)}</span>
            </div>
            <div className="mt-2 whitespace-pre-wrap leading-5 text-[var(--nova-text-muted)]">{item.summary}</div>
            <div className="mt-2 text-[11px] text-[var(--nova-text-faint)]">{new Date(item.created_at).toLocaleString()}</div>
          </div>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          {!item.read_at && (
            <button type="button" onClick={() => void onRead(item)} className="nova-nav-item inline-flex items-center gap-1 rounded px-2 py-1 text-[var(--nova-text-muted)]">
              <Check className="h-3.5 w-3.5" />
              {t('automations.inbox.markRead')}
            </button>
          )}
          {(item.run_id || item.source_run_id) && (
            <button type="button" onClick={() => void onOpenRun(item.run_id || item.source_run_id || '', item)} className="nova-nav-item inline-flex items-center gap-1 rounded px-2 py-1 text-[var(--nova-text-muted)]">
              <MessageSquareText className="h-3.5 w-3.5" />
              {item.run_id ? t('automations.runs.viewTimeline') : t('automations.inbox.viewSourceRun')}
            </button>
          )}
          <button type="button" onClick={() => onSendToWritingAgent(item)} className="nova-nav-item inline-flex items-center gap-1 rounded px-2 py-1 text-[var(--nova-text-muted)]">
            <ExternalLink className="h-3.5 w-3.5" />
            {t('writingReview.sendToAgent')}
          </button>
          {item.status === 'pending' && item.action_policy === 'confirm' && (
            <button type="button" onClick={() => void onConfirm(item)} className="nova-nav-item inline-flex items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-active)] px-2 py-1 text-[var(--nova-text)]">
              <Play className="h-3.5 w-3.5" />
              {item.purpose === 'write_confirmation' ? t('automations.inbox.confirmWrite') : t('automations.inbox.confirmRun')}
            </button>
          )}
          {item.status === 'pending' && (
            <button type="button" onClick={() => void onDismiss(item)} className="nova-nav-item inline-flex items-center gap-1 rounded px-2 py-1 text-[var(--nova-text-muted)]">
              <X className="h-3.5 w-3.5" />
              {t('automations.inbox.dismiss')}
            </button>
          )}
        </div>
      </div>
      {item.evidence.length > 0 && (
        <div className="space-y-2">
          <div className="text-[11px] font-medium uppercase text-[var(--nova-text-faint)]">{t('writingReview.evidence')}</div>
          {item.evidence.map((evidence, index) => (
            <div key={`${item.id}-${index}`} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="truncate font-medium text-[var(--nova-text)]">{evidence.title || evidence.ref || evidence.source}</div>
                  <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">{evidence.source}{evidence.ref ? ` · ${evidence.ref}` : ''}</div>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {evidence.ref && (
                    <button type="button" onClick={() => void onOpenFile(evidence.ref || '')} className="nova-nav-item rounded px-1.5 py-1 text-[11px]">
                      {t('writingReview.openFile')}
                    </button>
                  )}
                  <button type="button" onClick={() => onSendToWritingAgent(item, evidence)} className="nova-nav-item rounded px-1.5 py-1 text-[11px]">
                    {t('writingReview.send')}
                  </button>
                </div>
              </div>
              {evidence.snippet && <div className="mt-2 line-clamp-4 whitespace-pre-wrap text-[11px] leading-4 text-[var(--nova-text-muted)]">{evidence.snippet}</div>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function ReviewRunSummary({ run, onOpen }: { run: AutomationRunRecord, onOpen: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="p-3 text-xs">
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
        <div className="font-medium text-[var(--nova-text)]">{t('writingReview.latestRun')}</div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{new Date(run.started_at).toLocaleString()} · {t(`writingReview.runStatus.${run.status}`)}</div>
        {run.summary && <div className="mt-2 line-clamp-6 whitespace-pre-wrap leading-5 text-[var(--nova-text-muted)]">{run.summary}</div>}
        {run.error && <div className="mt-2 text-[var(--nova-danger)]">{run.error}</div>}
        <button type="button" onClick={onOpen} className="nova-nav-item mt-3 inline-flex items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-active)] px-2 py-1 text-[var(--nova-text)]">
          <MessageSquareText className="h-3.5 w-3.5" />
          {t('automations.runs.viewTimeline')}
        </button>
      </div>
    </div>
  )
}

function EmptyReviewState({ hasTask, onOpenConfig }: { hasTask: boolean, onOpenConfig: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex h-full items-center justify-center p-5 text-center text-xs">
      <div className="max-w-72">
        <ClipboardCheck className="mx-auto h-6 w-6 text-[var(--nova-text-faint)]" />
        <div className="mt-3 font-medium text-[var(--nova-text)]">{hasTask ? t('writingReview.empty.title') : t('writingReview.noTaskTitle')}</div>
        <div className="mt-2 leading-5 text-[var(--nova-text-faint)]">{hasTask ? t('writingReview.empty.desc') : t('writingReview.noTaskDesc')}</div>
        <button type="button" onClick={onOpenConfig} className="nova-nav-item mt-3 inline-flex items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[var(--nova-text-muted)]">
          <Settings className="h-3.5 w-3.5" />
          {t('writingReview.configure')}
        </button>
      </div>
    </div>
  )
}

function taskName(taskID: string, tasks: AutomationTask[]) {
  return tasks.find((task) => task.id === taskID)?.name || taskID
}

function isChapterPath(path: string) {
  return path.startsWith('chapters/') && /\.(md|markdown|txt)$/i.test(path)
}

function chapterTitle(path: string) {
  const filename = path.split('/').pop() || path
  return filename.replace(/\.(md|markdown|txt)$/i, '')
}

function findRunRecord(runID: string, activeRuns: AutomationActiveRun[], recentRuns: AutomationRunRecord[], item?: AutomationInboxItem): AutomationRunRecord | null {
  const active = activeRuns.find((entry) => entry.run.id === runID)?.run
  if (active) return active
  const recent = recentRuns.find((run) => run.id === runID)
  if (recent) return recent
  if (!item) return null
  return {
    id: runID,
    task_id: item.task_id,
    scope: item.scope,
    workspace: item.workspace,
    trigger: item.purpose === 'write_confirmation' ? 'write_confirmation' : 'condition',
    source_run_id: item.source_run_id,
    trigger_evidence: item.evidence,
    status: item.status === 'pending' ? 'running' : 'success',
    started_at: item.created_at,
    finished_at: item.handled_at,
    summary: item.summary,
    tool_manifest: [],
  }
}
