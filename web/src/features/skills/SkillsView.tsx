import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Bot, RefreshCw, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { Button } from '@/components/ui/button'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { deleteSkillDocument, getSkillDocument, getSkillFileDocument, getSkills, saveSkillDocument, saveSkillFileDocument } from '@/lib/api'
import { isRevisionConflict, saveWithRevisionRecovery } from '@/lib/revision-conflict'
import { rebaseTextWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import type { SkillDocument, SkillFileDocument, SkillInstallResult, SkillScope, SkillSnapshot, SkillSummary } from '@/lib/api'
import { SkillConfigPanel, type SkillConfigPanelHandle } from './SkillConfigPanel'
import { SkillCreatePanel } from './SkillCreatePanel'
import { SkillEditor } from './SkillEditor'
import { SkillInstallPanel } from './SkillInstallPanel'
import { SkillListPanel } from './SkillListPanel'
import { keyOf, preferredBuiltinOverrideScope, scopeLabel, skillEntryFile, skillFilePath, type SkillContentViewMode, type SkillsMode } from './skill-utils'

interface SkillsViewProps {
  workspace: string
  onClose?: () => void
}

interface SkillContentAutosaveDraft {
  id: string
  updated_at?: string
  scope: SkillScope
  name: string
  path: string
  content: string
}

interface SkillContentAutosaveSaved extends SkillContentAutosaveDraft {
  document?: SkillDocument
  fileDocument?: SkillFileDocument
}

let nextSkillsViewSourceID = 1

/** Delete/restore snapshot the display values so later state changes cannot alter dialog copy. */
type ConfirmRequest =
  | { kind: 'delete'; name: string }
  | { kind: 'restore'; name: string; scope: string }

function skillContentDraft(
  document: SkillDocument,
  path: string,
  content: string,
  updatedAt = document.revision,
): SkillContentAutosaveDraft {
  return {
    id: `${document.scope}:${document.name}:${path}`,
    updated_at: updatedAt,
    scope: document.scope,
    name: document.name,
    path,
    content,
  }
}

function skillContentSignature(value: Partial<SkillContentAutosaveDraft>) {
  return `${value.scope || ''}\u0000${value.name || ''}\u0000${value.path || ''}\u0000${value.content || ''}`
}

function skillSummaryOf(value: SkillSummary): SkillSummary {
  const { name, description, context, agent, model, scope, path, editable, active, updated_at } = value
  return { name, description, context, agent, model, scope, path, editable, active, updated_at }
}

export function SkillsView({ workspace, onClose }: SkillsViewProps) {
  const { t } = useTranslation()
  const [snapshot, setSnapshot] = useState<SkillSnapshot>({ scopes: [], skills: [] })
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [document, setDocument] = useState<SkillDocument | null>(null)
  const [draft, setDraft] = useState('')
  const [selectedFilePath, setSelectedFilePath] = useState(skillEntryFile)
  const [fileDocument, setFileDocument] = useState<SkillFileDocument | null>(null)
  const [fileDraft, setFileDraft] = useState('')
  const [contentViewMode, setContentViewMode] = useState<SkillContentViewMode>('preview')
  const [fileTreeOpen, setFileTreeOpen] = useState(false)
  const [fileLoading, setFileLoading] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<SkillsMode>('editor')
  const [agentOpen, setAgentOpen] = useState(false)
  const [confirmRequest, setConfirmRequest] = useState<ConfirmRequest | null>(null)
  const [documentReloadVersion, setDocumentReloadVersion] = useState(0)
  const [eventSource] = useState(() => `skills-view-${nextSkillsViewSourceID++}`)
  const configPanelRef = useRef<SkillConfigPanelHandle | null>(null)
  const notifySkillsUpdated = useCallback(() => {
    window.dispatchEvent(new CustomEvent('nova:skills-updated', { detail: { source: eventSource } }))
  }, [eventSource])

  const selectedSkill = useMemo(() => snapshot.skills.find((skill) => keyOf(skill) === selectedKey) ?? null, [selectedKey, snapshot.skills])
  const selectedSkillScope = selectedSkill?.scope
  const selectedSkillName = selectedSkill?.name
  const editingEntryFile = selectedFilePath === skillEntryFile
  const activeEditable = editingEntryFile ? Boolean(document?.editable) : Boolean(fileDocument?.file.editable)
  const writableScopes = useMemo(() => snapshot.scopes.filter((scope) => scope.writable), [snapshot.scopes])
  const builtinOverrideScope = useMemo(() => preferredBuiltinOverrideScope(snapshot.scopes), [snapshot.scopes])
  const defaultWritableScope: SkillScope = builtinOverrideScope?.scope || 'user'
  const builtinOverride = useMemo(() => {
    if (!document) return null
    if (!builtinOverrideScope) return null
    return snapshot.skills.find((skill) => skill.scope === builtinOverrideScope.scope && skill.name === document.name) ?? null
  }, [builtinOverrideScope, document, snapshot.skills])
  const builtinPeer = useMemo(() => {
    if (!document || document.scope === 'builtin') return null
    return snapshot.skills.find((skill) => skill.scope === 'builtin' && skill.name === document.name) ?? null
  }, [document, snapshot.skills])

  const autosaveDraft = useMemo<SkillContentAutosaveDraft | null>(() => {
    if (!document || mode !== 'editor') return null
    if (editingEntryFile) return skillContentDraft(document, skillEntryFile, draft)
    if (!fileDocument) return null
    return skillContentDraft(document, selectedFilePath, fileDraft, fileDocument.revision)
  }, [document, draft, editingEntryFile, fileDocument, fileDraft, mode, selectedFilePath])

  const contentAutosave = useResourceAutosave<SkillContentAutosaveDraft, SkillContentAutosaveDraft, SkillContentAutosaveSaved>({
    draft: autosaveDraft,
    active: Boolean(autosaveDraft && activeEditable && !fileLoading),
    scopeKey: `${workspace}\u0000${autosaveDraft?.id || selectedKey || ''}`,
    makePayload: (value) => value,
    baselineFromSaved: (saved) => saved,
    signature: skillContentSignature,
    save: async (_id, payload, baseRevision) => {
      if (payload.path === skillEntryFile) {
        const saved = await saveSkillDocument(payload.scope, payload.name, payload.content, undefined, baseRevision)
        return { ...payload, updated_at: saved.revision, document: saved }
      }
      const saved = await saveSkillFileDocument(payload.scope, payload.name, payload.path, payload.content, baseRevision)
      return { ...payload, updated_at: saved.revision, fileDocument: saved }
    },
    resolveConflict: async ({ error: saveError, baseline, draft: submitted }) => {
      if (!isRevisionConflict(saveError)) return null
      if (submitted.path === skillEntryFile) {
        const latest = await getSkillDocument(submitted.scope, submitted.name)
        const content = await rebaseTextWithRecovery({
          resource: 'skill_document',
          scope: `${workspace}:${submitted.scope}`,
          id: submitted.id,
          baseline: { revision: baseline?.updated_at, value: baseline?.content ?? latest.content },
          local: { revision: baseline?.updated_at, value: submitted.content },
          external: { revision: latest.revision, value: latest.content },
        })
        return {
          payload: {
            ...submitted,
            content,
            updated_at: latest.revision,
          },
          baseRevision: latest.revision,
        }
      }
      const latest = await getSkillFileDocument(submitted.scope, submitted.name, submitted.path)
      const content = await rebaseTextWithRecovery({
        resource: 'skill_file',
        scope: `${workspace}:${submitted.scope}`,
        id: submitted.id,
        baseline: { revision: baseline?.updated_at, value: baseline?.content ?? latest.content },
        local: { revision: baseline?.updated_at, value: submitted.content },
        external: { revision: latest.revision, value: latest.content },
      })
      return {
        payload: {
          ...submitted,
          content,
          updated_at: latest.revision,
        },
        baseRevision: latest.revision,
      }
    },
    onSaved: (saved, _mode, submitted) => {
      if (saved.document) {
        setDocument(saved.document)
        setDraft((current) => current === submitted.content ? saved.document!.content : current)
      } else if (saved.fileDocument) {
        setFileDocument(saved.fileDocument)
        setFileDraft((current) => current === submitted.content ? saved.fileDocument!.content : current)
        setDocument((current) => {
          if (!current || current.scope !== saved.scope || current.name !== saved.name) return current
          const files = current.files?.map((file) => file.path === saved.path ? saved.fileDocument!.file : file)
          return { ...current, ...saved.fileDocument!.skill, content: current.content, files }
        })
      }
      const summary = saved.document ? skillSummaryOf(saved.document) : saved.fileDocument?.skill
      if (summary) {
        setSnapshot((current) => ({
          ...current,
          skills: current.skills.map((skill) => keyOf(skill) === keyOf(summary) ? { ...skill, ...summary } : skill),
        }))
      }
      notifySkillsUpdated()
    },
  })

  const baselineDraft = useMemo<SkillContentAutosaveDraft | null>(() => {
    if (!document) return null
    if (editingEntryFile) return skillContentDraft(document, skillEntryFile, document.content)
    if (!fileDocument) return null
    return skillContentDraft(document, selectedFilePath, fileDocument.content, fileDocument.revision)
  }, [document, editingEntryFile, fileDocument, selectedFilePath])

  useEffect(() => {
    contentAutosave.resetBaseline(baselineDraft)
  }, [baselineDraft, contentAutosave.resetBaseline])

  const flushContentAutosave = useCallback(async (force = false) => {
    try {
      const pending = contentAutosave.flushPending()
      if (pending) {
        await pending
      } else if (force || contentAutosave.status === 'error') {
        await contentAutosave.saveNow('manual')
      }
      return true
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : String(saveError))
      return false
    }
  }, [contentAutosave.flushPending, contentAutosave.saveNow, contentAutosave.status])

  const flushActiveAutosave = useCallback(async (force = false) => {
    if (mode === 'config') return configPanelRef.current?.flush() ?? true
    return flushContentAutosave(force)
  }, [flushContentAutosave, mode])

  const load = useCallback(async (): Promise<SkillSnapshot | null> => {
    setLoading(true)
    setError(null)
    try {
      const data = await getSkills()
      setSnapshot(data)
      setSelectedKey((current) => {
        if (current && data.skills.some((skill) => keyOf(skill) === current)) return current
        const firstActive = data.skills.find((skill) => skill.active)
        return firstActive ? keyOf(firstActive) : (data.skills[0] ? keyOf(data.skills[0]) : null)
      })
      return data
    } catch (e) {
      setError((e as Error).message)
      return null
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load, workspace])

  useEffect(() => {
    let cancelled = false
    if (!selectedSkillScope || !selectedSkillName) {
      setDocument(null)
      setDraft('')
      setSelectedFilePath(skillEntryFile)
      setFileDocument(null)
      setFileDraft('')
      return () => { cancelled = true }
    }
    setError(null)
    getSkillDocument(selectedSkillScope, selectedSkillName)
      .then((doc) => {
        if (cancelled) return
        setDocument(doc)
        setDraft(doc.content)
        setSelectedFilePath(skillEntryFile)
        setFileDocument(null)
        setFileDraft('')
        setContentViewMode('preview')
        setFileTreeOpen(false)
      })
      .catch((e) => {
        if (!cancelled) {
          setDocument(null)
          setDraft('')
          setSelectedFilePath(skillEntryFile)
          setFileDocument(null)
          setFileDraft('')
          setError((e as Error).message)
        }
      })
    return () => { cancelled = true }
  }, [documentReloadVersion, selectedSkillName, selectedSkillScope, workspace])

  const resetFileState = () => {
    setSelectedFilePath(skillEntryFile)
    setFileDocument(null)
    setFileDraft('')
  }

  const switchSkillFile = async (path: string) => {
    if (!document || path === selectedFilePath) return
    setError(null)
    if (path === skillEntryFile) {
      resetFileState()
      return
    }
    setFileLoading(true)
    try {
      const doc = await getSkillFileDocument(document.scope, document.name, path)
      setFileDocument(doc)
      setFileDraft(doc.content)
      setSelectedFilePath(path)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setFileLoading(false)
    }
  }

  const selectSkillFile = async (path: string) => {
    if (!document || path === selectedFilePath) return
    if (!await flushContentAutosave()) return
    await switchSkillFile(path)
  }

  const onCreateBuiltinOverride = async () => {
    if (!document) return
    if (builtinOverride) {
      setSelectedKey(keyOf(builtinOverride))
      setMode('editor')
      setError(null)
      return
    }
    if (!builtinOverrideScope) {
      setError(t('skills.override.noWritable'))
      return
    }
    setSaving(true)
    setError(null)
    try {
      let recoveryBaselineRevision = document.revision
      let latestRevision: string | undefined
      const doc = await saveWithRevisionRecovery({
        baseline: document.content,
        draft,
        revision: document.revision,
        save: (content, revision) => saveSkillDocument(
          document.scope,
          document.name,
          content,
          { scope: builtinOverrideScope.scope, name: document.name },
          revision,
        ),
        loadLatest: async () => {
          const latest = await getSkillDocument(document.scope, document.name)
          latestRevision = latest.revision
          return { value: latest.content, revision: latest.revision }
        },
        rebase: async (baseline, local, external) => {
          const content = await rebaseTextWithRecovery({
            resource: 'skill_document',
            scope: `${workspace}:${document.scope}`,
            id: `${document.scope}:${document.name}:override`,
            baseline: { revision: recoveryBaselineRevision, value: baseline },
            local: { revision: recoveryBaselineRevision, value: local },
            external: { revision: latestRevision, value: external },
          })
          recoveryBaselineRevision = latestRevision || recoveryBaselineRevision
          return content
        },
      })
      setDocument(doc)
      setDraft(doc.content)
      resetFileState()
      setSelectedKey(keyOf(doc))
      setMode('editor')
      notifySkillsUpdated()
      await load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const requestDelete = async () => {
    if (!document?.editable) return
    if (!await flushActiveAutosave()) return
    setConfirmRequest({ kind: 'delete', name: document.name })
  }

  const requestRestoreBuiltin = async () => {
    if (!document?.editable || !builtinPeer) return
    if (!await flushActiveAutosave()) return
    setConfirmRequest({ kind: 'restore', name: document.name, scope: scopeLabel(document.scope, t) })
  }

  /** 删除当前文档并刷新列表；失败时抛错由 ConfirmDialog 内联展示。返回刷新后的快照 */
  const deleteCurrentDocument = async (): Promise<SkillSnapshot | null> => {
    if (!document?.editable) return null
    setSaving(true)
    setError(null)
    try {
      await deleteSkillDocument(document.scope, document.name)
      setDocument(null)
      setDraft('')
      resetFileState()
      setMode('editor')
      notifySkillsUpdated()
      return await load()
    } catch (e) {
      setError((e as Error).message)
      throw e
    } finally {
      setSaving(false)
    }
  }

  const performRestoreBuiltin = async () => {
    if (!builtinPeer) return
    const name = document?.name
    const data = await deleteCurrentDocument()
    const revealed = data?.skills.find((skill) => skill.name === name && skill.active) ||
      data?.skills.find((skill) => skill.name === name && skill.scope === 'builtin')
    setSelectedKey(revealed ? keyOf(revealed) : null)
  }

  const confirmContent = useMemo(() => {
    if (!confirmRequest) return null
    if (confirmRequest.kind === 'delete') return { title: t('skills.delete.action'), description: t('skills.delete.confirm', { name: confirmRequest.name }), confirmLabel: t('skills.delete.action'), tone: 'danger' as const }
    return { title: t('skills.restoreBuiltin.action'), description: t('skills.restoreBuiltin.confirm', { name: confirmRequest.name, scope: confirmRequest.scope }), confirmLabel: t('skills.restoreBuiltin.action'), tone: 'danger' as const }
  }, [confirmRequest, t])

  const onConfirmAction = async () => {
    if (!confirmRequest) return
    if (confirmRequest.kind === 'delete') {
      await deleteCurrentDocument()
      return
    }
    await performRestoreBuiltin()
  }

  const onCreated = async (doc: SkillDocument) => {
    setMode('editor')
    notifySkillsUpdated()
    await load()
    setSelectedKey(keyOf(doc))
  }

  const onInstalled = async (result: SkillInstallResult) => {
    const first = result.installed[0]
    setMode('editor')
    notifySkillsUpdated()
    await load()
    if (first) setSelectedKey(keyOf(first))
  }

  const onConfigUpdated = (doc: SkillDocument) => {
    const summary = skillSummaryOf(doc)
    setDocument(doc)
    setDraft(doc.content)
    setSnapshot((current) => ({
      ...current,
      skills: current.skills.map((skill) => keyOf(skill) === keyOf(summary) ? { ...skill, ...summary } : skill),
    }))
    notifySkillsUpdated()
  }

  const onConfigIdentityChanged = async (doc: SkillDocument) => {
    setDocument(doc)
    setDraft(doc.content)
    resetFileState()
    setMode('editor')
    notifySkillsUpdated()
    await load()
    setSelectedKey(keyOf(doc))
  }

  const refreshSkills = useCallback(async () => {
    if (!await flushActiveAutosave()) return
    if (await load()) setDocumentReloadVersion((current) => current + 1)
  }, [flushActiveAutosave, load])

  useEffect(() => {
    const onSkillsUpdated = (event: Event) => {
      const source = (event as CustomEvent<{ source?: string }>).detail?.source
      if (source === eventSource) return
      void refreshSkills()
    }
    window.addEventListener('nova:skills-updated', onSkillsUpdated)
    return () => window.removeEventListener('nova:skills-updated', onSkillsUpdated)
  }, [eventSource, refreshSkills])

  const openMode = async (nextMode: SkillsMode) => {
    if (!await flushActiveAutosave()) return
    setMode(nextMode)
    setError(null)
  }

  const selectSkill = async (key: string) => {
    if (!await flushActiveAutosave()) return
    setSelectedKey(key)
    setMode('editor')
    setError(null)
  }

  const closeSkills = async () => {
    if (!onClose || !await flushActiveAutosave()) return
    onClose()
  }

  const agentContext = useMemo(() => {
    const targetName = document?.name || 'new-skill'
    const scope = document?.scope === 'builtin' && builtinOverrideScope
      ? builtinOverrideScope.scope
      : document?.scope || defaultWritableScope
    return {
      mode,
      skill_name: targetName,
      skill_scope: scope,
      skill_source_scope: document?.scope || scope,
      skill_path: skillFilePath(snapshot.scopes.find((item) => item.scope === scope), targetName) || '',
    }
  }, [builtinOverrideScope, defaultWritableScope, document?.name, document?.scope, mode, snapshot.scopes])

  const agentPanel = agentOpen ? (
    <div className="h-full min-h-0 bg-[var(--nova-surface)]">
      <ConfigManagerChat
        workspace={workspace}
        origin="skills"
        resourceId={agentContext.skill_name}
        context={agentContext}
        onMutated={() => {
          notifySkillsUpdated()
          void refreshSkills()
        }}
      />
    </div>
  ) : null

  return (
    <FeaturePageShell
      icon={Sparkles}
      title={t('skills.title')}
      subtitle={t('skills.subtitle')}
      error={error}
      errorTitle={t('skills.error')}
      onClose={onClose ? () => void closeSkills() : undefined}
      onSaveShortcut={() => flushActiveAutosave(true)}
      className="bg-[var(--nova-bg)] text-[var(--nova-text)]"
      actions={(
        <>
          {mode === 'editor' && document && activeEditable && (
            <AutosaveStatusIndicator
              status={contentAutosave.status}
              error={contentAutosave.error}
              onRetry={contentAutosave.retry}
            />
          )}
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void refreshSkills()}
            disabled={loading}
            className="nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)]"
          >
            <RefreshCw data-icon="inline-start" className={loading ? 'animate-spin' : undefined} />
            {t('common.refresh')}
          </Button>
        </>
      )}
    >
      <AdaptiveSurface
        left={{
          id: 'skills-list',
          title: t('skills.title'),
          side: 'left',
          icon: <Sparkles className="h-4 w-4" />,
          content: (
            <SkillListPanel
              snapshot={snapshot}
              selectedKey={selectedKey}
              loading={loading}
              agentOpen={agentOpen}
              mode={mode}
              onToggleAgent={() => setAgentOpen((value) => !value)}
              onCreate={() => void openMode('create')}
              onInstall={() => void openMode('install')}
              onSelect={(key) => void selectSkill(key)}
            />
          ),
          desktopClassName: 'w-80 shrink-0 min-h-0 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(90vw,380px)]',
        }}
        right={
          agentOpen && agentPanel
            ? {
                id: 'skills-agent',
                title: t('skills.agent.button'),
                side: 'right',
                icon: <Bot className="h-4 w-4" />,
                content: agentPanel,
                desktopClassName: 'min-h-0 border-l border-[var(--nova-border)]',
              }
            : undefined
        }
        className="flex-1 text-xs"
        mainClassName="min-h-0 min-w-0"
        desktopGridClassName={agentOpen ? 'grid-cols-[20rem_minmax(0,1fr)_minmax(320px,28rem)]' : 'grid-cols-[20rem_minmax(0,1fr)]'}
        rightResize={{
          layoutKey: 'nova-skills-config-agent-layout',
          label: t('layout.resize.right'),
          defaultSize: '420px',
          minSize: '300px',
          maxSize: '65%',
          mainMinSize: '240px',
        }}
      >
        {({ openLeft, openRight }) => (
          <main className="flex h-full min-h-0 flex-col">
            <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 md:hidden">
              <MobilePaneTrigger
                side="left"
                label={t('workbench.mobile.openSidePanel', { label: t('skills.title') })}
                onClick={openLeft}
              />
              <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-muted)]">{document?.name || t('skills.title')}</span>
              {agentOpen && (
                <MobilePaneTrigger
                  side="right"
                  label={t('workbench.mobile.openSidePanel', { label: t('skills.agent.button') })}
                  onClick={openRight}
                />
              )}
            </div>
            {mode === 'create' ? (
              <SkillCreatePanel
                scopes={writableScopes}
                defaultScope={defaultWritableScope}
                onCreated={onCreated}
                onAskAgent={() => setAgentOpen((value) => !value)}
              />
            ) : mode === 'install' ? (
              <SkillInstallPanel
                scopes={writableScopes}
                defaultScope={defaultWritableScope}
                onInstalled={onInstalled}
              />
            ) : mode === 'config' && document ? (
              <SkillConfigPanel
                ref={configPanelRef}
                document={document}
                content={draft}
                scopes={writableScopes}
                onUpdated={onConfigUpdated}
                onIdentityChanged={onConfigIdentityChanged}
                onCancel={() => setMode('editor')}
                onDelete={() => void requestDelete()}
              />
            ) : document ? (
              <SkillEditor
                document={document}
                fileDocument={fileDocument}
                draft={draft}
                fileDraft={fileDraft}
                selectedFilePath={selectedFilePath}
                viewMode={contentViewMode}
                fileTreeOpen={fileTreeOpen}
                fileLoading={fileLoading}
                saving={saving || contentAutosave.status === 'saving'}
                builtinOverride={builtinOverride}
                builtinOverrideScope={builtinOverrideScope}
                builtinPeer={builtinPeer}
                onDraftChange={setDraft}
                onFileDraftChange={setFileDraft}
                onSelectFile={(path) => void selectSkillFile(path)}
                onToggleFileTree={() => setFileTreeOpen((value) => !value)}
                onViewModeChange={setContentViewMode}
                onOpenConfig={() => {
                  if (!document.editable) return
                  void openMode('config')
                }}
                onDelete={() => void requestDelete()}
                onRestoreBuiltin={() => void requestRestoreBuiltin()}
                onCreateBuiltinOverride={() => void onCreateBuiltinOverride()}
              />
            ) : (
              <EmptyState
                icon={Sparkles}
                title={loading ? t('skills.loading') : t('skills.empty')}
                variant="page"
                className="h-full text-xs text-[var(--nova-text-faint)]"
              />
            )}
          </main>
        )}
      </AdaptiveSurface>

      {confirmContent && (
        <ConfirmDialog
          open={confirmRequest !== null}
          onOpenChange={(open) => { if (!open) setConfirmRequest(null) }}
          title={confirmContent.title}
          description={confirmContent.description}
          confirmLabel={confirmContent.confirmLabel}
          tone={confirmContent.tone}
          onConfirm={onConfirmAction}
        />
      )}
    </FeaturePageShell>
  )
}
