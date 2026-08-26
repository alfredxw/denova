import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Plus, Stethoscope } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { WRITING_COMPOSER_SETTING_DEFAULTS } from '@/components/Chat/AgentPanel'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { AgentChatConversationTab } from '@/features/agent-chat/AgentChatConversationTab'
import {
  createAgentChatSession,
  getAgentChatProjects,
  type AgentChatProject,
} from '@/features/agent-chat/api'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'

const SCHEDULED_SESSION_ID = 'harness-scheduled'

interface HarnessAgentPanelProps {
  evidence?: string[]
  evidenceControl?: ReactNode
  onSettled: () => void
}

/** Compact trajectory-side entry into the standard multi-session Harness Project Agent. */
export function HarnessAgentPanel({ evidence, evidenceControl, onSettled }: HarnessAgentPanelProps) {
  const { t } = useTranslation()
  const [project, setProject] = useState<AgentChatProject | null>(null)
  const [sessionID, setSessionID] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [pendingAction, setPendingAction] = useState<{ id: string; message: string; displayMessage: string } | null>(null)
  const runningSessionsRef = useRef(new Map<string, boolean>())

  const sessions = useMemo(
    () => (project?.sessions ?? []).filter((session) => session.id !== SCHEDULED_SESSION_ID),
    [project?.sessions],
  )
  const composerSettings = usePersistedUserSettings({
    workspace: project?.path ?? '',
    defaults: WRITING_COMPOSER_SETTING_DEFAULTS,
  })

  const refresh = useCallback(async () => {
    const projects = await getAgentChatProjects()
    const harness = projects.find((candidate) => candidate.type === 'harness') ?? null
    if (!harness) throw new Error(t('continualLearning.agent.unavailable'))
    const visible = harness.sessions.filter((session) => session.id !== SCHEDULED_SESSION_ID)
    setProject(harness)
    setSessionID((current) => visible.some((session) => session.id === current) ? current : (visible[0]?.id ?? ''))
    return { harness, visible }
  }, [t])

  const createConversation = useCallback(async () => {
    if (!project || creating) return
    setCreating(true)
    setError('')
    try {
      const created = await createAgentChatSession(project.id)
      setProject((current) => current ? { ...current, total: current.total + 1, sessions: [created, ...current.sessions] } : current)
      setSessionID(created.id)
    } catch (cause) {
      console.warn('[features/trajectory/HarnessAgentPanel.tsx] creating conversation failed', { error: cause })
      setError(t('continualLearning.agent.createFailed'))
    } finally {
      setCreating(false)
    }
  }, [creating, project, t])

  useEffect(() => {
    let alive = true
    setLoading(true)
    void refresh()
      .then(async ({ harness, visible }) => {
        if (!alive || visible.length > 0) return
        const created = await createAgentChatSession(harness.id)
        if (!alive) return
        setProject((current) => current ? { ...current, total: current.total + 1, sessions: [created, ...current.sessions] } : current)
        setSessionID(created.id)
      })
      .catch((cause) => {
        console.warn('[features/trajectory/HarnessAgentPanel.tsx] loading Harness Project failed', { error: cause })
        if (alive) setError(t('continualLearning.agent.loadFailed'))
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => { alive = false }
  }, [refresh])

  useEffect(() => {
    const observed = runningSessionsRef.current.get(sessionID)
    setRunning(observed ?? sessions.find((session) => session.id === sessionID)?.running ?? false)
  }, [sessionID, sessions])

  const handleRunningChange = useCallback((_projectID: string, changedSessionID: string, next: boolean | null) => {
    if (next == null) {
      runningSessionsRef.current.delete(changedSessionID)
      return
    }
    const previous = runningSessionsRef.current.get(changedSessionID) === true
    runningSessionsRef.current.set(changedSessionID, next)
    if (changedSessionID === sessionID) setRunning(next)
    if (previous && !next) {
      onSettled()
      void refresh().catch((cause) => console.warn('[features/trajectory/HarnessAgentPanel.tsx] refreshing sessions failed', { error: cause }))
    }
  }, [onSettled, refresh, sessionID])

  const diagnose = useCallback(() => {
    if (!sessionID || running) return
    setPendingAction({
      id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
      message: diagnosisPrompt(evidence),
      displayMessage: t('continualLearning.agent.diagnoseDisplay'),
    })
  }, [evidence, running, sessionID, t])

  let conversationContent: ReactNode = null
  if (project && sessionID) {
    conversationContent = (
      <AgentChatConversationTab
        key={sessionID}
        projectId={project.id}
        projectType="harness"
        workspace={project.path}
        sessionId={sessionID}
        active
        composerSettings={composerSettings}
        tellers={[]}
        imagePresets={[]}
        pendingAction={pendingAction}
        onPendingActionConsumed={(id) => {
          setPendingAction((current) => current?.id === id ? null : current)
        }}
        onRunningChange={handleRunningChange}
        onWorkspaceChanged={() => onSettled()}
      />
    )
  } else if (loading) {
    conversationContent = <div className="grid h-full place-items-center text-xs text-[var(--nova-text-faint)]">{t('common.loading')}</div>
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <div className="flex min-h-12 flex-wrap items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs font-medium text-[var(--nova-text)]">{t('continualLearning.agent.title')}</div>
          <div className="truncate text-[10px] text-[var(--nova-text-faint)]">{t('continualLearning.agent.subtitle')}</div>
        </div>
        {evidenceControl}
        <Button type="button" size="xs" variant="outline" disabled={!sessionID || running || loading} onClick={diagnose}>
          <Stethoscope />{t('continualLearning.agent.diagnose')}
        </Button>
      </div>
      <div className="flex min-h-10 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-1.5">
        <Select value={sessionID} onValueChange={setSessionID} disabled={loading || sessions.length === 0}>
          <SelectTrigger size="sm" className="min-w-0 flex-1">
            <SelectValue placeholder={t('continualLearning.agent.session')} />
          </SelectTrigger>
          <SelectContent>
            {sessions.map((session) => (
              <SelectItem key={session.id} value={session.id}>{session.title || session.id}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button type="button" size="icon-xs" variant="ghost" disabled={!project || creating} onClick={() => void createConversation()} aria-label={t('continualLearning.agent.newSession')}>
          <Plus />
        </Button>
      </div>
      {error ? <div className="border-b border-[var(--nova-border)] px-3 py-2 text-xs text-red-400">{error}</div> : null}
      <div className="min-h-0 flex-1">
        {conversationContent}
      </div>
    </div>
  )
}

function diagnosisPrompt(evidence?: string[]) {
  const scope = evidence?.length
    ? `Use only these selected trajectory resources as evidence:\n- ${evidence.join('\n- ')}`
    : 'Start with trajectory://index and select the most relevant recent run or session evidence.'
  return `[Agent Health Diagnosis]

${scope}

Inspect the complete relevant trajectory resources and harness://state/current. Diagnose concrete Agent Health issues, separating symptoms, likely causes, and evidence-backed recommendations. Check tool failures, repeated retries, context pressure, instruction conflicts, invalid Harness State, and missed user intent. This is diagnosis only: do not modify workspace files unless the user explicitly asks in a follow-up. Report healthy areas as well as problems, and say when evidence is insufficient.`
}
