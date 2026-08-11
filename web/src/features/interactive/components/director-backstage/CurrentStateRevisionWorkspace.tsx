import { useEffect, useMemo, useState } from 'react'
import { ArrowLeft, Check, History, Redo2, Save, Undo2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { createInteractiveStateRevision, restoreInteractiveStateRevision, undoInteractiveStateRevision } from '../../api'
import type { ActorStateOp, Snapshot, StateOp, StateRevisionEvent } from '../../types'
import { actorName, humanizeStateKey } from '../story-state/model'

interface DraftField {
  id: string
  label: string
  original: unknown
  value: string
  path?: string
  actorId?: string
  fieldId?: string
}

interface StateChange {
  field: DraftField
  value: unknown
}

export function CurrentStateRevisionWorkspace({ storyId, branchId, snapshot, onBack, onSaved }: {
  storyId: string
  branchId: string
  snapshot: Snapshot
  onBack: () => void
  onSaved?: () => void | Promise<unknown>
}) {
  const { t, i18n } = useTranslation()
  const [fields, setFields] = useState(() => editableStateFields(snapshot.state))
  const [reviewing, setReviewing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [discardOpen, setDiscardOpen] = useState(false)
  const [view, setView] = useState<'edit' | 'history'>('edit')
  const [historyActionId, setHistoryActionId] = useState('')
  const parsed = useMemo(() => changedFields(fields, t), [fields, t])
  const changes = parsed.changes
  const dirty = fields.some((field) => field.value !== serializeValue(field.original))
  const currentHeadID = snapshot.head_id ?? ''

  const requestBack = () => {
    if (dirty) {
      setDiscardOpen(true)
      return
    }
    onBack()
  }

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || discardOpen) return
      event.preventDefault()
      requestBack()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  })

  const save = async () => {
    if (!snapshot.current_turn?.id || !currentHeadID || changes.length === 0 || parsed.error) return
    setSaving(true)
    setError('')
    const ops: StateOp[] = []
    const actorOps: ActorStateOp[] = []
    for (const change of changes) {
      if (change.field.actorId && change.field.fieldId) {
        actorOps.push({
          op: 'set',
          actor_id: change.field.actorId,
          field_id: change.field.fieldId,
          value: change.value,
          reason: 'Manual state revision',
        })
      } else if (change.field.path) {
        ops.push({ op: 'set', path: change.field.path, value: change.value, reason: 'Manual state revision' })
      }
    }
    try {
      await createInteractiveStateRevision(storyId, {
        branch_id: branchId,
        expected_head_id: currentHeadID,
        base_turn_id: snapshot.current_turn.id,
        source: 'manual_state_editor',
        ops,
        actor_ops: actorOps,
      })
      await onSaved?.()
      onBack()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('directorPanel.stateRevision.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const undoRevision = async (revision: StateRevisionEvent) => {
    setHistoryActionId(revision.id)
    setError('')
    try {
      await undoInteractiveStateRevision(storyId, revision.id, {
        branch_id: branchId,
        expected_head_id: currentHeadID,
        source: 'manual_state_editor',
      })
      await onSaved?.()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('directorPanel.stateRevision.actionFailed'))
    } finally {
      setHistoryActionId('')
    }
  }

  const restoreRevision = async (revision: StateRevisionEvent) => {
    setHistoryActionId(revision.id)
    setError('')
    try {
      await restoreInteractiveStateRevision(storyId, revision.id, {
        branch_id: branchId,
        expected_head_id: currentHeadID,
        source: 'manual_state_editor',
      })
      await onSaved?.()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('directorPanel.stateRevision.actionFailed'))
    } finally {
      setHistoryActionId('')
    }
  }

  const appliedRevisions = (snapshot.state_revisions || []).filter((revision) => revision.action === 'apply').reverse()
  const revisionActions = latestRevisionActions(snapshot.state_revisions || [])

  return (
    <section className="flex h-full min-h-0 flex-col bg-[var(--director-canvas)] text-[var(--nova-text)]" aria-labelledby="state-revision-title">
      <header className="flex min-w-0 items-center gap-3 border-b border-[var(--nova-border)] px-4 py-3">
        <Button type="button" variant="ghost" size="icon-sm" aria-label={t('directorPanel.stateRevision.back')} onClick={requestBack}>
          <ArrowLeft className="size-4" />
        </Button>
        <div className="min-w-0 flex-1">
          <h2 id="state-revision-title" className="truncate text-sm font-semibold">{t('directorPanel.stateRevision.title')}</h2>
          <p className="truncate text-xs text-[var(--nova-text-faint)]">{t('directorPanel.branch', { branch: branchId })}</p>
        </div>
      </header>

      <Tabs value={view} onValueChange={(value) => setView(value as 'edit' | 'history')} className="shrink-0 border-b border-[var(--nova-border)] px-4 py-2">
        <TabsList aria-label={t('directorPanel.stateRevision.views')}>
          <TabsTrigger value="edit">{t('directorPanel.stateRevision.editTab')}</TabsTrigger>
          <TabsTrigger value="history">{t('directorPanel.stateRevision.historyTab')}</TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        <div className="mx-auto max-w-2xl">
          {error ? <p role="alert" className="mb-4 border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{error}</p> : null}
          {parsed.error ? <p role="alert" className="mb-4 border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{parsed.error}</p> : null}

          {view === 'history' ? (
            <div>
              <h3 className="text-sm font-semibold">{t('directorPanel.stateRevision.historyTitle')}</h3>
              <p className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">{t('directorPanel.stateRevision.historyHint')}</p>
              {appliedRevisions.length === 0 ? (
                <p className="mt-4 border-y border-dashed border-[var(--nova-border)] py-10 text-center text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateRevision.historyEmpty')}</p>
              ) : (
                <ol className="mt-4 divide-y divide-[var(--nova-border)] border-y border-[var(--nova-border)]">
                  {appliedRevisions.map((revision) => {
                    const undone = revisionActions.get(revision.id) === 'undo'
                    return <li key={revision.id} className="py-3">
                      <div className="flex min-w-0 items-start gap-3">
                        <History className="mt-0.5 size-4 shrink-0 text-[var(--nova-text-faint)]" />
                        <div className="min-w-0 flex-1">
                          <p className="text-xs font-medium">{t('directorPanel.stateRevision.historyEntry')}</p>
                          <p className="mt-0.5 text-[11px] text-[var(--nova-text-faint)]">{new Intl.DateTimeFormat(i18n.language?.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(revision.ts))}</p>
                          <ul className="mt-2 space-y-1 text-xs text-[var(--nova-text-muted)]">
                            {revisionChangeLabels(revision).map((label) => <li key={label}>{label}</li>)}
                          </ul>
                        </div>
                        <Button type="button" variant="outline" size="sm" disabled={historyActionId !== ''} onClick={() => void (undone ? restoreRevision(revision) : undoRevision(revision))}>
                          {undone ? <Redo2 className="size-4" /> : <Undo2 className="size-4" />}
                          {t(undone ? 'directorPanel.stateRevision.restore' : 'directorPanel.stateRevision.undo')}
                        </Button>
                      </div>
                    </li>
                  })}
                </ol>
              )}
            </div>
          ) : reviewing ? (
            <div>
              <h3 className="text-sm font-semibold">{t('directorPanel.stateRevision.summaryTitle')}</h3>
              <p className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">{t('directorPanel.stateRevision.summaryHint')}</p>
              <ol className="mt-4 divide-y divide-[var(--nova-border)] border-y border-[var(--nova-border)]">
                {changes.map((change) => (
                  <li key={change.field.id} className="py-3">
                    <p className="text-xs font-medium">{change.field.label}</p>
                    <dl className="mt-2 grid gap-2 text-xs sm:grid-cols-2">
                      <div><dt className="text-[var(--nova-text-faint)]">{t('directorPanel.stateRevision.before')}</dt><dd className="mt-1 break-words">{displayValue(change.field.original)}</dd></div>
                      <div><dt className="text-[var(--nova-text-faint)]">{t('directorPanel.stateRevision.after')}</dt><dd className="mt-1 break-words">{displayValue(change.value)}</dd></div>
                    </dl>
                  </li>
                ))}
              </ol>
            </div>
          ) : (
            <div className="space-y-4">
              <div>
                <h3 className="text-sm font-semibold">{t('directorPanel.stateRevision.editTitle')}</h3>
                <p className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">{t('directorPanel.stateRevision.editHint')}</p>
              </div>
              {fields.length === 0 ? (
                <p className="border-y border-dashed border-[var(--nova-border)] py-10 text-center text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateRevision.empty')}</p>
              ) : fields.map((field) => (
                <label key={field.id} className="block">
                  <span className="mb-1.5 block text-xs font-medium">{field.label}</span>
                  {typeof field.original === 'boolean' ? (
                    <Select value={field.value} onValueChange={(value) => setFields((current) => patchField(current, field.id, value))}>
                      <SelectTrigger aria-label={field.label} className="nova-field w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="true">{t('directorPanel.stateRevision.trueOption')}</SelectItem>
                        <SelectItem value="false">{t('directorPanel.stateRevision.falseOption')}</SelectItem>
                      </SelectContent>
                    </Select>
                  ) : isComplex(field.original) ? (
                    <Textarea aria-label={field.label} value={field.value} onChange={(event) => setFields((current) => patchField(current, field.id, event.target.value))} className="nova-field min-h-24 font-mono text-xs" />
                  ) : (
                    <Input aria-label={field.label} type={typeof field.original === 'number' ? 'number' : 'text'} value={field.value} onChange={(event) => setFields((current) => patchField(current, field.id, event.target.value))} className="nova-field" />
                  )}
                </label>
              ))}
            </div>
          )}
        </div>
      </div>

      {view === 'edit' ? <footer className="flex shrink-0 items-center justify-end gap-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3">
        {reviewing ? <Button type="button" variant="outline" onClick={() => setReviewing(false)}>{t('directorPanel.stateRevision.continueEditing')}</Button> : null}
        {reviewing ? (
          <Button type="button" disabled={saving || Boolean(parsed.error)} onClick={() => void save()}><Save className="size-4" />{saving ? t('directorPanel.stateRevision.saving') : t('directorPanel.stateRevision.save')}</Button>
        ) : (
          <Button type="button" disabled={changes.length === 0 || Boolean(parsed.error)} onClick={() => setReviewing(true)}><Check className="size-4" />{t('directorPanel.stateRevision.review')}</Button>
        )}
      </footer> : null}

      <AlertDialog open={discardOpen} onOpenChange={setDiscardOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('directorPanel.stateRevision.discardTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('directorPanel.stateRevision.discardDescription')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('directorPanel.stateRevision.keepEditing')}</AlertDialogCancel>
            <AlertDialogAction onClick={onBack}>{t('directorPanel.stateRevision.discard')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}

function editableStateFields(state: Record<string, unknown>): DraftField[] {
  const fields: DraftField[] = []
  for (const [key, value] of Object.entries(state)) {
    if (key === 'actors') {
      if (!isRecord(value)) continue
      for (const [actorId, actorValue] of Object.entries(value)) {
        if (!isRecord(actorValue)) continue
        const actorState = isRecord(actorValue.state) ? actorValue.state : Object.fromEntries(Object.entries(actorValue).filter(([field]) => !['name', 'role', 'template_id', 'traits'].includes(field)))
        for (const [fieldId, fieldValue] of Object.entries(actorState)) {
          fields.push({ id: `actor:${actorId}:${fieldId}`, label: `${actorName(actorId, actorValue)} / ${humanizeStateKey(fieldId)}`, original: fieldValue, value: serializeValue(fieldValue), actorId, fieldId })
        }
      }
      continue
    }
    if (key === 'actor_archives') continue
    appendWorldFields(fields, value, [key])
  }
  return fields
}

function appendWorldFields(fields: DraftField[], value: unknown, path: string[]) {
  if (isRecord(value)) {
    for (const [key, child] of Object.entries(value)) appendWorldFields(fields, child, [...path, key])
    return
  }
  const statePath = path.join('.')
  fields.push({ id: `state:${statePath}`, label: path.map(humanizeStateKey).join(' / '), original: value, value: serializeValue(value), path: statePath })
}

function changedFields(fields: DraftField[], t: (key: string, options?: Record<string, unknown>) => string): { changes: StateChange[]; error: string } {
  const changes: StateChange[] = []
  for (const field of fields) {
    if (field.value === serializeValue(field.original)) continue
    try {
      changes.push({ field, value: parseValue(field.value, field.original) })
    } catch {
      return { changes, error: t('directorPanel.stateRevision.invalidValue', { label: field.label }) }
    }
  }
  return { changes, error: '' }
}

function parseValue(value: string, original: unknown): unknown {
  if (typeof original === 'number') {
    const parsed = Number(value)
    if (!Number.isFinite(parsed)) throw new Error('invalid number')
    return parsed
  }
  if (typeof original === 'boolean') {
    if (value === 'true') return true
    if (value === 'false') return false
    throw new Error('invalid boolean')
  }
  if (isComplex(original)) return JSON.parse(value)
  return value
}

function serializeValue(value: unknown) {
  if (isComplex(value)) return JSON.stringify(value, null, 2)
  return String(value ?? '')
}

function displayValue(value: unknown) {
  return typeof value === 'string' ? value : JSON.stringify(value)
}

function patchField(fields: DraftField[], id: string, value: string) {
  return fields.map((field) => field.id === id ? { ...field, value } : field)
}

function isComplex(value: unknown) {
  return value !== null && typeof value === 'object'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function revisionChangeLabels(revision: StateRevisionEvent) {
  return [
    ...(revision.ops || []).map((op) => op.path.split('.').map(humanizeStateKey).join(' / ')),
    ...(revision.actor_ops || []).map((op) => `${humanizeStateKey(op.actor_id)} / ${humanizeStateKey(op.field_id)}`),
  ]
}

function latestRevisionActions(revisions: StateRevisionEvent[]) {
  const actions = new Map<string, StateRevisionEvent['action']>()
  for (const revision of revisions) {
    if (revision.source_revision_id && (revision.action === 'undo' || revision.action === 'restore')) actions.set(revision.source_revision_id, revision.action)
  }
  return actions
}
