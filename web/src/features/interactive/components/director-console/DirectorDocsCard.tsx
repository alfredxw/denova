import { useEffect, useState } from 'react'
import { ChevronRight, Edit3, Eye, FileText, Loader2, RefreshCw, Save } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MarkdownRenderer, type MarkdownRendererComponents } from '@/components/common/MarkdownRenderer'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import type { DirectorPlan, DirectorPlanDocs } from '../../types'
import type { DirectorStatusLike } from './types'
import { directorStatusLabel, formatBytes } from './utils'

// 导演节拍表文档卡：预览模式下三份文档按可折叠分区展示，避免窄面板里的滚动套滚动。
export function DirectorDocsCard({ storyId, directorPlan, draftDocs, onDraftDocsChange, directorStatus, loading, saving, rebuilding, onSave, onRebuild }: {
  storyId?: string
  directorPlan: DirectorPlan | null
  draftDocs: DirectorPlanDocs | null
  onDraftDocsChange: (docs: DirectorPlanDocs) => void
  directorStatus?: DirectorStatusLike
  loading: boolean
  saving: boolean
  rebuilding: boolean
  onSave: () => void
  onRebuild: () => void
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)

  useEffect(() => {
    setEditing(false)
  }, [directorPlan?.metadata?.revision])

  return (
    <section className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <div className="flex min-w-0 items-start justify-between gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3">
        <div className="flex min-w-0 items-start gap-2.5">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]">
            <FileText className="h-3.5 w-3.5" />
          </span>
          <div className="min-w-0">
            <h3 className="director-console__display truncate text-sm font-semibold text-[var(--nova-text)]">{t('directorPanel.planTitle')}</h3>
            <p className="mt-1 truncate text-[9px] uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">{directorStatusLabel(directorStatus, loading, t)}</p>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap justify-end gap-1">
          <Button type="button" variant="outline" size="xs" aria-label={editing ? t('directorPanel.plan.preview') : t('directorPanel.plan.edit')} title={editing ? t('directorPanel.plan.preview') : t('directorPanel.plan.edit')} className="h-7 gap-1.5 rounded-[8px] border-[var(--nova-border)] bg-[var(--director-panel)] px-2 text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!draftDocs} onClick={() => setEditing((value) => !value)}>
            {editing ? <Eye className="h-3 w-3" /> : <Edit3 className="h-3 w-3" />}
            <span className="director-plan-action-label">{editing ? t('directorPanel.plan.preview') : t('directorPanel.plan.edit')}</span>
          </Button>
          {editing ? (
            <Button type="button" variant="outline" size="xs" aria-label={saving ? t('common.saving') : t('common.save')} title={saving ? t('common.saving') : t('common.save')} className="h-7 gap-1.5 rounded-[8px] border-[var(--nova-border)] bg-[var(--director-panel)] px-2 text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!storyId || !draftDocs || !directorPlan || saving} onClick={onSave}>
              {saving ? <Loader2 className="h-3 w-3 animate-spin" /> : <Save className="h-3 w-3" />}
              <span className="director-plan-action-label">{saving ? t('common.saving') : t('common.save')}</span>
            </Button>
          ) : null}
          <Button type="button" variant="outline" size="xs" aria-label={rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')} title={rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')} className="h-7 gap-1.5 rounded-[8px] border-[var(--nova-border)] bg-[var(--director-panel)] px-2 text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!storyId || rebuilding} onClick={onRebuild}>
            {rebuilding ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
            <span className="director-plan-action-label">{rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')}</span>
          </Button>
        </div>
      </div>
      <div className="p-3">
        {draftDocs ? (
          editing ? (
            <div className="space-y-4">
              <DirectorPlanTextarea label={t('snapshot.director.plan')} value={draftDocs.plan} onChange={(value) => onDraftDocsChange({ ...draftDocs, plan: value })} />
              <DirectorPlanTextarea label={t('snapshot.director.agentBrief')} value={draftDocs.agent_brief || ''} onChange={(value) => onDraftDocsChange({ ...draftDocs, agent_brief: value })} />
              <DirectorPlanTextarea label={t('snapshot.director.loreContext')} value={draftDocs.lore_context || ''} onChange={(value) => onDraftDocsChange({ ...draftDocs, lore_context: value })} />
            </div>
          ) : (
            <div className="divide-y divide-[var(--nova-border-soft)]">
              <DirectorDocumentSection title={t('snapshot.director.plan')} docKey="plan" content={draftDocs.plan} bytes={directorPlan?.metadata?.docs?.plan?.bytes} testId="director-plan-markdown" defaultOpen />
              <DirectorDocumentSection title={t('snapshot.director.agentBrief')} docKey="agent_brief" content={draftDocs.agent_brief || ''} bytes={directorPlan?.metadata?.docs?.agent_brief?.bytes} testId="director-agent-brief-markdown" />
              <DirectorDocumentSection title={t('snapshot.director.loreContext')} docKey="lore_context" content={draftDocs.lore_context || ''} bytes={directorPlan?.metadata?.docs?.lore_context?.bytes} testId="director-lore-context-markdown" />
            </div>
          )
        ) : (
          <div className="flex min-h-[220px] items-center justify-center rounded-[10px] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('directorPanel.directorEmpty')}</div>
        )}
      </div>
    </section>
  )
}

function DirectorDocumentSection({ title, docKey, content, bytes, testId, defaultOpen = false }: { title: string; docKey: string; content: string; bytes?: number; testId: string; defaultOpen?: boolean }) {
  const { t } = useTranslation()
  return (
    <Collapsible defaultOpen={defaultOpen} className="py-1 first:pt-0 last:pb-0">
      <CollapsibleTrigger className="group flex w-full min-w-0 items-center gap-2 rounded-[8px] px-1 py-2 text-left transition-colors hover:bg-[var(--nova-hover)]" aria-label={title}>
        <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform group-data-[state=open]:rotate-90" />
        <span className="min-w-0 truncate text-[10px] font-medium uppercase tracking-[0.1em] text-[var(--nova-text-faint)]">{title}</span>
        <span className="ml-auto shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{formatBytes(bytes ?? content.length)}</span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div data-testid={testId} data-doc-key={docKey} className="director-plan-sheet mb-2 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-4 text-xs leading-5 text-[var(--nova-text)]">
          {content.trim() ? (
            <MarkdownRenderer content={content} components={directorMarkdownComponents} />
          ) : (
            <div className="flex min-h-[120px] items-center justify-center text-center text-[var(--nova-text-muted)]">{t('snapshot.director.documentEmpty')}</div>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

const directorMarkdownComponents: MarkdownRendererComponents = {
  h1: ({ children }) => <h1 className="director-console__display mb-4 break-words text-lg font-semibold leading-7 text-[var(--nova-text)] [overflow-wrap:anywhere]">{children}</h1>,
  h2: ({ children }) => <h2 className="director-console__display mb-2 mt-5 break-words border-l-2 border-[var(--director-brass)] pl-3 text-[15px] font-semibold leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1.5 mt-3 break-words text-xs font-semibold leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{children}</h3>,
  h4: ({ children }) => <h4 className="mb-1 mt-3 break-words text-xs font-semibold leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{children}</h4>,
  p: ({ children }) => <p className="my-2 break-words text-xs leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{children}</p>,
  ul: ({ children }) => <ul className="my-2 list-disc space-y-1 pl-5 text-xs leading-5 text-[var(--nova-text)]">{children}</ul>,
  ol: ({ children }) => <ol className="my-2 list-decimal space-y-1 pl-5 text-xs leading-5 text-[var(--nova-text)]">{children}</ol>,
  li: ({ children }) => <li className="break-words pl-1 [overflow-wrap:anywhere]">{children}</li>,
  blockquote: ({ children }) => <blockquote className="my-3 border-l-2 border-[var(--nova-warning)]/70 pl-3 text-[var(--nova-text-muted)]">{children}</blockquote>,
  code: ({ children }) => <code className="rounded-[5px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1 py-0.5 font-mono text-[11px] text-[var(--nova-text)]">{children}</code>,
  pre: ({ children }) => <pre className="my-3 overflow-x-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 text-[11px] leading-5 text-[var(--nova-text)]">{children}</pre>,
  hr: () => <hr className="my-4 border-[var(--nova-border)]" />,
}

function DirectorPlanTextarea({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label className="block">
      <span className="mb-1 block text-[11px] font-medium text-[var(--nova-text-faint)]">{label}</span>
      <textarea
        className="min-h-[320px] w-full resize-y rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3 font-mono text-[11px] leading-5 text-[var(--nova-text)] outline-none transition-colors focus:border-[var(--director-brass)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--director-brass)_16%,transparent)]"
        value={value}
        spellCheck={false}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}
