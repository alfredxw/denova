import { useCallback, useEffect, useMemo, useState } from 'react'
import { Bot, Loader2, RefreshCw, Save, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { Button } from '@/components/ui/button'
import { deleteSkillDocument, getSkillDocument, getSkillFileDocument, getSkills, saveSkillDocument, saveSkillFileDocument } from '@/lib/api'
import type { SkillDocument, SkillFileDocument, SkillInstallResult, SkillScope, SkillSnapshot } from '@/lib/api'
import { SkillConfigPanel } from './SkillConfigPanel'
import { SkillCreatePanel } from './SkillCreatePanel'
import { SkillEditor } from './SkillEditor'
import { SkillInstallPanel } from './SkillInstallPanel'
import { SkillListPanel } from './SkillListPanel'
import { keyOf, preferredBuiltinOverrideScope, scopeLabel, skillEntryFile, skillFilePath, type SkillContentViewMode, type SkillsMode } from './skill-utils'

interface SkillsViewProps {
  workspace: string
  onClose?: () => void
}

/** 待确认动作：discard 记录待切换的文件路径，delete/restore 快照名称避免文档变化影响弹窗文案 */
type ConfirmRequest =
  | { kind: 'discard'; path: string }
  | { kind: 'delete'; name: string }
  | { kind: 'restore'; name: string; scope: string }

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

  const selectedSkill = useMemo(() => snapshot.skills.find((skill) => keyOf(skill) === selectedKey) ?? null, [selectedKey, snapshot.skills])
  const editingEntryFile = selectedFilePath === skillEntryFile
  const dirty = document ? (editingEntryFile ? draft !== document.content : Boolean(fileDocument && fileDraft !== fileDocument.content)) : false
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
    if (!selectedSkill) {
      setDocument(null)
      setDraft('')
      setSelectedFilePath(skillEntryFile)
      setFileDocument(null)
      setFileDraft('')
      return () => { cancelled = true }
    }
    setError(null)
    getSkillDocument(selectedSkill.scope, selectedSkill.name)
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
  }, [selectedSkill])

  const resetFileState = () => {
    setSelectedFilePath(skillEntryFile)
    setFileDocument(null)
    setFileDraft('')
  }

  /** 切文件实际逻辑，不检查脏状态；脏检查与确认弹窗在 selectSkillFile */
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
    if (dirty) {
      setConfirmRequest({ kind: 'discard', path })
      return
    }
    await switchSkillFile(path)
  }

  const onSave = async () => {
    if (!document || !activeEditable) return
    setSaving(true)
    setError(null)
    try {
      if (editingEntryFile) {
        const doc = await saveSkillDocument(document.scope, document.name, draft)
        setDocument(doc)
        setDraft(doc.content)
        setSelectedKey(keyOf(doc))
        resetFileState()
        window.dispatchEvent(new CustomEvent('nova:skills-updated'))
        await load()
      } else {
        const fileDoc = await saveSkillFileDocument(document.scope, document.name, selectedFilePath, fileDraft)
        setFileDocument(fileDoc)
        setFileDraft(fileDoc.content)
        const refreshed = await getSkillDocument(document.scope, document.name)
        setDocument(refreshed)
        setDraft(refreshed.content)
        window.dispatchEvent(new CustomEvent('nova:skills-updated'))
      }
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
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
      const doc = await saveSkillDocument(document.scope, document.name, draft, { scope: builtinOverrideScope.scope, name: document.name })
      setDocument(doc)
      setDraft(doc.content)
      resetFileState()
      setSelectedKey(keyOf(doc))
      setMode('editor')
      window.dispatchEvent(new CustomEvent('nova:skills-updated'))
      await load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const requestDelete = () => {
    if (!document?.editable) return
    setConfirmRequest({ kind: 'delete', name: document.name })
  }

  const requestRestoreBuiltin = () => {
    if (!document?.editable || !builtinPeer) return
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
      window.dispatchEvent(new CustomEvent('nova:skills-updated'))
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
    if (confirmRequest.kind === 'discard') return { title: t('skills.unsaved'), description: t('skills.files.discardConfirm'), confirmLabel: undefined, tone: 'default' as const }
    if (confirmRequest.kind === 'delete') return { title: t('skills.delete.action'), description: t('skills.delete.confirm', { name: confirmRequest.name }), confirmLabel: t('skills.delete.action'), tone: 'danger' as const }
    return { title: t('skills.restoreBuiltin.action'), description: t('skills.restoreBuiltin.confirm', { name: confirmRequest.name, scope: confirmRequest.scope }), confirmLabel: t('skills.restoreBuiltin.action'), tone: 'danger' as const }
  }, [confirmRequest, t])

  const onConfirmAction = async () => {
    if (!confirmRequest) return
    if (confirmRequest.kind === 'discard') {
      await switchSkillFile(confirmRequest.path)
      return
    }
    if (confirmRequest.kind === 'delete') {
      await deleteCurrentDocument()
      return
    }
    await performRestoreBuiltin()
  }

  const onCreated = async (doc: SkillDocument) => {
    setMode('editor')
    window.dispatchEvent(new CustomEvent('nova:skills-updated'))
    await load()
    setSelectedKey(keyOf(doc))
  }

  const onInstalled = async (result: SkillInstallResult) => {
    const first = result.installed[0]
    setMode('editor')
    window.dispatchEvent(new CustomEvent('nova:skills-updated'))
    await load()
    if (first) setSelectedKey(keyOf(first))
  }

  const onConfigSaved = async (doc: SkillDocument) => {
    setDocument(doc)
    setDraft(doc.content)
    resetFileState()
    setMode('editor')
    window.dispatchEvent(new CustomEvent('nova:skills-updated'))
    await load()
    setSelectedKey(keyOf(doc))
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
          window.dispatchEvent(new CustomEvent('nova:skills-updated'))
          void load()
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
      onClose={onClose}
      className="bg-[var(--nova-bg)] text-[var(--nova-text)]"
      actions={(
        <>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void load()}
            disabled={loading}
            className="nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)]"
          >
            <RefreshCw data-icon="inline-start" className={loading ? 'animate-spin' : undefined} />
            {t('common.refresh')}
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => void onSave()}
            disabled={mode !== 'editor' || !dirty || saving || fileLoading || !activeEditable}
            className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-active)]"
          >
            {saving ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Save data-icon="inline-start" />}
            {t('common.save')}
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
              onCreate={() => {
                setMode('create')
                setError(null)
              }}
              onInstall={() => {
                setMode('install')
                setError(null)
              }}
              onSelect={(key) => {
                setSelectedKey(key)
                setMode('editor')
              }}
            />
          ),
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
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
                document={document}
                content={draft}
                scopes={writableScopes}
                onSaved={onConfigSaved}
                onCancel={() => setMode('editor')}
                onDelete={requestDelete}
              />
            ) : document ? (
              <SkillEditor
                document={document}
                fileDocument={fileDocument}
                draft={draft}
                fileDraft={fileDraft}
                dirty={dirty}
                selectedFilePath={selectedFilePath}
                viewMode={contentViewMode}
                fileTreeOpen={fileTreeOpen}
                fileLoading={fileLoading}
                saving={saving}
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
                  setMode('config')
                  setError(null)
                }}
                onDelete={requestDelete}
                onRestoreBuiltin={requestRestoreBuiltin}
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
