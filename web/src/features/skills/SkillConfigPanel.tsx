import { useState } from 'react'
import { Bot, FileCode2, Loader2, Save, Settings2, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { FormSectionHeader } from '@/components/forms/form-section-header'
import { Button } from '@/components/ui/button'
import { saveSkillDocument } from '@/lib/api'
import type { SkillDocument, SkillScope, SkillScopeInfo } from '@/lib/api'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'
import { SkillAgentSelector } from './skill-form-fields'
import { SkillIdentityFields } from './SkillIdentityFields'
import { parseAgentKeys, skillFilePath, skillNamePattern, updateSkillConfigContent } from './skill-utils'

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

        <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
          <FormSectionHeader icon={FileCode2} title={t('skills.create.section.identity')} />
          <SkillIdentityFields
            scopes={scopes}
            scope={scope}
            onScopeChange={setScope}
            name={name}
            onNameChange={setName}
            description={description}
            onDescriptionChange={setDescription}
            invalidName={invalidName}
            descriptionRequired
            targetName={targetName}
            targetPath={targetPath}
            showPreview
          />
        </section>

        <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
          <FormSectionHeader icon={Bot} title={t('skills.create.section.agents')} />
          <SkillAgentSelector agents={agents} onAgentsChange={setAgents} />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
            {agents.length === 0 ? t('skills.create.agentsAllHint') : t('skills.create.agentsHint')}
          </div>
        </section>

        <section className="flex flex-wrap gap-2 pb-5">
          <Button
            type="button"
            size="sm"
            variant="secondary"
            onClick={() => void onSave()}
            disabled={saving || !trimmedName || invalidName || !trimmedDescription || !targetWritable}
            className="nova-nav-item h-8 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3"
          >
            {saving ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Save data-icon="inline-start" />}
            {t('skills.config.save')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onCancel}
            className="nova-nav-item h-8 rounded-[var(--nova-radius)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3"
          >
            {t('common.cancel')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="destructive"
            onClick={onDelete}
            disabled={saving}
            className="nova-nav-item ml-auto h-8 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3"
          >
            <Trash2 data-icon="inline-start" />
            {t('skills.delete.action')}
          </Button>
        </section>
      </div>
    </div>
  )
}
