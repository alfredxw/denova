import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Bot, LoaderCircle, Plus } from 'lucide-react'
import { nanoid } from 'nanoid'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import type { AgentPanelProps, AgentPanelView } from '@/components/Chat/AgentPanel'
import {
  AgentChatConversationTab,
  type AgentChatConversationHost,
  type AgentChatPendingAction,
  type AgentChatConversationState,
} from './AgentChatConversationTab'
import {
  createAgentChatSession,
  deleteAgentChatSession,
  AGENT_CHAT_PROJECT_UPDATED_EVENT,
  getAgentChatProjects,
  renameAgentChatSession,
  type AgentChatSession,
  type AgentChatSessionChannel,
  type AgentChatProjectType,
} from './api'
import { readAgentChatActiveSession, writeAgentChatActiveSession } from './session-preferences'
import { draftSessionTitle } from './tab-state'
import { WritingSessionRail } from './WritingSessionRail'

export const WRITING_SESSION_RAIL_STORAGE_KEY = 'nova.writingAgent.sessionRailVisible.v1'

/**
 * Project-scoped Writing Agent host. Every mounted conversation owns an immutable transport,
 * so changing the visible session never aborts a background run in the same Book.
 */
type RequiredWorkspaceProps = Pick<AgentPanelProps,
  | 'projectId'
  | 'workspace'
  | 'composerSettings'
  | 'tellers'
  | 'selectedFile'
  | 'references'
  | 'loreReferences'
  | 'loreReferenceLabels'
  | 'loreSuggestions'
  | 'styleScenes'
  | 'textSelections'
  | 'fileSuggestions'
  | 'onReferenceRemove'
  | 'onLoreReferenceRemove'
  | 'onStyleSceneRemove'
  | 'onTextSelectionRemove'
>

export type WritingAgentWorkspaceProps = RequiredWorkspaceProps & Partial<AgentPanelProps> & {
  projectType?: AgentChatProjectType
  sessionChannel?: AgentChatSessionChannel
  pendingAction?: AgentChatPendingAction | null
  onPendingActionConsumed?: (id: string) => void
  messageTransform?: (message: string) => string
  onSettled?: () => void
  onConversationStateChange?: (state: AgentChatConversationState) => void
}

type WorkspaceSession = AgentChatSession & { draft?: boolean }

export function WritingAgentWorkspace(props: WritingAgentWorkspaceProps) {
  const { t } = useTranslation()
  const sessionChannel = props.sessionChannel ?? 'agent'
  const [sessions, setSessions] = useState<WorkspaceSession[]>([])
  const [activeSessionId, setActiveSessionId] = useState('')
  const [mountedSessionIds, setMountedSessionIds] = useState<string[]>([])
  const [view, setView] = useState<AgentPanelView>('chat')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sessionPending, setSessionPending] = useState(false)
  const [internalRailVisible, setInternalRailVisible] = useState(readWritingSessionRailVisibility)
  const railVisible = props.sessionRailVisible ?? internalRailVisible
  const sessionsRef = useRef(sessions)
  sessionsRef.current = sessions

  const storeActiveSession = useCallback((sessionId: string) => {
    writeAgentChatActiveSession(props.projectId, sessionId, sessionChannel)
  }, [props.projectId, sessionChannel])

  const selectSession = useCallback((sessionId: string) => {
    if (!sessionId || !sessionsRef.current.some((session) => session.id === sessionId)) return
    setActiveSessionId(sessionId)
    setMountedSessionIds((current) => current.includes(sessionId) ? current : [...current, sessionId])
    setView('chat')
    storeActiveSession(sessionId)
  }, [storeActiveSession])

  const refreshSessions = useCallback(async () => {
    const projects = await getAgentChatProjects(sessionChannel === 'agent' ? undefined : { channel: sessionChannel })
    const project = projects.find((candidate) => candidate.id === props.projectId)
    if (!project) throw new Error(`Writing Agent Project is unavailable: ${props.projectId}`)
    const persistedIDs = new Set(project.sessions.map((session) => session.id))
    const drafts = sessionsRef.current.filter((session) => session.draft && !persistedIDs.has(session.id))
    const nextSessions = sortSessions([...drafts, ...project.sessions])
    sessionsRef.current = nextSessions
    setSessions(nextSessions)
    setActiveSessionId((current) => {
      const stored = readAgentChatActiveSession(props.projectId, sessionChannel)
      const next = firstAvailableSession(nextSessions, current, stored, props.activeSessionId)
      if (next) storeActiveSession(next)
      return next
    })
    setMountedSessionIds((current) => {
      const available = new Set(nextSessions.map((session) => session.id))
      const retained = current.filter((sessionId) => available.has(sessionId))
      const selected = firstAvailableSession(nextSessions, activeSessionId, readAgentChatActiveSession(props.projectId, sessionChannel), props.activeSessionId)
      return uniqueStrings([
        ...retained,
        ...nextSessions.filter((session) => session.running).map((session) => session.id),
        selected,
      ])
    })
    setError('')
    return nextSessions
  }, [activeSessionId, props.activeSessionId, props.projectId, sessionChannel, storeActiveSession])

  const createDraftSession = useCallback((title = '') => {
    const existing = sessionsRef.current.find((session) => session.draft)
    if (existing) {
      selectSession(existing.id)
      return existing
    }
    const now = new Date().toISOString()
    const draft: WorkspaceSession = {
      id: `s-${nanoid()}`,
      channel: sessionChannel,
      title: title.trim() || t('chat.newSession'),
      created_at: now,
      updated_at: now,
      message_count: 0,
      running: false,
      active: false,
      draft: true,
    }
    const next = sortSessions([draft, ...sessionsRef.current])
    sessionsRef.current = next
    setSessions(next)
    setActiveSessionId(draft.id)
    setMountedSessionIds((current) => current.includes(draft.id) ? current : [...current, draft.id])
    setView('chat')
    storeActiveSession(draft.id)
    return draft
  }, [selectSession, sessionChannel, storeActiveSession, t])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    setSessions([])
    sessionsRef.current = []
    setActiveSessionId('')
    setMountedSessionIds([])
    if (!props.projectId.trim()) return () => { cancelled = true }
    void refreshSessions()
      .then((nextSessions) => {
        if (!cancelled && nextSessions.length === 0) {
          createDraftSession()
        }
      })
      .catch((loadError) => {
        if (cancelled) return
        console.error(
          `[features/agent-chat/WritingAgentWorkspace.tsx] loading Book conversations failed project_id=${props.projectId} error=${errorMessage(loadError)}`,
          loadError,
        )
        setError(loadError instanceof Error ? loadError.message : String(loadError))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [props.projectId, sessionChannel]) // Project or list channel changes reset every mounted transport.

  useEffect(() => {
    const onProjectUpdated = (event: Event) => {
      const detail = (event as CustomEvent<{ projectId?: string }>).detail
      if (detail?.projectId !== props.projectId) return
      void refreshSessions().catch((refreshError) => {
        console.error(
          `[features/agent-chat/WritingAgentWorkspace.tsx] refreshing externally updated conversation failed project_id=${props.projectId} error=${errorMessage(refreshError)}`,
          refreshError,
        )
      })
    }
    window.addEventListener(AGENT_CHAT_PROJECT_UPDATED_EVENT, onProjectUpdated)
    return () => window.removeEventListener(AGENT_CHAT_PROJECT_UPDATED_EVENT, onProjectUpdated)
  }, [props.projectId, refreshSessions])

  const createSession = useCallback(async (title?: string, customAgentId?: string) => {
    if (sessionPending) return
    if (customAgentId === undefined) {
      createDraftSession(title)
      return
    }
    setSessionPending(true)
    try {
      const created = await createAgentChatSession(props.projectId, title ?? '', customAgentId, sessionChannel)
      const next = sortSessions([
        created,
        ...sessionsRef.current.filter((session) => (
          session.id !== created.id && !(session.draft && session.id === activeSessionId)
        )),
      ])
      sessionsRef.current = next
      setSessions(next)
      setActiveSessionId(created.id)
      setMountedSessionIds((current) => current.includes(created.id) ? current : [...current, created.id])
      setView('chat')
      storeActiveSession(created.id)
    } catch (createError) {
      console.error(
        `[features/agent-chat/WritingAgentWorkspace.tsx] creating Book conversation failed project_id=${props.projectId} error=${errorMessage(createError)}`,
        createError,
      )
      toast.error(t('chat.sessionRail.createFailed'), {
        description: createError instanceof Error ? createError.message : String(createError),
      })
    } finally {
      setSessionPending(false)
    }
  }, [activeSessionId, createDraftSession, props.projectId, sessionChannel, sessionPending, storeActiveSession, t])

  const renameSession = useCallback(async (sessionId: string, title: string) => {
    const target = sessionsRef.current.find((session) => session.id === sessionId)
    if (target?.draft) {
      const next = sessionsRef.current.map((session) => session.id === sessionId ? { ...session, title } : session)
      sessionsRef.current = next
      setSessions(next)
      return
    }
    await renameAgentChatSession(props.projectId, sessionId, title)
    const next = sessionsRef.current.map((session) => session.id === sessionId ? { ...session, title } : session)
    sessionsRef.current = next
    setSessions(next)
  }, [props.projectId])

  const deleteSession = useCallback(async (sessionId: string) => {
    const target = sessionsRef.current.find((session) => session.id === sessionId)
    if (!target || target.running || sessionsRef.current.length <= 1) return
    if (!target.draft) await deleteAgentChatSession(props.projectId, sessionId)
    const remaining = sessionsRef.current.filter((session) => session.id !== sessionId)
    sessionsRef.current = remaining
    setSessions(remaining)
    setMountedSessionIds((current) => current.filter((id) => id !== sessionId))
    if (activeSessionId === sessionId) {
      const next = remaining[0]?.id || ''
      setActiveSessionId(next)
      if (next) storeActiveSession(next)
    }
  }, [activeSessionId, props.projectId, storeActiveSession])

  const commitDraftSession = useCallback((sessionId: string, message: string) => {
    const now = new Date().toISOString()
    const next = sessionsRef.current.map((session) => session.id === sessionId
      ? {
          ...session,
          draft: undefined,
          title: draftSessionTitle(message) || session.title,
          updated_at: now,
          message_count: Math.max(session.message_count, 1),
        }
      : session)
    sessionsRef.current = next
    setSessions(next)
    storeActiveSession(sessionId)
  }, [storeActiveSession])

  const handleRunningChange = useCallback((_projectId: string, sessionId: string, running: boolean | null) => {
    if (running === null) return
    const settled = Boolean(sessionsRef.current.find((session) => session.id === sessionId)?.running && !running)
    const nextSessions = sessionsRef.current.map((session) => {
      if (session.id !== sessionId) return session
      return { ...session, running }
    })
    sessionsRef.current = nextSessions
    setSessions(nextSessions)
    if (settled) {
      props.onSettled?.()
      void refreshSessions().catch((refreshError) => {
        console.error(
          `[features/agent-chat/WritingAgentWorkspace.tsx] refreshing settled conversation failed project_id=${props.projectId} session_id=${sessionId} error=${errorMessage(refreshError)}`,
          refreshError,
        )
      })
    }
  }, [props.onSettled, props.projectId, refreshSessions])

  const setSessionRailVisible = useCallback((visible: boolean) => {
    if (props.onSessionRailVisibleChange) {
      props.onSessionRailVisibleChange(visible)
      return
    }
    setInternalRailVisible(visible)
    writeWritingSessionRailVisibility(visible)
  }, [props.onSessionRailVisibleChange])

  const retrySessions = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      await refreshSessions()
    } catch (loadError) {
      console.error(
        `[features/agent-chat/WritingAgentWorkspace.tsx] retrying Book conversations failed project_id=${props.projectId} error=${errorMessage(loadError)}`,
        loadError,
      )
      setError(loadError instanceof Error ? loadError.message : String(loadError))
    } finally {
      setLoading(false)
    }
  }, [props.projectId, refreshSessions])

  const host = useMemo<AgentChatConversationHost>(() => ({
    chrome: props.chrome ?? 'panel',
    view,
    onViewChange: setView,
    sessions,
    sessionTransitionPending: sessionPending,
    sessionActionsDisabled: sessionPending,
    sessionRailVisible: railVisible,
    onSessionRailVisibleChange: setSessionRailVisible,
    onCreateSession: createSession,
    onSwitchSession: selectSession,
    onRenameSession: renameSession,
    onDeleteSession: deleteSession,
    quickPromptScope: props.quickPromptScope,
    composerDraftScope: props.composerDraftScope,
    currentChapter: props.currentChapter,
    selectedFile: props.selectedFile,
    ideContext: props.ideContext,
    fileSuggestions: props.fileSuggestions,
    loreReferenceLabels: props.loreReferenceLabels,
    loreSuggestions: props.loreSuggestions,
    onInsertIllustration: props.onInsertIllustration,
    onSubAgentDetailsChange: props.onSubAgentDetailsChange,
    composerContext: {
      references: props.references,
      loreReferences: props.loreReferences,
      styleScenes: props.styleScenes,
      textSelections: props.textSelections,
      onReferenceConsumed: props.onReferenceRemove,
      onLoreReferenceConsumed: props.onLoreReferenceRemove,
      onStyleSceneConsumed: props.onStyleSceneRemove,
      onTextSelectionConsumed: props.onTextSelectionRemove,
    },
  }), [
    createSession,
    deleteSession,
    props.chrome,
    props.composerDraftScope,
    props.currentChapter,
    props.fileSuggestions,
    props.ideContext,
    props.loreReferenceLabels,
    props.loreReferences,
    props.loreSuggestions,
    props.onInsertIllustration,
    props.onLoreReferenceRemove,
    props.onReferenceRemove,
    props.onStyleSceneRemove,
    props.onSubAgentDetailsChange,
    props.onTextSelectionRemove,
    props.quickPromptScope,
    props.references,
    props.selectedFile,
    props.styleScenes,
    props.textSelections,
    railVisible,
    renameSession,
    selectSession,
    sessionPending,
    sessions,
    setSessionRailVisible,
    view,
  ])

  const mountedSessions = mountedSessionIds
    .map((sessionId) => sessions.find((session) => session.id === sessionId))
    .filter((session): session is WorkspaceSession => Boolean(session))

  useEffect(() => {
    if (!activeSessionId) props.onConversationStateChange?.({ messages: [], isStreaming: false })
  }, [activeSessionId, props.onConversationStateChange])

  return (
    <div className="flex h-full min-h-0 min-w-0 bg-[var(--nova-bg)]">
      <div className="relative min-h-0 min-w-0 flex-1">
        {loading ? (
          <div role="status" className="flex h-full items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]">
            <LoaderCircle className="size-4 animate-spin" />
            {t('router.loading')}
          </div>
        ) : error ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
            <Bot className="size-5 text-[var(--nova-text-muted)]" />
            <div className="text-xs text-[var(--nova-text)]">{t('chat.sessionRail.loadFailed')}</div>
            <div className="max-w-72 break-words text-[10px] text-[var(--nova-text-faint)]">{error}</div>
            <Button type="button" variant="outline" size="xs" onClick={() => void retrySessions()}>{t('common.retry')}</Button>
          </div>
        ) : mountedSessions.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
            <Bot className="size-5 text-[var(--nova-text-muted)]" />
            <div className="text-xs text-[var(--nova-text-faint)]">{t('chat.noSession')}</div>
            <Button type="button" variant="outline" size="sm" onClick={() => void createSession()} disabled={sessionPending}>
              <Plus data-icon="inline-start" />
              {t('chat.newSession')}
            </Button>
          </div>
        ) : mountedSessions.map((session) => {
          const active = session.id === activeSessionId
          return (
            <section
              key={session.id}
              hidden={!active}
              aria-hidden={!active}
              className="absolute inset-0 min-h-0 min-w-0"
            >
              <AgentChatConversationTab
                projectId={props.projectId}
                projectType={props.projectType ?? 'book'}
                workspace={props.workspace}
                sessionId={session.id}
                sessionChannel={session.channel}
                syncRevision={`${session.updated_at}:${session.message_count}:${session.running ? 'running' : 'idle'}`}
                draft={session.draft}
                active={props.active !== false && active}
                composerSettings={props.composerSettings}
                tellers={props.tellers}
                imagePresets={props.imagePresets || []}
                reviewFeedback={active ? props.reviewFeedback : null}
                onReviewFeedbackOpen={active ? props.onReviewFeedbackOpen : undefined}
                onReviewFeedbackRemove={active ? props.onReviewFeedbackRemove : undefined}
                onReviewFeedbackSubmitted={active ? props.onReviewFeedbackSubmitted : undefined}
                onReviewFeedbackSubmissionFailed={active ? props.onReviewFeedbackSubmissionFailed : undefined}
                onOpenChangeReview={props.onOpenChangeReview}
                onWorkspaceChanged={(_workspace, paths) => props.onWorkspaceChanged?.(paths)}
                onRunningChange={handleRunningChange}
                onDraftCommitted={(message) => commitDraftSession(session.id, message)}
                onConversationStateChange={active ? props.onConversationStateChange : undefined}
                pendingAction={active ? props.pendingAction : null}
                onPendingActionConsumed={active ? props.onPendingActionConsumed : undefined}
                messageTransform={props.messageTransform}
                host={host}
              />
            </section>
          )
        })}
      </div>

      {railVisible ? (
        <WritingSessionRail
          sessions={sessions}
          activeSessionId={activeSessionId}
          onSwitch={selectSession}
          onVisibleChange={setSessionRailVisible}
        />
      ) : null}
    </div>
  )
}

function firstAvailableSession(sessions: WorkspaceSession[], ...preferredIds: Array<string | undefined>) {
  for (const preferredId of preferredIds) {
    if (preferredId && sessions.some((session) => session.id === preferredId)) return preferredId
  }
  return sessions.find((session) => session.active)?.id || sessions[0]?.id || ''
}

function sortSessions(sessions: WorkspaceSession[]) {
  return [...sessions].sort((left, right) => {
    return Date.parse(right.updated_at || right.created_at || '') - Date.parse(left.updated_at || left.created_at || '')
  })
}

function uniqueStrings(values: string[]) {
  return [...new Set(values.filter(Boolean))]
}

export function readWritingSessionRailVisibility() {
  if (typeof window === 'undefined') return true
  return window.localStorage.getItem(WRITING_SESSION_RAIL_STORAGE_KEY) !== 'false'
}

export function writeWritingSessionRailVisibility(visible: boolean) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(WRITING_SESSION_RAIL_STORAGE_KEY, String(visible))
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}
