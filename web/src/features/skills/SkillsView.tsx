import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ElementType, ReactNode } from 'react'
import { Bot, CheckCircle2, FileCode2, Loader2, Lock, PanelLeft, PanelRight, Plus, RefreshCw, Save, Settings2, Sparkles, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { Textarea } from '@/components/ui/textarea'
import { createSkill, deleteSkillDocument, getSkillDocument, getSkills, saveSkillDocument } from '@/lib/api'
import type { SkillDocument, SkillScope, SkillScopeInfo, SkillSnapshot, SkillSummary } from '@/lib/api'
import { AGENTS } from '@/features/agents/agent-registry'
import type { AgentViewDefinition, VisibleAgentKey } from '@/features/agents/agent-registry'

const skillNamePattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/
const scopes: SkillScope[] = ['workspace', 'user', 'builtin']
const skillAgentOptions = AGENTS.filter((agent) => agent.capabilityMode === 'tools')

interface SkillsViewProps {
  workspace: string
  onClose?: () => void
  onRequestAgent?: (prompt: string) => void
}

type SkillsMode = 'editor' | 'create' | 'config'

export function SkillsView({ workspace, onClose, onRequestAgent }: SkillsViewProps) {
  void onRequestAgent
  const { t } = useTranslation()
  const [snapshot, setSnapshot] = useState<SkillSnapshot>({ scopes: [], skills: [] })
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [document, setDocument] = useState<SkillDocument | null>(null)
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<SkillsMode>('editor')
  const [newScope, setNewScope] = useState<SkillScope>('workspace')
  const [newName, setNewName] = useState('')
  const [newDescription, setNewDescription] = useState('')
  const [newAgents, setNewAgents] = useState<VisibleAgentKey[]>(['ide'])
  const [configDescription, setConfigDescription] = useState('')
  const [configAgents, setConfigAgents] = useState<VisibleAgentKey[]>([])
  const [agentOpen, setAgentOpen] = useState(false)

  const selectedSkill = useMemo(() => snapshot.skills.find((skill) => keyOf(skill) === selectedKey) ?? null, [selectedKey, snapshot.skills])
  const dirty = document ? draft !== document.content : false
  const writableScopes = useMemo(() => snapshot.scopes.filter((scope) => scope.writable), [snapshot.scopes])

  const load = useCallback(async () => {
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
      const nextWritable = data.scopes.find((scope) => scope.scope === 'workspace' && scope.writable) ||
        data.scopes.find((scope) => scope.scope === 'user' && scope.writable)
      if (nextWritable) setNewScope(nextWritable.scope)
    } catch (e) {
      setError((e as Error).message)
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
      return () => { cancelled = true }
    }
    setError(null)
    getSkillDocument(selectedSkill.scope, selectedSkill.name)
      .then((doc) => {
        if (cancelled) return
        setDocument(doc)
        setDraft(doc.content)
      })
      .catch((e) => {
        if (!cancelled) {
          setDocument(null)
          setDraft('')
          setError((e as Error).message)
        }
      })
    return () => { cancelled = true }
  }, [selectedSkill])

  const onCreate = async () => {
    const name = newName.trim()
    if (!skillNamePattern.test(name)) {
      setError(t('skills.create.invalidName'))
      return
    }
    setSaving(true)
    setError(null)
    try {
      const doc = await createSkill(newScope, name, newDescription.trim(), newAgents)
      const docKey = keyOf(doc)
      setNewName('')
      setNewDescription('')
      setNewAgents(['ide'])
      setMode('editor')
      window.dispatchEvent(new CustomEvent('nova:skills-updated'))
      await load()
      setSelectedKey(docKey)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const onSave = async () => {
    if (!document || !document.editable) return
    setSaving(true)
    setError(null)
    try {
      const doc = await saveSkillDocument(document.scope, document.name, draft)
      setDocument(doc)
      setDraft(doc.content)
      setSelectedKey(keyOf(doc))
      window.dispatchEvent(new CustomEvent('nova:skills-updated'))
      await load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const openConfig = () => {
    if (!document?.editable) return
    setConfigDescription(document.description)
    setConfigAgents(parseAgentKeys(document.agent))
    setMode('config')
    setError(null)
  }

  const onSaveConfig = async () => {
    if (!document?.editable) return
    const description = configDescription.trim()
    if (!description) {
      setError(t('skills.config.descriptionRequired'))
      return
    }
    setSaving(true)
    setError(null)
    try {
      const nextContent = updateSkillConfigContent(document.content, document.name, description, configAgents)
      const doc = await saveSkillDocument(document.scope, document.name, nextContent)
      setDocument(doc)
      setDraft(doc.content)
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

  const onDelete = async () => {
    if (!document?.editable) return
    if (!window.confirm(t('skills.delete.confirm', { name: document.name }))) return
    setSaving(true)
    setError(null)
    try {
      await deleteSkillDocument(document.scope, document.name)
      setDocument(null)
      setDraft('')
      setMode('editor')
      setSelectedKey(null)
      window.dispatchEvent(new CustomEvent('nova:skills-updated'))
      await load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const askAgent = () => {
    setAgentOpen((value) => !value)
  }

  const agentContext = useMemo(() => {
    const targetName = mode === 'create' ? newName.trim() || 'new-skill' : document?.name || newName.trim() || 'new-skill'
    const scope = mode === 'create' ? newScope : document?.scope || newScope
    return {
      mode,
      skill_name: targetName,
      skill_scope: scope,
      skill_path: skillFilePath(snapshot.scopes.find((item) => item.scope === scope), targetName) || '',
    }
  }, [document?.name, document?.scope, mode, newName, newScope, snapshot.scopes])
  const skillListPanel = (
    <div className="h-full min-h-0 overflow-y-auto bg-[var(--nova-surface-2)] p-3">
      <div className="mb-4 grid grid-cols-2 gap-2">
        <button
          type="button"
          onClick={askAgent}
          className={`nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded border border-[var(--nova-border)] px-2 ${agentOpen ? 'is-active' : 'bg-[var(--nova-surface)]'}`}
        >
          <Bot className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('skills.agent.button')}</span>
        </button>
        <button
          type="button"
          onClick={() => {
            setMode('create')
            setError(null)
          }}
          className={`nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded border border-[var(--nova-border)] px-2 ${mode === 'create' ? 'is-active' : 'bg-[var(--nova-surface)]'}`}
        >
          <Plus className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('skills.create.title')}</span>
        </button>
      </div>

      <div className="space-y-4">
        {scopes.map((scope) => (
          <SkillScopeList
            key={scope}
            scope={scope}
            scopeInfo={snapshot.scopes.find((item) => item.scope === scope)}
            skills={snapshot.skills.filter((skill) => skill.scope === scope)}
            selectedKey={selectedKey}
            onSelect={(key) => {
              setSelectedKey(key)
              setMode('editor')
            }}
          />
        ))}
      </div>
    </div>
  )
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
    <div className="flex h-full min-h-0 w-full flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="nova-topbar flex min-h-10 shrink-0 flex-wrap items-center gap-2 border-b px-4 py-1.5 text-xs">
        <Sparkles className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="font-medium">{t('skills.title')}</span>
        <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">{t('skills.subtitle')}</span>
        <button
          type="button"
          onClick={() => void load()}
          disabled={loading}
          className="nova-nav-item ml-auto inline-flex items-center gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1 disabled:opacity-50"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
          {t('common.refresh')}
        </button>
        <button
          type="button"
          onClick={() => void onSave()}
          disabled={mode !== 'editor' || !dirty || saving || !document?.editable}
          className="nova-nav-item inline-flex items-center gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-active)] px-2.5 py-1 disabled:cursor-not-allowed disabled:opacity-45"
        >
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
          {t('common.save')}
        </button>
        {onClose && (
          <button type="button" onClick={onClose} className="nova-nav-item rounded p-1" aria-label={t('common.close')} title={t('common.close')}>
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {error && <InlineErrorNotice className="mx-3 mt-2" message={error} title={t('skills.error')} />}

      <AdaptiveSurface
        left={{
          id: 'skills-list',
          title: t('skills.title'),
          side: 'left',
          icon: <Sparkles className="h-4 w-4" />,
          content: skillListPanel,
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
              <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: t('skills.title') })} onClick={openLeft}>
                <PanelLeft className="h-4 w-4" />
              </button>
              <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--nova-text-muted)]">{document?.name || t('skills.title')}</span>
              {agentOpen && (
                <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: t('skills.agent.button') })} onClick={openRight}>
                  <PanelRight className="h-4 w-4" />
                </button>
              )}
            </div>
            {mode === 'create' ? (
              <CreateSkillPanel
                name={newName}
                description={newDescription}
                scope={newScope}
                agents={newAgents}
                scopes={writableScopes}
                scopeInfo={snapshot.scopes.find((item) => item.scope === newScope)}
                saving={saving}
                onNameChange={setNewName}
                onDescriptionChange={setNewDescription}
                onScopeChange={setNewScope}
                onAgentsChange={setNewAgents}
                onCreate={() => void onCreate()}
                onAskAgent={askAgent}
              />
            ) : mode === 'config' && document ? (
              <SkillConfigPanel
                document={document}
                description={configDescription}
                agents={configAgents}
                saving={saving}
                onDescriptionChange={setConfigDescription}
                onAgentsChange={setConfigAgents}
                onSave={() => void onSaveConfig()}
                onCancel={() => setMode('editor')}
                onDelete={() => void onDelete()}
              />
            ) : document ? (
              <>
              <div className="flex min-h-12 shrink-0 items-center gap-3 border-b border-[var(--nova-border)] px-4">
                <FileCode2 className="h-4 w-4 text-[var(--nova-text-muted)]" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm text-[var(--nova-text)]">/{document.name}</span>
                    <span className="rounded bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{scopeLabel(document.scope, t)}</span>
                    {!document.active && <span className="rounded bg-[var(--nova-warning-bg)] px-1.5 py-0.5 text-[10px] text-[var(--nova-warning)]">{t('skills.shadowed')}</span>}
                    {document.agent && <span className="rounded bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{document.agent}</span>}
                    {!document.editable && <Lock className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />}
                  </div>
                  <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]" title={document.path}>{document.path}</div>
                </div>
                {dirty && <span className="text-[11px] text-[var(--nova-warning)]">{t('skills.unsaved')}</span>}
                {document.editable && (
                  <>
                    <button
                      type="button"
                      onClick={openConfig}
                      className="nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px]"
                    >
                      <Settings2 className="h-3.5 w-3.5" />
                      {t('skills.config.action')}
                    </button>
                    <button
                      type="button"
                      onClick={() => void onDelete()}
                      disabled={saving}
                      className="nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] text-[var(--nova-danger)] disabled:cursor-not-allowed disabled:opacity-45"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {t('skills.delete.action')}
                    </button>
                  </>
                )}
              </div>
              <Textarea
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                readOnly={!document.editable}
                spellCheck={false}
                className="min-h-0 flex-1 resize-none rounded-none border-0 bg-[var(--nova-bg)] px-5 py-4 font-mono text-xs leading-5 text-[var(--nova-text)] shadow-none focus-visible:ring-0"
              />
              </>
            ) : (
              <div className="flex h-full items-center justify-center px-6 text-center text-xs text-[var(--nova-text-faint)]">
                {loading ? t('skills.loading') : t('skills.empty')}
              </div>
            )}
          </main>
        )}
      </AdaptiveSurface>
    </div>
  )
}

function CreateSkillPanel({
  name,
  description,
  scope,
  agents,
  scopes,
  scopeInfo,
  saving,
  onNameChange,
  onDescriptionChange,
  onScopeChange,
  onAgentsChange,
  onCreate,
  onAskAgent,
}: {
  name: string
  description: string
  scope: SkillScope
  agents: VisibleAgentKey[]
  scopes: SkillScopeInfo[]
  scopeInfo?: SkillScopeInfo
  saving: boolean
  onNameChange: (value: string) => void
  onDescriptionChange: (value: string) => void
  onScopeChange: (value: SkillScope) => void
  onAgentsChange: (value: VisibleAgentKey[]) => void
  onCreate: () => void
  onAskAgent: () => void
}) {
  const { t } = useTranslation()
  const trimmedName = name.trim()
  const invalidName = trimmedName !== '' && !skillNamePattern.test(trimmedName)
  const targetName = trimmedName || t('skills.create.namePlaceholder')
  const targetPath = skillFilePath(scopeInfo, targetName)

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex max-w-5xl flex-col gap-5 px-6 py-5">
        <section className="border-b border-[var(--nova-border)] pb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
              <Plus className="h-4 w-4 text-[var(--nova-text-muted)]" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-sm font-semibold">{t('skills.create.title')}</h1>
              <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('skills.create.subtitle')}</div>
            </div>
          </div>
        </section>

        {scopes.length === 0 ? (
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">
            {t('skills.create.noWritableScope')}
          </div>
        ) : (
          <>
            <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
              <SectionTitle icon={FileCode2} title={t('skills.create.section.identity')} />
              <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                <Field label={t('skills.create.scope')}>
                  <div className="flex gap-1">
                    {scopes.map((item) => (
                      <button
                        key={item.scope}
                        type="button"
                        onClick={() => onScopeChange(item.scope)}
                        className={`nova-nav-item h-8 flex-1 rounded-[var(--nova-radius)] px-2 ${scope === item.scope ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
                      >
                        {scopeLabel(item.scope, t)}
                      </button>
                    ))}
                  </div>
                </Field>
                <Field label={t('skills.create.name')}>
                  <input
                    value={name}
                    onChange={(event) => onNameChange(event.target.value)}
                    aria-invalid={invalidName}
                    aria-label={t('skills.create.name')}
                    placeholder={t('skills.create.namePlaceholder')}
                    className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 font-mono outline-none aria-invalid:border-[var(--nova-danger)]"
                  />
                  <div className={`mt-1 text-[11px] ${invalidName ? 'text-[var(--nova-danger)]' : 'text-[var(--nova-text-faint)]'}`}>
                    {invalidName ? t('skills.create.invalidName') : t('skills.create.nameHint')}
                  </div>
                </Field>
              </div>
              <Field label={t('skills.create.description')}>
                <input
                  value={description}
                  onChange={(event) => onDescriptionChange(event.target.value)}
                  aria-label={t('skills.create.description')}
                  placeholder={t('skills.create.descriptionPlaceholder')}
                  className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 outline-none"
                />
                <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('skills.create.descriptionHint')}</div>
              </Field>
            </section>

            <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
              <SectionTitle icon={Bot} title={t('skills.create.section.agents')} />
              <SkillAgentSelector agents={agents} onAgentsChange={onAgentsChange} />
              <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
                {agents.length === 0 ? t('skills.create.agentsAllHint') : t('skills.create.agentsHint')}
              </div>
            </section>

            <section className="space-y-3 pb-5">
              <SectionTitle icon={Sparkles} title={t('skills.create.section.preview')} />
              <div className="grid gap-2 md:grid-cols-2">
                <PreviewRow label={t('skills.create.preview.command')} value={`/${targetName}`} />
                <PreviewRow label={t('skills.create.preview.scope')} value={scopeLabel(scope, t)} />
                <PreviewRow label={t('skills.create.preview.path')} value={targetPath || t('skills.agent.pathFallback')} wide />
                <PreviewRow
                  label={t('skills.create.preview.agents')}
                  value={agents.length > 0 ? agents.map((agent) => t(AGENTS.find((item) => item.key === agent)?.titleKey || agent)).join(', ') : t('skills.create.preview.allAgents')}
                  wide
                />
              </div>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={onCreate}
                  disabled={saving || !trimmedName || invalidName}
                  className="nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 disabled:cursor-not-allowed disabled:opacity-45"
                >
                  {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
                  {t('skills.create.submit')}
                </button>
                <button
                  type="button"
                  onClick={onAskAgent}
                  className="nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3"
                >
                  <Bot className="h-3.5 w-3.5" />
                  {t('skills.create.askAgent')}
                </button>
              </div>
            </section>
          </>
        )}
      </div>
    </div>
  )
}

function SkillConfigPanel({
  document,
  description,
  agents,
  saving,
  onDescriptionChange,
  onAgentsChange,
  onSave,
  onCancel,
  onDelete,
}: {
  document: SkillDocument
  description: string
  agents: VisibleAgentKey[]
  saving: boolean
  onDescriptionChange: (value: string) => void
  onAgentsChange: (value: VisibleAgentKey[]) => void
  onSave: () => void
  onCancel: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const trimmedDescription = description.trim()

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex max-w-5xl flex-col gap-5 px-6 py-5">
        <section className="border-b border-[var(--nova-border)] pb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
              <Settings2 className="h-4 w-4 text-[var(--nova-text-muted)]" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-sm font-semibold">{t('skills.config.title')}</h1>
              <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('skills.config.subtitle')}</div>
            </div>
          </div>
        </section>

        <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
          <SectionTitle icon={FileCode2} title={t('skills.create.section.identity')} />
          <div className="grid gap-2 md:grid-cols-2">
            <PreviewRow label={t('skills.create.preview.command')} value={`/${document.name}`} />
            <PreviewRow label={t('skills.create.preview.scope')} value={scopeLabel(document.scope, t)} />
            <PreviewRow label={t('skills.create.preview.path')} value={document.path} wide />
          </div>
          <Field label={t('skills.create.description')}>
            <input
              value={description}
              onChange={(event) => onDescriptionChange(event.target.value)}
              aria-label={t('skills.create.description')}
              placeholder={t('skills.create.descriptionPlaceholder')}
              className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 outline-none"
            />
            <div className={`mt-1 text-[11px] ${trimmedDescription ? 'text-[var(--nova-text-faint)]' : 'text-[var(--nova-danger)]'}`}>
              {trimmedDescription ? t('skills.create.descriptionHint') : t('skills.config.descriptionRequired')}
            </div>
          </Field>
        </section>

        <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
          <SectionTitle icon={Bot} title={t('skills.create.section.agents')} />
          <SkillAgentSelector agents={agents} onAgentsChange={onAgentsChange} />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
            {agents.length === 0 ? t('skills.create.agentsAllHint') : t('skills.create.agentsHint')}
          </div>
        </section>

        <section className="flex flex-wrap gap-2 pb-5">
          <button
            type="button"
            onClick={onSave}
            disabled={saving || !trimmedDescription}
            className="nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 disabled:cursor-not-allowed disabled:opacity-45"
          >
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
            {t('skills.config.save')}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3"
          >
            {t('common.cancel')}
          </button>
          <button
            type="button"
            onClick={onDelete}
            disabled={saving}
            className="nova-nav-item ml-auto inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-[var(--nova-danger)] disabled:cursor-not-allowed disabled:opacity-45"
          >
            <Trash2 className="h-3.5 w-3.5" />
            {t('skills.delete.action')}
          </button>
        </section>
      </div>
    </div>
  )
}

function SkillAgentSelector({
  agents,
  onAgentsChange,
}: {
  agents: VisibleAgentKey[]
  onAgentsChange: (value: VisibleAgentKey[]) => void
}) {
  const { t } = useTranslation()
  const agentGroups = groupSkillAgents(skillAgentOptions)
  const toggleAgent = (agent: VisibleAgentKey, checked: boolean) => {
    if (checked) {
      onAgentsChange(agents.includes(agent) ? agents : [...agents, agent])
      return
    }
    onAgentsChange(agents.filter((item) => item !== agent))
  }

  return (
    <div className="space-y-3">
      {agentGroups.map((group) => (
        <div key={group.group}>
          <div className="mb-1.5 text-[11px] font-medium text-[var(--nova-text-faint)]">{t(group.group)}</div>
          <div className="grid gap-2 md:grid-cols-2">
            {group.agents.map((agent) => {
              const Icon = agent.icon
              const checked = agents.includes(agent.key)
              return (
                <label
                  key={agent.key}
                  className={`nova-nav-item flex min-h-14 cursor-pointer items-center gap-3 rounded-[var(--nova-radius)] border px-3 py-2 ${checked ? 'is-active border-[var(--nova-border)]' : 'border-transparent bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:border-[var(--nova-border)]'}`}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={(event) => toggleAgent(agent.key, event.target.checked)}
                    className="h-3.5 w-3.5"
                  />
                  <Icon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
                  <span className="min-w-0">
                    <span className="block truncate font-medium text-[var(--nova-text)]">{t(agent.titleKey)}</span>
                    <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{t(agent.subtitleKey)}</span>
                  </span>
                </label>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}

function SkillScopeList({
  scope,
  scopeInfo,
  skills,
  selectedKey,
  onSelect,
}: {
  scope: SkillScope
  scopeInfo?: SkillScopeInfo
  skills: SkillSummary[]
  selectedKey: string | null
  onSelect: (key: string) => void
}) {
  const { t } = useTranslation()
  return (
    <section>
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <div className="font-medium text-[var(--nova-text-muted)]">{scopeLabel(scope, t)}</div>
        <div className="text-[10px] text-[var(--nova-text-faint)]">{scopeInfo?.writable ? t('skills.scope.editable') : t('skills.scope.readonly')}</div>
      </div>
      {scopeInfo?.path && <div className="mb-2 truncate font-mono text-[10px] text-[var(--nova-text-faint)]" title={scopeInfo.path}>{scopeInfo.path}</div>}
      {skills.length === 0 ? (
        <div className="rounded border border-dashed border-[var(--nova-border)] px-2 py-3 text-center text-[11px] text-[var(--nova-text-faint)]">{t('skills.scope.empty')}</div>
      ) : (
        <div className="space-y-1">
          {skills.map((skill) => {
            const active = selectedKey === keyOf(skill)
            return (
              <button
                key={keyOf(skill)}
                type="button"
                onClick={() => onSelect(keyOf(skill))}
                className={`nova-nav-item w-full rounded border px-2.5 py-2 text-left ${
                  active
                    ? 'is-active border-[var(--nova-border)]'
                    : 'border-transparent bg-[var(--nova-surface)] hover:border-[var(--nova-border)]'
                }`}
              >
                <span className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate font-mono text-xs text-[var(--nova-text)]">/{skill.name}</span>
                  {skill.active ? <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-[var(--nova-success)]" /> : <span className="shrink-0 text-[10px] text-[var(--nova-warning)]">{t('skills.shadowed')}</span>}
                  {!skill.editable && <Lock className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
                </span>
                <span className="mt-1 line-clamp-2 block text-[11px] leading-4 text-[var(--nova-text-faint)]">{skill.description}</span>
              </button>
            )
          })}
        </div>
      )}
    </section>
  )
}

function SectionTitle({ icon: Icon, title }: { icon: ElementType; title: string }) {
  return (
    <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
      <Icon className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
      {title}
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="block">
      <span className="mb-1.5 block text-[11px] font-medium text-[var(--nova-text-muted)]">{label}</span>
      {children}
    </div>
  )
}

function PreviewRow({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return (
    <div className={`rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 ${wide ? 'md:col-span-2' : ''}`}>
      <div className="text-[10px] uppercase text-[var(--nova-text-faint)]">{label}</div>
      <div className="mt-1 truncate font-mono text-xs text-[var(--nova-text)]" title={value}>{value}</div>
    </div>
  )
}

function keyOf(skill: Pick<SkillSummary, 'scope' | 'name'>) {
  return `${skill.scope}:${skill.name}`
}

function skillFilePath(scope: SkillScopeInfo | undefined, name: string) {
  if (!scope?.path) return ''
  return `${scope.path.replace(/\/+$/, '')}/${name}/SKILL.md`
}

function parseAgentKeys(agentField?: string): VisibleAgentKey[] {
  const allowed = new Set<string>(skillAgentOptions.map((agent) => agent.key))
  const seen = new Set<VisibleAgentKey>()
  const out: VisibleAgentKey[] = []
  for (const part of (agentField || '').split(/[,;\s]+/)) {
    if (!allowed.has(part)) continue
    const agent = part as VisibleAgentKey
    if (seen.has(agent)) continue
    seen.add(agent)
    out.push(agent)
  }
  return out
}

function updateSkillConfigContent(content: string, name: string, description: string, agents: VisibleAgentKey[]) {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---(\r?\n?[\s\S]*)$/)
  if (!match) return content
  const newline = content.includes('\r\n') ? '\r\n' : '\n'
  const seen = { name: false, description: false, agent: false }
  const nextLines: string[] = []
  for (const line of match[1].split(/\r?\n/)) {
    const key = line.match(/^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*:/)?.[1]
    if (key === 'name') {
      seen.name = true
      nextLines.push(`name: ${yamlString(name)}`)
      continue
    }
    if (key === 'description') {
      seen.description = true
      nextLines.push(`description: ${yamlString(description)}`)
      continue
    }
    if (key === 'agent') {
      seen.agent = true
      if (agents.length > 0) nextLines.push(`agent: ${yamlString(agents.join(','))}`)
      continue
    }
    nextLines.push(line)
  }
  if (!seen.name) nextLines.unshift(`name: ${yamlString(name)}`)
  if (!seen.description) nextLines.push(`description: ${yamlString(description)}`)
  if (!seen.agent && agents.length > 0) nextLines.push(`agent: ${yamlString(agents.join(','))}`)
  return `---${newline}${nextLines.join(newline)}${newline}---${match[2]}`
}

function yamlString(value: string) {
  return JSON.stringify(value)
}

function scopeLabel(scope: SkillScope, t: (key: string) => string) {
  if (scope === 'workspace') return t('skills.scope.workspace')
  if (scope === 'user') return t('skills.scope.user')
  return t('skills.scope.builtin')
}

function groupSkillAgents(agentOptions: AgentViewDefinition[]) {
  return agentOptions.reduce<Array<{ group: string; agents: AgentViewDefinition[] }>>((groups, agent) => {
    const last = groups[groups.length - 1]
    if (last?.group === agent.groupKey) {
      last.agents.push(agent)
    } else {
      groups.push({ group: agent.groupKey, agents: [agent] })
    }
    return groups
  }, [])
}
