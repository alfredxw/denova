import { useCallback, useEffect, useMemo, useState } from 'react'
import { Bot, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { WRITING_COMPOSER_SETTING_DEFAULTS } from '@/components/Chat/AgentPanel'
import { Button } from '@/components/ui/button'
import { WritingAgentWorkspace } from '@/features/agent-chat/WritingAgentWorkspace'
import { getAgentChatProjects, type AgentChatProject } from '@/features/agent-chat/api'
import { buildConfigurationAgentMessage, type ConfigurationPageContext } from '@/features/agent-chat/configuration-message'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'

interface ConfigManagerChatProps {
  projectId: string
  origin: string
  resourceId?: string
  storyId?: string
  branchId?: string
  context?: Record<string, string>
  initialInstruction?: string
  initialInstructionKey?: string
  onInitialInstructionAccepted?: () => void
  onMutated?: () => void
  className?: string
}

const EMPTY_STRINGS: string[] = []
const EMPTY_TELLERS: never[] = []
const EMPTY_IMAGE_PRESETS: never[] = []
const EMPTY_LORE_SUGGESTIONS: never[] = []
const EMPTY_TEXT_SELECTIONS: never[] = []
const EMPTY_LABELS: Record<string, string> = {}

/**
 * Keeps Configuration Manager as a visible page surface while every turn uses the
 * ordinary Project Agent session, transport, history, recovery, and settings.
 */
export function ConfigManagerChat({
  projectId,
  origin,
  resourceId,
  storyId,
  branchId,
  context,
  initialInstruction,
  initialInstructionKey,
  onInitialInstructionAccepted,
  onMutated,
  className = '',
}: ConfigManagerChatProps) {
  const { t } = useTranslation()
  const [project, setProject] = useState<AgentChatProject | null>(null)
  const [loading, setLoading] = useState(Boolean(projectId.trim()))
  const [error, setError] = useState('')
  const [acceptedInitialInstructionKey, setAcceptedInitialInstructionKey] = useState('')
  const composerSettings = usePersistedUserSettings({
    workspace: project?.path || '',
    defaults: WRITING_COMPOSER_SETTING_DEFAULTS,
  })

  const fetchProject = useCallback(async (): Promise<AgentChatProject> => {
    const projects = await getAgentChatProjects()
    const match = projects.find((candidate) => candidate.id === projectId)
    if (!match) throw new Error(`Project Agent is unavailable: ${projectId}`)
    return match
  }, [projectId])
  const retryProject = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setProject(await fetchProject())
    } catch (loadError) {
      console.error(
        `[components/Chat/ConfigManagerChat.tsx] retrying Project Agent failed project_id=${projectId} error=${errorMessage(loadError)}`,
        loadError,
      )
      setError(errorMessage(loadError))
    } finally {
      setLoading(false)
    }
  }, [fetchProject, projectId])

  useEffect(() => {
    let cancelled = false
    setAcceptedInitialInstructionKey('')
    setProject(null)
    setError('')
    setLoading(Boolean(projectId.trim()))
    if (!projectId.trim()) return () => { cancelled = true }
    void fetchProject()
      .then((match) => {
        if (!cancelled) setProject(match)
      })
      .catch((loadError) => {
        if (cancelled) return
        console.error(
          `[components/Chat/ConfigManagerChat.tsx] loading Project Agent failed project_id=${projectId} error=${errorMessage(loadError)}`,
          loadError,
        )
        setError(errorMessage(loadError))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [fetchProject, projectId])

  const pageContext = useMemo<ConfigurationPageContext>(() => ({
    origin,
    resourceId,
    storyId,
    branchId,
    context,
  }), [branchId, context, origin, resourceId, storyId])
  const messageTransform = useCallback(
    (message: string) => buildConfigurationAgentMessage(message, pageContext),
    [pageContext],
  )
  const pendingAction = useMemo(() => {
    const message = initialInstruction?.trim() || ''
    const id = initialInstructionKey?.trim() || ''
    if (!message || !id || acceptedInitialInstructionKey === id) return null
    return { id, message, displayMessage: message }
  }, [acceptedInitialInstructionKey, initialInstruction, initialInstructionKey])
  const consumePendingAction = useCallback((id: string) => {
    setAcceptedInitialInstructionKey(id)
    onInitialInstructionAccepted?.()
  }, [onInitialInstructionAccepted])

  if (loading) {
    return (
      <div role="status" className={`flex h-full items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)] ${className}`}>
        <LoaderCircle className="size-4 animate-spin" />
        {t('router.loading')}
      </div>
    )
  }
  if (!project || error) {
    return (
      <div className={`flex h-full flex-col items-center justify-center gap-3 px-6 text-center ${className}`}>
        <Bot className="size-5 text-[var(--nova-text-muted)]" />
        <div className="text-xs text-[var(--nova-text)]">{t('chat.sessionRail.loadFailed')}</div>
        {error ? <div className="max-w-72 break-words text-[10px] text-[var(--nova-text-faint)]">{error}</div> : null}
        <Button type="button" variant="outline" size="xs" onClick={() => void retryProject()}>{t('common.retry')}</Button>
      </div>
    )
  }

  return (
    <div className={`h-full min-h-0 overflow-hidden ${className}`}>
      <WritingAgentWorkspace
        projectId={project.id}
        projectType={project.type}
        workspace={project.path}
        composerSettings={composerSettings}
        tellers={EMPTY_TELLERS}
        imagePresets={EMPTY_IMAGE_PRESETS}
        selectedFile={null}
        references={EMPTY_STRINGS}
        loreReferences={EMPTY_STRINGS}
        loreReferenceLabels={EMPTY_LABELS}
        loreSuggestions={EMPTY_LORE_SUGGESTIONS}
        styleScenes={EMPTY_STRINGS}
        textSelections={EMPTY_TEXT_SELECTIONS}
        fileSuggestions={EMPTY_STRINGS}
        onReferenceRemove={noop}
        onLoreReferenceRemove={noop}
        onStyleSceneRemove={noop}
        onTextSelectionRemove={noop}
        sessionRailVisible={false}
        pendingAction={pendingAction}
        onPendingActionConsumed={consumePendingAction}
        messageTransform={messageTransform}
        onSettled={onMutated}
      />
    </div>
  )
}

function noop(): void {}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error || 'Unknown error')
}
