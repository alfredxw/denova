import { useState } from 'react'
import { Bot, FileCode2, Loader2, Save, Settings2, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Input } from '@/components/ui/input'
import { saveSkillDocument } from '@/lib/api'
import type { SkillDocument, SkillScope, SkillScopeInfo } from '@/lib/api'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'
import { Field, PreviewRow, SectionTitle, SkillAgentSelector } from './skill-form-fields'
import { parseAgentKeys, scopeLabel, skillFilePath, skillNamePattern, updateSkillConfigContent } from './skill-utils'

interface SkillConfigPanelProps {
  document: SkillDocument
  /** 当前编辑器内容（可能含未保存修改），配置保存基于它重写 frontmatter */
  content: string
  /** 可写 scope 列表 */
  scopes: SkillScopeInfo[]
  onSaved: (document: SkillDocument) => void | Promise<void>
  onCancel: () => void
  onDelete: () => void
}

/** 配置 Skill 整页表单：改名/迁移 scope/触发说明/可用 Agent，自持表单状态。 */
export function SkillConfigPanel({ document, content, scopes, onSaved, onCancel, onDelete }: SkillConfigPanelProps) {
  const { t } = useTranslation()
  const [name, setName] = useState(document.name)
  const [scope, setScope] = useState<SkillScope>(document.scope)
  const [description, setDescription] = useState(document.description)
  const [agents, setAgents] = useState<VisibleAgentKey[]>(() => parseAgentKeys(document.agent))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const trimmedName = name.trim()
  const invalidName = trimmedName !== '' && !skillNamePattern.test(trimmedName)
  const trimmedDescription = description.trim()
  const targetName = trimmedName || document.name
  const targetPath = skillFilePath(scopes.find((item) => item.scope === scope), targetName)
  const targetWritable = scopes.some((item) => item.scope === scope)

  const onSave = async () => {
    if (!document.editable) return
    if (!skillNamePattern.test(trimmedName)) {
      setError(t('skills.create.invalidName'))
      return
    }
    if (!targetWritable) {
      setError(t('skills.config.scopeRequired'))
      return
    }
    if (!trimmedDescription) {
      setError(t('skills.config.descriptionRequired'))
      return
    }
    setSaving(true)
    setError(null)
    try {
      const nextContent = updateSkillConfigContent(content, trimmedName, trimmedDescription, agents)
      const saved = await saveSkillDocument(document.scope, document.name, nextContent, { scope, name: trimmedName })
      await onSaved(saved)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
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

        {error && <InlineErrorNotice message={error} title={t('skills.error')} />}

        <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
          <SectionTitle icon={FileCode2} title={t('skills.create.section.identity')} />
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <Field label={t('skills.create.scope')}>
              <div className="flex gap-1">
                {scopes.map((item) => (
                  <button
                    key={item.scope}
                    type="button"
                    onClick={() => setScope(item.scope)}
                    className={`nova-nav-item h-8 flex-1 rounded-[var(--nova-radius)] px-2 ${scope === item.scope ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
                  >
                    {scopeLabel(item.scope, t)}
                  </button>
                ))}
              </div>
            </Field>
            <Field label={t('skills.create.name')}>
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
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
          <div className="grid gap-2 md:grid-cols-2">
            <PreviewRow label={t('skills.create.preview.command')} value={`/${targetName}`} />
            <PreviewRow label={t('skills.create.preview.scope')} value={scopeLabel(scope, t)} />
            <PreviewRow label={t('skills.create.preview.path')} value={targetPath || t('skills.agent.pathFallback')} wide />
          </div>
          <Field label={t('skills.create.description')}>
            <Input
              value={description}
              onChange={(event) => setDescription(event.target.value)}
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
          <SkillAgentSelector agents={agents} onAgentsChange={setAgents} />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
            {agents.length === 0 ? t('skills.create.agentsAllHint') : t('skills.create.agentsHint')}
          </div>
        </section>

        <section className="flex flex-wrap gap-2 pb-5">
          <button
            type="button"
            onClick={() => void onSave()}
            disabled={saving || !trimmedName || invalidName || !trimmedDescription || !targetWritable}
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
