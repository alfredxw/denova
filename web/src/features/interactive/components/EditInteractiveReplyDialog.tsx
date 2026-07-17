import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'

interface EditInteractiveReplyDialogProps {
  turnId: string
  initialContent: string
  onClose: () => void
  onSave: (content: string) => Promise<void>
}

/** Prose-only editor for a persisted Game turn; state and turn regeneration stay outside this component. */
export function EditInteractiveReplyDialog({ turnId, initialContent, onClose, onSave }: EditInteractiveReplyDialogProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(initialContent)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const normalizedDraft = normalizeReplyContent(draft)
  const unchanged = normalizedDraft === normalizeReplyContent(initialContent)

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
        className="max-h-[min(88dvh,720px)] max-w-[min(calc(100vw-2rem),760px)] gap-0 overflow-hidden border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)]"
      >
        <form
          className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)_auto]"
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
        >
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
            <label htmlFor={`interactive-reply-${turnId}`} className="block text-xs font-medium text-[var(--nova-text-muted)]">
              {t('storyStage.replyEdit.fieldLabel')}
            </label>
            <Textarea
              id={`interactive-reply-${turnId}`}
              autoFocus
              autoResize={false}
              value={draft}
              disabled={saving}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
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
            <Button type="submit" disabled={saving || !normalizedDraft || unchanged}>
              {saving ? t('common.saving') : t('common.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function normalizeReplyContent(value: string) {
  return value.replace(/\r\n?/g, '\n').trim()
}
