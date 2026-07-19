import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useState } from 'react'
import { Bot, FileCode2, Loader2, Settings2, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { FormSectionHeader } from '@/components/forms/form-section-header'
import { Button } from '@/components/ui/button'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { getSkillDocument, saveSkillDocument } from '@/lib/api'
import type { SkillDocument, SkillScope, SkillScopeInfo } from '@/lib/api'
import { isRevisionConflict, saveWithRevisionRecovery } from '@/lib/revision-conflict'
import { rebaseTextWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { SkillAgentSelector } from './skill-form-fields'
import { SkillIdentityFields } from './SkillIdentityFields'
import { parseAgentKeys, skillFilePath, skillNamePattern, updateSkillConfigContent } from './skill-utils'

interface SkillConfigPanelProps {
  document: SkillDocument
  /** Current editor content. Configuration autosave rewrites only its frontmatter. */
  content: string
  scopes: SkillScopeInfo[]
  onUpdated: (document: SkillDocument) => void
  onIdentityChanged: (document: SkillDocument) => void | Promise<void>
  onCancel: () => void
  onDelete: () => void
}

export interface SkillConfigPanelHandle {
  flush: () => Promise<boolean>
}

interface SkillConfigAutosaveDraft {
  id: string
  updated_at?: string
  content: string
}

interface SkillConfigAutosaveSaved {
  updated_at?: string
  content: string
  document: SkillDocument
}

function configSignature(value: Partial<SkillConfigAutosaveDraft>) {
  return value.content || ''
}

/** Autosaves metadata while keeping directory rename/move as an explicit command. */
export const SkillConfigPanel = forwardRef<SkillConfigPanelHandle, SkillConfigPanelProps>(function SkillConfigPanel({
  document,
  content,
  scopes,
  onUpdated,
  onIdentityChanged,
  onCancel,
  onDelete,
}, ref) {
  const { t } = useTranslation()
  const [name, setName] = useState(document.name)
  const [scope, setScope] = useState<SkillScope>(document.scope)
  const [description, setDescription] = useState(document.description)
  const [agents, setAgents] = useState<VisibleAgentKey[]>(() => parseAgentKeys(document.agent))
  const [savingIdentity, setSavingIdentity] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const trimmedName = name.trim()
  const invalidName = trimmedName !== '' && !skillNamePattern.test(trimmedName)
  const trimmedDescription = description.trim()
  const targetName = trimmedName || document.name
  const targetPath = skillFilePath(scopes.find((item) => item.scope === scope), targetName)
  const targetWritable = scopes.some((item) => item.scope === scope)
  const identityChanged = trimmedName !== document.name || scope !== document.scope
  const configContent = useMemo(
    () => updateSkillConfigContent(content, document.name, trimmedDescription, agents),
    [agents, content, document.name, trimmedDescription],
  )
  const configDraft = useMemo<SkillConfigAutosaveDraft>(() => ({
    id: `${document.scope}:${document.name}:config`,
    updated_at: document.revision,
    content: configContent,
  }), [configContent, document.name, document.revision, document.scope])

  const configAutosave = useResourceAutosave<SkillConfigAutosaveDraft, SkillConfigAutosaveDraft, SkillConfigAutosaveSaved>({
    draft: configDraft,
    active: document.editable && !identityChanged,
    scopeKey: configDraft.id,
    valid: Boolean(trimmedDescription),
    makePayload: (value) => value,
    baselineFromSaved: (saved, submitted) => ({
      ...submitted,
      content: saved.content,
      updated_at: saved.updated_at,
    }),
    signature: configSignature,
    save: async (_id, payload, baseRevision) => {
      const saved = await saveSkillDocument(document.scope, document.name, payload.content, undefined, baseRevision)
      return { content: saved.content, document: saved, updated_at: saved.revision }
    },
    resolveConflict: async ({ error: saveError, baseline, draft: submitted }) => {
      if (!isRevisionConflict(saveError)) return null
      const latest = await getSkillDocument(document.scope, document.name)
      const rebasedContent = await rebaseTextWithRecovery({
        resource: 'skill_config',
        scope: document.scope,
        id: submitted.id,
        baseline: { revision: baseline?.updated_at, value: baseline?.content ?? latest.content },
        local: { revision: baseline?.updated_at, value: submitted.content },
        external: { revision: latest.revision, value: latest.content },
      })
      return {
        payload: {
          ...submitted,
          content: rebasedContent,
          updated_at: latest.revision,
        },
        baseRevision: latest.revision,
      }
    },
    onSaved: (saved) => onUpdated(saved.document),
  })

  useEffect(() => {
    configAutosave.resetBaseline({
      id: `${document.scope}:${document.name}:config`,
      updated_at: document.revision,
      content,
    })
  }, [configAutosave.resetBaseline, content, document.name, document.revision, document.scope])

  const flushConfigAutosave = useCallback(async (force = false) => {
    if (identityChanged) return true
    try {
      const pending = configAutosave.flushPending()
      if (pending) {
        await pending
      } else if (force || configAutosave.status === 'error') {
        await configAutosave.saveNow('manual')
      }
      return true
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : String(saveError))
      return false
    }
  }, [configAutosave.flushPending, configAutosave.saveNow, configAutosave.status, identityChanged])

  useImperativeHandle(ref, () => ({
    flush: () => flushConfigAutosave(false),
  }), [flushConfigAutosave])

  const applyIdentityChange = async () => {
    if (!document.editable || !identityChanged) return
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
    setSavingIdentity(true)
    setError(null)
    try {
      const nextContent = updateSkillConfigContent(content, trimmedName, trimmedDescription, agents)
      const target = { scope, name: trimmedName }
      let recoveryBaselineRevision = document.revision
      let latestRevision: string | undefined
      const saved = await saveWithRevisionRecovery({
        baseline: content,
        draft: nextContent,
        revision: document.revision,
        save: (nextDraft, revision) => saveSkillDocument(
          document.scope,
          document.name,
          nextDraft,
          target,
          revision,
        ),
        loadLatest: async () => {
          const latest = await getSkillDocument(document.scope, document.name)
          latestRevision = latest.revision
          return { value: latest.content, revision: latest.revision }
        },
        rebase: async (baseline, local, external) => {
          const merged = await rebaseTextWithRecovery({
            resource: 'skill_config',
            scope: document.scope,
            id: `${document.scope}:${document.name}:identity`,
            baseline: { revision: recoveryBaselineRevision, value: baseline },
            local: { revision: recoveryBaselineRevision, value: local },
            external: { revision: latestRevision, value: external },
          })
          recoveryBaselineRevision = latestRevision || recoveryBaselineRevision
          return merged
        },
      })
      await onIdentityChanged(saved)
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : String(saveError))
    } finally {
      setSavingIdentity(false)
    }
  }

  const cancel = async () => {
    if (!await flushConfigAutosave()) return
    onCancel()
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
        <section className="border-b border-[var(--nova-border)] pb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
              <Settings2 className="h-4 w-4 text-[var(--nova-text-muted)]" />
            </div>
            <div className="min-w-0 flex-1">
              <h1 className="truncate text-sm font-semibold">{t('skills.config.title')}</h1>
              <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('skills.config.subtitle')}</div>
            </div>
            {!identityChanged && (
              <AutosaveStatusIndicator
                status={configAutosave.status}
                error={configAutosave.error}
                onRetry={configAutosave.retry}
              />
            )}
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

        <section className="flex flex-wrap items-center gap-2 pb-5">
          {identityChanged && (
            <>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => void applyIdentityChange()}
                disabled={savingIdentity || !trimmedName || invalidName || !trimmedDescription || !targetWritable}
                className="nova-nav-item h-8 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3"
              >
                {savingIdentity && <Loader2 data-icon="inline-start" className="animate-spin" />}
                {t('skills.config.applyIdentity')}
              </Button>
              <span className="text-[11px] text-[var(--nova-warning)]">{t('skills.config.identityHint')}</span>
            </>
          )}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void cancel()}
            className="nova-nav-item h-8 rounded-[var(--nova-radius)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3"
          >
            {t('common.cancel')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="destructive"
            onClick={onDelete}
            disabled={savingIdentity}
            className="nova-nav-item ml-auto h-8 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3"
          >
            <Trash2 data-icon="inline-start" />
            {t('skills.delete.action')}
          </Button>
        </section>
      </div>
    </div>
  )
})
