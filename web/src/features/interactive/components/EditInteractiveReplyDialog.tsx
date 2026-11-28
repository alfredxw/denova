import { useCallback, useRef, useState } from 'react'
import { Sparkles, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { InlineEditPopover } from '@/components/Editor/inline-edit/InlineEditPopover'
import type { InlineEditApplyAction } from '@/components/Editor/inline-edit/applyInlineEditToEditor'
import {
  INLINE_EDIT_AFTER_BUDGET,
  INLINE_EDIT_BEFORE_BUDGET,
  matchInlineEditLoreIds,
} from '@/components/Editor/inline-edit/inlineEditPresets'
import { createLoreEntityMatcher } from '@/features/lore/lore-entity-matcher'
import { useProjectLoreItems } from '@/features/lore/lore-lens-preferences'

interface EditInteractiveReplyDialogProps {
  turnId: string
  projectId?: string
  storyId?: string
  initialContent: string
  onClose: () => void
  onSave: (content: string) => Promise<void>
}

interface TextareaInlineEditSession {
  start: number
  end: number
  selection: string
  before: string
  after: string
  matchedLoreIds: string[]
}

/** Prose-only editor for a persisted Game turn; state and turn regeneration stay outside this component. */
export function EditInteractiveReplyDialog({
  turnId,
  projectId = '',
  storyId,
  initialContent,
  onClose,
  onSave,
}: EditInteractiveReplyDialogProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(initialContent)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [inlineEditSession, setInlineEditSession] = useState<TextareaInlineEditSession | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const loreItems = useProjectLoreItems(projectId)

  const normalizedDraft = normalizeReplyContent(draft)
  const unchanged = normalizedDraft === normalizeReplyContent(initialContent)
  const inlineEditAvailable = Boolean(projectId) && !saving

  const openInlineEdit = useCallback(() => {
    if (!inlineEditAvailable) return
    const el = textareaRef.current
    const start = el ? Math.min(el.selectionStart ?? draft.length, el.selectionEnd ?? draft.length) : 0
    const end = el ? Math.max(el.selectionStart ?? draft.length, el.selectionEnd ?? draft.length) : draft.length
    const selection = draft.slice(start, end)
    const before = draft.slice(Math.max(0, start - INLINE_EDIT_BEFORE_BUDGET), start)
    const after = draft.slice(end, end + INLINE_EDIT_AFTER_BUDGET)
    const matcher = loreItems.length > 0
      ? createLoreEntityMatcher(loreItems, { enabled: true, matchKeywords: true })
      : null
    const matchedLoreIds = matchInlineEditLoreIds(matcher, selection, before, after)
    setInlineEditSession({
      start,
      end,
      selection,
      before,
      after,
      matchedLoreIds,
    })
  }, [draft, inlineEditAvailable, loreItems])

  const applyInlineEdit = useCallback((text: string, action: InlineEditApplyAction) => {
    if (!inlineEditSession) return
    const { start, end } = inlineEditSession
    const normalized = text.replace(/\r\n/g, '\n')
    let nextDraft = draft
    let nextCaret = start + normalized.length

    if (action === 'insert_below') {
      const lineEnd = draft.indexOf('\n', end)
      const insertAt = lineEnd >= 0 ? lineEnd : draft.length
      const prefix = draft.slice(0, insertAt)
      const suffix = draft.slice(insertAt)
      const separator = prefix.endsWith('\n') || !prefix ? '\n' : '\n\n'
      nextDraft = `${prefix}${separator}${normalized}${suffix}`
      nextCaret = insertAt + separator.length + normalized.length
    } else {
      nextDraft = `${draft.slice(0, start)}${normalized}${draft.slice(end)}`
      nextCaret = start + normalized.length
    }

    setDraft(nextDraft)
    setInlineEditSession(null)
    window.requestAnimationFrame(() => {
      const el = textareaRef.current
      if (!el) return
      el.focus()
      el.setSelectionRange(nextCaret, nextCaret)
    })
  }, [draft, inlineEditSession])

  const closeInlineEdit = useCallback(() => {
    setInlineEditSession(null)
    window.requestAnimationFrame(() => {
      textareaRef.current?.focus()
    })
  }, [])

  const save = async () => {
    if (saving) return
    if (!normalizedDraft) {
      setError(t('storyStage.replyEdit.empty'))
      return
    }
    setSaving(true)
    setError('')
    try {
      await onSave(normalizedDraft)
      onClose()
    } catch (saveError) {
      console.warn('[interactive-turn-edit] save failed', { turnId, error: saveError })
      setError(saveError instanceof Error ? saveError.message : t('storyStage.replyEdit.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => {
      if (!open && !saving) onClose()
    }}>
      <DialogContent
        showCloseButton={false}
        className="max-h-[min(88dvh,760px)] max-w-[min(calc(100vw-2rem),760px)] gap-0 overflow-hidden border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)]"
      >
        <div className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)_auto]">
          <DialogHeader className="relative border-b border-[var(--nova-border)] px-4 py-3 pr-12 text-left">
            <DialogTitle>{t('storyStage.replyEdit.title')}</DialogTitle>
            <DialogDescription>{t('storyStage.replyEdit.description')}</DialogDescription>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="absolute right-2 top-2"
              disabled={saving}
              onClick={onClose}
              aria-label={t('common.close')}
            >
              <X className="h-4 w-4" />
            </Button>
          </DialogHeader>

          <div className="min-h-0 space-y-2 overflow-y-auto px-4 py-3">
            <div className="flex items-center justify-between gap-2">
              <label htmlFor={`interactive-reply-${turnId}`} className="block text-xs font-medium text-[var(--nova-text-muted)]">
                {t('storyStage.replyEdit.fieldLabel')}
              </label>
              {inlineEditAvailable ? (
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  onPointerDown={(event) => event.preventDefault()}
                  onClick={openInlineEdit}
                  disabled={Boolean(inlineEditSession)}
                  className="h-6 gap-1 px-2 text-[11px]"
                >
                  <Sparkles className="h-3 w-3 text-amber-500 dark:text-amber-400" />
                  {t('editor.inlineEdit.action')}
                </Button>
              ) : null}
            </div>

            {inlineEditSession && projectId ? (
              <InlineEditPopover
                projectId={projectId}
                mode="game_turn"
                storyId={storyId}
                layout="embedded"
                selection={inlineEditSession.selection}
                before={inlineEditSession.before}
                after={inlineEditSession.after}
                matchedLoreIds={inlineEditSession.matchedLoreIds}
                onApply={applyInlineEdit}
                onClose={closeInlineEdit}
              />
            ) : null}

            <Textarea
              ref={textareaRef}
              id={`interactive-reply-${turnId}`}
              autoFocus
              autoResize={false}
              value={draft}
              disabled={saving || Boolean(inlineEditSession)}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (!event.nativeEvent.isComposing && (event.metaKey || event.ctrlKey) && !event.shiftKey && !event.altKey && event.key.toLowerCase() === 'j' && inlineEditAvailable) {
                  event.preventDefault()
                  openInlineEdit()
                  return
                }
                if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
                  event.preventDefault()
                  void save()
                }
              }}
              className="nova-field h-[min(52dvh,420px)] min-h-48 resize-y rounded-lg px-3 py-2 text-sm leading-6 shadow-none focus-visible:ring-0"
            />
            <div className="flex min-h-5 flex-wrap items-start justify-between gap-2 text-[11px] text-[var(--nova-text-faint)]">
              <span>{t('storyStage.replyEdit.shortcut')}</span>
              <span>{t('storyStage.replyEdit.characterCount', { count: Array.from(draft).length })}</span>
            </div>
            {error ? (
              <div role="alert" className="rounded-md border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">
                {error}
              </div>
            ) : null}
          </div>

          <DialogFooter className="border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
            <Button type="button" variant="outline" disabled={saving} onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              disabled={saving || Boolean(inlineEditSession) || !normalizedDraft || unchanged}
              onClick={() => { void save() }}
            >
              {saving ? t('common.saving') : t('common.save')}
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function normalizeReplyContent(value: string) {
  return value.replace(/\r\n?/g, '\n').trim()
}
