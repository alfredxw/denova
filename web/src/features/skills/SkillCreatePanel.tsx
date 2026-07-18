import { useState } from 'react'
import { Bot, FileCode2, Loader2, Plus, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Input } from '@/components/ui/input'
import { createSkill } from '@/lib/api'
import type { SkillDocument, SkillScope, SkillScopeInfo } from '@/lib/api'
import { AGENTS } from '@/features/agents/agent-registry'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'
import { Field, PreviewRow, SectionTitle, SkillAgentSelector } from './skill-form-fields'
import { scopeLabel, skillFilePath, skillNamePattern } from './skill-utils'

interface SkillCreatePanelProps {
  /** 可写 scope 列表；为空时展示不可写提示 */
  scopes: SkillScopeInfo[]
  defaultScope: SkillScope
  onCreated: (document: SkillDocument) => void | Promise<void>
  onAskAgent: () => void
}

/** 新建 Skill 整页表单：自持表单状态与提交，成功后回调宿主刷新列表。 */
export function SkillCreatePanel({ scopes, defaultScope, onCreated, onAskAgent }: SkillCreatePanelProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [scope, setScope] = useState<SkillScope>(defaultScope)
  const [agents, setAgents] = useState<VisibleAgentKey[]>(['ide'])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const trimmedName = name.trim()
  const invalidName = trimmedName !== '' && !skillNamePattern.test(trimmedName)
  const targetName = trimmedName || t('skills.create.namePlaceholder')
  const targetPath = skillFilePath(scopes.find((item) => item.scope === scope), targetName)

  const onCreate = async () => {
    if (!skillNamePattern.test(trimmedName)) {
      setError(t('skills.create.invalidName'))
      return
    }
    setSaving(true)
    setError(null)
    try {
      const document = await createSkill(scope, trimmedName, description.trim(), agents)
      await onCreated(document)
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
              <Plus className="h-4 w-4 text-[var(--nova-text-muted)]" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-sm font-semibold">{t('skills.create.title')}</h1>
              <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('skills.create.subtitle')}</div>
            </div>
          </div>
        </section>

        {error && <InlineErrorNotice message={error} title={t('skills.error')} />}

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
              <Field label={t('skills.create.description')}>
                <Input
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  aria-label={t('skills.create.description')}
                  placeholder={t('skills.create.descriptionPlaceholder')}
                  className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 outline-none"
                />
                <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('skills.create.descriptionHint')}</div>
              </Field>
            </section>

            <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
              <SectionTitle icon={Bot} title={t('skills.create.section.agents')} />
              <SkillAgentSelector agents={agents} onAgentsChange={setAgents} />
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
                  onClick={() => void onCreate()}
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
