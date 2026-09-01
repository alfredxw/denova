import { useState, type ReactNode } from 'react'
import { ChevronDown, LoaderCircle, Map, Pencil, Save, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { MarkdownViewToggle } from '@/components/common/MarkdownEditPreview'
import { ThemedMarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { BranchPlan } from '../../types'

const MAX_BRANCH_PLAN_BYTES = 64 * 1024

interface BranchPlanSummaryProps {
  plan?: BranchPlan
  planningEnabled: boolean
  editingDisabled?: boolean
  onUpdate?: (markdown: string, baseRevision: string) => void | Promise<void>
}

type DraftValidationError = 'empty' | 'too-large' | 'no-sections' | 'duplicate-sections' | null

/** Current future blueprint. Collapsed mode deliberately reveals no planned story content. */
export function BranchPlanSummary({ plan, planningEnabled, editingDisabled = false, onUpdate }: BranchPlanSummaryProps) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [preview, setPreview] = useState(false)
  const [draft, setDraft] = useState('')
  const [baseRevision, setBaseRevision] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const markdown = plan?.markdown.trim() || ''
  const updatedAt = formatUpdatedAt(plan?.updated_at, i18n.language)
  const sectionCount = branchPlanH2Headings(markdown).length
  const summary = markdown
    ? t('directorPanel.plan.sectionCount', { count: sectionCount })
    : t('directorPanel.plan.empty')

  const beginEditing = () => {
    if (!markdown || !onUpdate || editingDisabled) return
    setDraft(markdown)
    setBaseRevision(plan?.revision || '')
    setPreview(false)
    setSaveError('')
    setEditing(true)
  }

  const cancelEditing = () => {
    if (saving) return
    setEditing(false)
    setPreview(false)
    setDraft('')
    setSaveError('')
  }

  const saveDraft = async () => {
    const validationError = validateBranchPlanDraft(draft)
    if (validationError) {
      setSaveError(t(`directorPanel.plan.validation.${validationError}`))
      return
    }
    if (!onUpdate || saving || editingDisabled) return
    setSaving(true)
    setSaveError('')
    try {
      await onUpdate(draft.trim(), baseRevision)
      setEditing(false)
      setPreview(false)
      setDraft('')
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : t('directorPanel.plan.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  let editControl: ReactNode = null
  if (editing) {
    editControl = <MarkdownViewToggle preview={preview} onPreviewChange={setPreview} sourceLabel={t('directorPanel.plan.source')} />
  } else if (onUpdate) {
    editControl = (
      <Button
        type="button"
        variant="outline"
        size="xs"
        onClick={beginEditing}
        disabled={editingDisabled}
        title={editingDisabled ? t('directorPanel.plan.editBlocked') : t('common.edit')}
      >
        <Pencil data-icon="inline-start" />
        {t('common.edit')}
      </Button>
    )
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <CollapsibleTrigger asChild>
        <button type="button" className="flex w-full min-w-0 items-center gap-3 px-3 py-3 text-left outline-none transition-colors hover:bg-[var(--nova-hover)] focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--director-brass)]/45">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]"><Map className="size-4" /></span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-xs font-semibold text-[var(--nova-text)]">{t('directorPanel.plan.title')}</span>
            <span className="mt-0.5 block truncate text-[10px] text-[var(--nova-text-faint)]">
              {t(planningEnabled ? 'directorPanel.planning.enabled' : 'directorPanel.planning.disabled')} · {summary}
            </span>
          </span>
          <ChevronDown className={cn('size-4 shrink-0 text-[var(--nova-text-faint)] transition-transform motion-reduce:transition-none', open && 'rotate-180')} />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="border-t border-[var(--nova-border)] px-3 py-3">
          {markdown ? (
            <>
              <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
                <p className="min-w-0 flex-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">
                  {t(planningEnabled ? 'directorPanel.plan.description' : 'directorPanel.plan.disabledDescription')}
                </p>
                {editControl}
              </div>

              {editing ? (
                <div className="space-y-3">
                  {preview ? (
                    <div className="max-h-[52vh] min-h-64 overflow-y-auto rounded-lg border border-[var(--nova-border)] bg-[var(--nova-bg)] px-3 py-3">
                      <ThemedMarkdownRenderer content={draft} className="text-xs leading-6 text-[var(--nova-text-muted)]" />
                    </div>
                  ) : (
                    <Textarea
                      autoResize={false}
                      value={draft}
                      onChange={(event) => {
                        setDraft(event.target.value)
                        setSaveError('')
                      }}
                      disabled={saving}
                      aria-label={t('directorPanel.plan.editorLabel')}
                      spellCheck={false}
                      className="h-[min(52vh,34rem)] min-h-64 resize-y rounded-lg border-[var(--nova-border)] bg-[var(--nova-bg)] px-3 py-3 font-mono text-xs leading-5 text-[var(--nova-text)] shadow-none focus-visible:border-[var(--director-brass)] focus-visible:ring-[var(--director-brass)]/25"
                    />
                  )}
                  {editingDisabled ? <p className="text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.plan.editBlocked')}</p> : null}
                  {saveError ? <InlineErrorNotice title={t('directorPanel.plan.saveFailed')} message={saveError} /> : null}
                  <div className="flex flex-wrap justify-end gap-2 border-t border-[var(--nova-border-soft)] pt-3">
                    <Button type="button" variant="outline" size="sm" onClick={cancelEditing} disabled={saving}>
                      <X data-icon="inline-start" />
                      {t('common.cancel')}
                    </Button>
                    <Button type="button" size="sm" onClick={() => void saveDraft()} disabled={saving || editingDisabled || draft.trim() === markdown}>
                      {saving ? <LoaderCircle data-icon="inline-start" className="animate-spin motion-reduce:animate-none" /> : <Save data-icon="inline-start" />}
                      {t(saving ? 'common.saving' : 'common.save')}
                    </Button>
                  </div>
                </div>
              ) : (
                <>
                  <ThemedMarkdownRenderer content={markdown} className="text-xs leading-6 text-[var(--nova-text-muted)]" />
                  {updatedAt ? <p className="mt-3 border-t border-[var(--nova-border-soft)] pt-2 text-[9px] text-[var(--nova-text-faint)]">{t('directorPanel.plan.updatedAt', { time: updatedAt })}</p> : null}
                </>
              )}
            </>
          ) : (
            <p className="py-3 text-center text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.plan.emptyHint')}</p>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

export function validateBranchPlanDraft(markdown: string): DraftValidationError {
  const normalized = markdown.trim()
  if (!normalized) return 'empty'
  if (new TextEncoder().encode(normalized).byteLength > MAX_BRANCH_PLAN_BYTES) return 'too-large'
  const headings = branchPlanH2Headings(normalized)
  if (headings.length === 0) return 'no-sections'
  const normalizedHeadings = headings.map((heading) => heading.trim().replace(/\s+/g, ' ').toLowerCase())
  if (new Set(normalizedHeadings).size !== normalizedHeadings.length) return 'duplicate-sections'
  return null
}

export function branchPlanH2Headings(markdown: string): string[] {
  const headings: string[] = []
  let fenceMarker = ''
  let fenceLength = 0
  for (const line of markdown.replace(/\r\n?/g, '\n').split('\n')) {
    const fenceMatch = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(line)
    if (fenceMarker) {
      if (fenceMatch && fenceMatch[1][0] === fenceMarker && fenceMatch[1].length >= fenceLength && fenceMatch[2].trim() === '') {
        fenceMarker = ''
        fenceLength = 0
      }
      continue
    }
    if (fenceMatch) {
      fenceMarker = fenceMatch[1][0]
      fenceLength = fenceMatch[1].length
      continue
    }
    const rawHeading = /^ {0,3}##[\t ]+(.+)$/.exec(line)?.[1]
    const heading = normalizeATXHeading(rawHeading)
    if (heading) headings.push(heading)
  }
  return headings
}

function normalizeATXHeading(value: string | undefined): string {
  const heading = value?.trim() || ''
  const closingSequence = /^(.*?)[\t ]+#+[\t ]*$/.exec(heading)
  return (closingSequence?.[1] ?? heading).trim()
}

function formatUpdatedAt(value: string | undefined, locale: string): string {
  if (!value) return ''
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? '' : parsed.toLocaleString(locale)
}
