import { useCallback, useEffect, useMemo, useState } from 'react'
import { Bot, CheckCircle2, FileCode2, Lock, Plus, RefreshCw, Save, Sparkles, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Textarea } from '@/components/ui/textarea'
import { createSkill, getSkillDocument, getSkills, saveSkillDocument } from '@/lib/api'
import type { SkillDocument, SkillScope, SkillScopeInfo, SkillSnapshot, SkillSummary } from '@/lib/api'

const skillNamePattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/
const scopes: SkillScope[] = ['workspace', 'user', 'builtin']

interface SkillsViewProps {
  workspace: string
  onClose?: () => void
  onRequestAgent?: (prompt: string) => void
}

export function SkillsView({ workspace, onClose, onRequestAgent }: SkillsViewProps) {
  const { t } = useTranslation()
  const [snapshot, setSnapshot] = useState<SkillSnapshot>({ scopes: [], skills: [] })
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [document, setDocument] = useState<SkillDocument | null>(null)
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [newScope, setNewScope] = useState<SkillScope>('workspace')
  const [newName, setNewName] = useState('')
  const [newDescription, setNewDescription] = useState('')

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
      const doc = await createSkill(newScope, name, newDescription.trim())
      setNewName('')
      setNewDescription('')
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

  const askAgent = () => {
    const targetName = document?.name || newName.trim() || 'new-skill'
    const scope = document?.scope || newScope
    const targetPath = skillFilePath(snapshot.scopes.find((item) => item.scope === scope), targetName)
    onRequestAgent?.(t('skills.agent.prompt', {
      name: targetName,
      scope: scopeLabel(scope, t),
      path: targetPath || t('skills.agent.pathFallback'),
    }))
  }

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
          onClick={askAgent}
          className="nova-nav-item inline-flex items-center gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1"
        >
          <Bot className="h-3.5 w-3.5" />
          {t('skills.agent.button')}
        </button>
        <button
          type="button"
          onClick={() => void onSave()}
          disabled={!dirty || saving || !document?.editable}
          className="nova-nav-item inline-flex items-center gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-active)] px-2.5 py-1 disabled:cursor-not-allowed disabled:opacity-45"
        >
          <Save className="h-3.5 w-3.5" />
          {saving ? t('common.saving') : t('common.save')}
        </button>
        {onClose && (
          <button type="button" onClick={onClose} className="nova-nav-item rounded p-1" aria-label={t('common.close')} title={t('common.close')}>
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {error && <InlineErrorNotice className="mx-3 mt-2" message={error} title={t('skills.error')} />}

      <div className="grid min-h-0 flex-1 grid-cols-[20rem_minmax(0,1fr)] text-xs">
        <aside className="min-h-0 overflow-y-auto border-r border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <section className="mb-4 rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
            <div className="mb-2 flex items-center gap-2 font-medium text-[var(--nova-text)]">
              <Plus className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
              {t('skills.create.title')}
            </div>
            <div className="flex gap-1">
              {writableScopes.map((scope) => (
                <button
                  key={scope.scope}
                  type="button"
                  onClick={() => setNewScope(scope.scope)}
                  className={`nova-nav-item flex-1 rounded px-2 py-1 ${newScope === scope.scope ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
                >
                  {scopeLabel(scope.scope, t)}
                </button>
              ))}
            </div>
            <input
              value={newName}
              onChange={(event) => setNewName(event.target.value)}
              placeholder={t('skills.create.namePlaceholder')}
              className="nova-field mt-2 h-8 w-full rounded border px-2 font-mono outline-none"
            />
            <input
              value={newDescription}
              onChange={(event) => setNewDescription(event.target.value)}
              placeholder={t('skills.create.descriptionPlaceholder')}
              className="nova-field mt-2 h-8 w-full rounded border px-2 outline-none"
            />
            <button
              type="button"
              onClick={() => void onCreate()}
              disabled={saving || writableScopes.length === 0 || !newName.trim()}
              className="nova-nav-item mt-2 inline-flex h-8 w-full items-center justify-center gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-active)] disabled:cursor-not-allowed disabled:opacity-45"
            >
              <Plus className="h-3.5 w-3.5" />
              {t('skills.create.submit')}
            </button>
          </section>

          <div className="space-y-4">
            {scopes.map((scope) => (
              <SkillScopeList
                key={scope}
                scope={scope}
                scopeInfo={snapshot.scopes.find((item) => item.scope === scope)}
                skills={snapshot.skills.filter((skill) => skill.scope === scope)}
                selectedKey={selectedKey}
                onSelect={setSelectedKey}
              />
            ))}
          </div>
        </aside>

        <main className="flex min-h-0 flex-col">
          {document ? (
            <>
              <div className="flex min-h-12 shrink-0 items-center gap-3 border-b border-[var(--nova-border)] px-4">
                <FileCode2 className="h-4 w-4 text-[var(--nova-text-muted)]" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm text-[var(--nova-text)]">/{document.name}</span>
                    <span className="rounded bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{scopeLabel(document.scope, t)}</span>
                    {!document.active && <span className="rounded bg-[#6f5d2a]/30 px-1.5 py-0.5 text-[10px] text-[#e7c36a]">{t('skills.shadowed')}</span>}
                    {!document.editable && <Lock className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />}
                  </div>
                  <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]" title={document.path}>{document.path}</div>
                </div>
                {dirty && <span className="text-[11px] text-[#e7c36a]">{t('skills.unsaved')}</span>}
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
      </div>
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
                  {skill.active ? <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-[#74c69d]" /> : <span className="shrink-0 text-[10px] text-[#e7c36a]">{t('skills.shadowed')}</span>}
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

function keyOf(skill: Pick<SkillSummary, 'scope' | 'name'>) {
  return `${skill.scope}:${skill.name}`
}

function skillFilePath(scope: SkillScopeInfo | undefined, name: string) {
  if (!scope?.path) return ''
  return `${scope.path.replace(/\/+$/, '')}/${name}/SKILL.md`
}

function scopeLabel(scope: SkillScope, t: (key: string) => string) {
  if (scope === 'workspace') return t('skills.scope.workspace')
  if (scope === 'user') return t('skills.scope.user')
  return t('skills.scope.builtin')
}
