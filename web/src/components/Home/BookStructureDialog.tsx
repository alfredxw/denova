import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { applyBookStructure, getProjectBookSummary, previewBookStructure } from '@/lib/api-client/project-book'
import type { BookRecord, ChapterStructure, StructurePreview } from '@/lib/api'
import { ChapterStructureSelector, ManuscriptPreviewTree } from './ChapterStructureSelector'

export function BookStructureDialog({ book, onClose, onBeforeApply, onApplied }: {
  book: Pick<BookRecord, 'project_id' | 'name'> | null
  onClose: () => void
  onBeforeApply?: () => Promise<boolean>
  onApplied: () => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [current, setCurrent] = useState<ChapterStructure>('chapters')
  const [target, setTarget] = useState<ChapterStructure>('chapters')
  const [preview, setPreview] = useState<StructurePreview | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!book) return
    let cancelled = false
    setBusy(true); setPreview(null); setError('')
    getProjectBookSummary(book.project_id).then((summary) => {
      if (!cancelled) { setCurrent(summary.structure || 'chapters'); setTarget(summary.structure || 'chapters') }
    }).catch((cause) => { if (!cancelled) setError(String(cause.message || cause)) })
      .finally(() => { if (!cancelled) setBusy(false) })
    return () => { cancelled = true }
  }, [book])

  const run = async () => {
    if (!book || busy) return
    setBusy(true); setError('')
    try {
      if (onBeforeApply && !(await onBeforeApply())) return
      if (!preview) setPreview(await previewBookStructure(book.project_id, target))
      else {
        await applyBookStructure(book.project_id, preview)
        await onApplied()
        onClose()
      }
    } catch (cause) {
      console.error('Book structure operation failed', { projectId: book.project_id, cause })
      setError(cause instanceof Error ? cause.message : String(cause))
      setPreview(null)
    } finally { setBusy(false) }
  }

  return <Dialog open={Boolean(book)} onOpenChange={(open) => { if (!open && !busy) onClose() }}>
    <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{t('bookStructure.change')}</DialogTitle>
        <DialogDescription className="truncate">{book?.name}</DialogDescription>
      </DialogHeader>
      <div className="flex min-h-0 flex-col gap-4 overflow-y-auto">
        <ChapterStructureSelector value={target} disabled={busy} showExample={!preview} onChange={(value) => { setTarget(value); setPreview(null); setError('') }} />
        {preview && <>
          <div className="grid min-w-0 gap-4 sm:grid-cols-2">
            <section className="flex min-w-0 flex-col gap-2 rounded-md border p-3" aria-label={t('bookStructure.before')}>
              <p className="text-sm font-medium">{t('bookStructure.before')}</p>
              <ManuscriptPreviewTree nodes={preview.before} />
            </section>
            <section className="flex min-w-0 flex-col gap-2 rounded-md border p-3" aria-label={t('bookStructure.after')}>
              <p className="text-sm font-medium">{t('bookStructure.after')}</p>
              <ManuscriptPreviewTree nodes={preview.after} />
            </section>
          </div>
          <p className="text-sm text-muted-foreground">{t('bookStructure.backup')}</p>
          {preview.merged_chapters > 0 ? <p className="text-sm text-muted-foreground">{t('bookStructure.merge', { count: preview.merged_chapters })}</p> : target === 'sections' ? <p className="text-sm text-muted-foreground">{t('bookStructure.wrap')}</p> : null}
        </>}
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      </div>
      <DialogFooter>
        <Button variant="outline" disabled={busy} onClick={onClose}>{t('common.cancel')}</Button>
        <Button disabled={busy || target === current} onClick={() => void run()}>{t(busy ? 'common.loading' : preview ? 'bookStructure.apply' : 'bookStructure.preview')}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
