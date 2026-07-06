import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Folder, Image as ImageIcon, Loader2, Sparkles, Upload } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  createBook,
  generateBookCover,
  getBookInfo,
  updateBookInfo,
  uploadBookCover,
  type BookMeta,
  type BookRecord,
} from '@/lib/api'
import type { ImagePreset } from '@/features/interactive/types'
import { BookCoverThumbnail } from './BookCoverThumbnail'

type BookFormMode = 'create' | 'edit'

interface BookFormDialogProps {
  open: boolean
  mode: BookFormMode
  book: BookRecord | null
  novaDir: string
  imagePresetOptions: ImagePreset[]
  defaultImagePresetId: string
  coverVersion: (book: Pick<BookRecord, 'path' | 'cover_updated_at'>) => string
  onOpenChange: (open: boolean) => void
  onSwitch: (path: string) => void
  onBooksChange: () => void
  onCoverUpdated: (path: string, version: string) => void
}

const inputCls = 'nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'
const ghostButtonCls = 'nova-nav-item border border-transparent bg-transparent text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const primaryButtonCls = 'border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]'

export function BookFormDialog({
  open,
  mode,
  book,
  novaDir,
  imagePresetOptions,
  defaultImagePresetId,
  coverVersion,
  onOpenChange,
  onSwitch,
  onBooksChange,
  onCoverUpdated,
}: BookFormDialogProps) {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [createdPath, setCreatedPath] = useState('')
  const [title, setTitle] = useState('')
  const [author, setAuthor] = useState('')
  const [description, setDescription] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [coverPresetId, setCoverPresetId] = useState(defaultImagePresetId || 'game-cg')
  const [coverInstruction, setCoverInstruction] = useState('')
  const [coverBusy, setCoverBusy] = useState<'upload' | 'generate' | ''>('')
  const [coverError, setCoverError] = useState('')
  const [pendingCoverFile, setPendingCoverFile] = useState<File | null>(null)
  const [pendingCoverPreview, setPendingCoverPreview] = useState('')

  const activePath = mode === 'edit' ? book?.path || '' : createdPath
  const previewBook = useMemo(() => {
    if (!activePath) return null
    return {
      name: title.trim() || book?.name || t('home.unnamedBook'),
      path: activePath,
      cover_updated_at: book?.cover_updated_at,
    }
  }, [activePath, book?.cover_updated_at, book?.name, title, t])
  const busy = saving || Boolean(coverBusy)
  const footerPrimaryLabel = mode === 'create' && !createdPath ? t('common.create') : t('common.save')
  const footerBusyLabel = mode === 'create' && !createdPath ? t('common.creating') : t('common.saving')

  useEffect(() => {
    if (!open) {
      clearPendingPreview()
      return
    }
    setCreatedPath('')
    setTitle(mode === 'edit' ? book?.name || '' : '')
    setAuthor(mode === 'edit' ? book?.author || '' : '')
    setDescription('')
    setFormError('')
    setCoverPresetId(defaultImagePresetId || 'game-cg')
    setCoverInstruction('')
    setCoverError('')
    setPendingCoverFile(null)
    clearPendingPreview()
    setLoading(mode === 'edit')
    if (mode !== 'edit' || !book) return

    let cancelled = false
    getBookInfo(book.path)
      .then((meta: BookMeta) => {
        if (cancelled) return
        setTitle(meta.title)
        setAuthor(meta.author)
        setDescription(meta.description)
      })
      .catch(() => {
        // Keep the list metadata as fallback.
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [book, defaultImagePresetId, mode, open])

  useEffect(() => {
    if (open) setCoverPresetId((current) => current || defaultImagePresetId || 'game-cg')
  }, [defaultImagePresetId, open])

  const dialogTitle = mode === 'create' && !createdPath ? t('home.createBook') : t('home.editInfo')
  const dialogDescription = activePath
    ? activePath
    : `${t('home.createIn')} ${novaDir || t('home.novaDirLoading')}`

  const closeDialog = (nextOpen: boolean) => {
    if (!nextOpen && busy) return
    onOpenChange(nextOpen)
  }

  const handleSubmit = async () => {
    const validTitle = title.trim()
    if (!validTitle) { setFormError(t('home.titleRequired')); return }
    if (mode === 'create' && !createdPath && !novaDir.trim()) { setFormError(t('home.waitNovaDir')); return }
    setSaving(true)
    setFormError('')
    try {
      let path = activePath
      if (!path) {
        path = await createWorkspace()
      } else {
        await updateBookInfo(path, validTitle, author.trim(), description.trim())
      }
      if (pendingCoverFile) {
        const result = await uploadBookCover(path, pendingCoverFile)
        onCoverUpdated(path, result.cover_updated_at || String(Date.now()))
        clearPendingCover()
      }
      await Promise.resolve(onBooksChange())
      onOpenChange(false)
    } catch (error) {
      setFormError(error instanceof Error ? error.message : t(mode === 'create' && !createdPath ? 'home.createError' : 'home.saveError'))
    } finally {
      setSaving(false)
    }
  }

  const handleFileSelected = async (file: File | null) => {
    if (!file) return
    setCoverError('')
    if (!activePath) {
      setPendingCoverFile(file)
      const previewURL = createPreviewURL(file)
      setPendingCoverPreview((current) => {
        revokePreviewURL(current)
        return previewURL
      })
      return
    }
    setCoverBusy('upload')
    try {
      const result = await uploadBookCover(activePath, file)
      onCoverUpdated(activePath, result.cover_updated_at || String(Date.now()))
      clearPendingCover()
      await Promise.resolve(onBooksChange())
    } catch (error) {
      setCoverError(error instanceof Error ? error.message : t('home.coverUploadError'))
    } finally {
      setCoverBusy('')
    }
  }

  const handleGenerateCover = async () => {
    if (!title.trim()) { setFormError(t('home.titleRequired')); return }
    setCoverBusy('generate')
    setCoverError('')
    setFormError('')
    try {
      const path = activePath || await createWorkspace()
      clearPendingCover()
      const result = await generateBookCover({
        path,
        imagePresetId: coverPresetId || defaultImagePresetId || 'game-cg',
        instruction: coverInstruction.trim(),
      })
      onCoverUpdated(path, result.cover_updated_at || String(Date.now()))
      await Promise.resolve(onBooksChange())
    } catch (error) {
      setCoverError(error instanceof Error ? error.message : t('home.coverGenerateError'))
    } finally {
      setCoverBusy('')
    }
  }

  const createWorkspace = async () => {
    if (!title.trim()) throw new Error(t('home.titleRequired'))
    if (!novaDir.trim()) throw new Error(t('home.waitNovaDir'))
    const data = await createBook(title.trim(), author.trim() || undefined, description.trim() || undefined)
    setCreatedPath(data.workspace)
    onSwitch(data.workspace)
    await Promise.resolve(onBooksChange())
    return data.workspace
  }

  const clearPendingCover = () => {
    setPendingCoverFile(null)
    clearPendingPreview()
  }

  const clearPendingPreview = () => {
    setPendingCoverPreview((current) => {
      revokePreviewURL(current)
      return ''
    })
  }

  return (
    <Dialog open={open} onOpenChange={closeDialog}>
      <DialogContent
        showCloseButton={false}
        className="nova-panel flex max-h-[min(760px,calc(100vh-2rem))] w-[min(720px,calc(100vw-2rem))] max-w-[min(720px,calc(100vw-2rem))] grid-rows-none flex-col gap-0 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
      >
        <DialogHeader className="border-b border-[var(--nova-border)] px-4 py-3">
          <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{dialogTitle}</DialogTitle>
          <DialogDescription className="truncate text-xs text-[var(--nova-text-faint)]">
            {dialogDescription}
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
          {loading ? (
            <div className="py-10 text-center text-xs text-[var(--nova-text-faint)]">{t('common.loading')}</div>
          ) : (
            <div className="grid min-w-0 gap-4 md:grid-cols-[minmax(0,1fr)_180px]">
              <div className="min-w-0 space-y-3">
                <Input
                  type="text"
                  value={title}
                  onChange={(event) => setTitle(event.target.value)}
                  placeholder={t('home.bookTitlePlaceholder')}
                  className={inputCls}
                  autoFocus
                />
                <Input
                  type="text"
                  value={author}
                  onChange={(event) => setAuthor(event.target.value)}
                  placeholder={t('home.authorPlaceholder')}
                  className={inputCls}
                />
                {mode === 'create' && !createdPath && (
                  <div className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-xs text-[var(--nova-text-faint)]">
                    <Folder className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                    <span className="shrink-0">{t('home.createIn')}</span>
                    <span className="truncate text-[var(--nova-text-muted)]">{novaDir || t('home.novaDirLoading')}</span>
                  </div>
                )}
                <Textarea
                  autoResize
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  placeholder={t('home.descriptionPlaceholder')}
                  rows={7}
                  className={inputCls + ' min-h-36 resize-none'}
                />
                {formError && <div className="text-xs text-[var(--nova-danger)]">{formError}</div>}
              </div>
              <div className="min-w-0 space-y-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
                <BookCoverThumbnail
                  book={previewBook}
                  previewURL={pendingCoverPreview}
                  version={previewBook ? coverVersion(previewBook) : ''}
                  title={title.trim() || t('home.unnamedBook')}
                  className="mx-auto aspect-[3/4] w-28 md:w-full"
                  iconClassName="h-5 w-5"
                />
                <div className="flex items-center gap-1.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
                  <ImageIcon className="h-3.5 w-3.5" />
                  {t('home.cover')}
                </div>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="image/png,image/jpeg"
                  className="hidden"
                  aria-label={t('home.coverFile')}
                  onChange={(event) => {
                    void handleFileSelected(event.target.files?.[0] || null)
                    event.target.value = ''
                  }}
                />
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  className={ghostButtonCls + ' w-full max-w-full justify-center'}
                  disabled={busy}
                  onClick={() => fileInputRef.current?.click()}
                >
                  {coverBusy === 'upload' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Upload className="h-3.5 w-3.5" />}
                  {coverBusy === 'upload' ? t('home.coverUploading') : t('home.uploadCover')}
                </Button>
                {pendingCoverFile && !activePath && (
                  <div className="line-clamp-2 text-[11px] text-[var(--nova-text-faint)]">{t('home.coverUploadPending')}</div>
                )}
                <select
                  aria-label={t('home.coverPreset')}
                  value={coverPresetId || defaultImagePresetId || 'game-cg'}
                  onChange={(event) => setCoverPresetId(event.target.value)}
                  className={inputCls + ' h-8 py-1 text-xs'}
                  disabled={busy}
                >
                  {imagePresetOptions.map((preset) => (
                    <option key={preset.id} value={preset.id}>{preset.name || preset.id}</option>
                  ))}
                </select>
                <Textarea
                  autoResize
                  value={coverInstruction}
                  onChange={(event) => setCoverInstruction(event.target.value)}
                  placeholder={t('home.coverInstructionPlaceholder')}
                  rows={3}
                  className={inputCls + ' min-h-20 resize-none'}
                  disabled={busy}
                />
                {coverError && <div className="line-clamp-3 text-[11px] text-[var(--nova-danger)]">{coverError}</div>}
                <Button
                  type="button"
                  size="xs"
                  className={primaryButtonCls + ' w-full max-w-full justify-center'}
                  disabled={busy || loading || (mode === 'create' && !novaDir.trim())}
                  onClick={() => void handleGenerateCover()}
                >
                  {coverBusy === 'generate' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Sparkles className="h-3.5 w-3.5" />}
                  {coverBusy === 'generate'
                    ? t('home.coverGenerating')
                    : activePath
                      ? t('home.generateCover')
                      : t('home.createAndGenerateCover')}
                </Button>
              </div>
            </div>
          )}
        </div>
        <DialogFooter className="!mx-0 !mb-0 rounded-none border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)]/95 !px-4 !py-3">
          <Button
            type="button"
            size="xs"
            variant="ghost"
            className={ghostButtonCls}
            disabled={busy}
            onClick={() => closeDialog(false)}
          >
            {t('common.cancel')}
          </Button>
          <Button
            type="button"
            size="xs"
            className={primaryButtonCls}
            disabled={busy || loading || (mode === 'create' && !createdPath && !novaDir.trim())}
            onClick={() => void handleSubmit()}
          >
            {saving ? footerBusyLabel : footerPrimaryLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function createPreviewURL(file: File) {
  if (typeof URL === 'undefined' || typeof URL.createObjectURL !== 'function') return ''
  return URL.createObjectURL(file)
}

function revokePreviewURL(url: string) {
  if (!url || typeof URL === 'undefined' || typeof URL.revokeObjectURL !== 'function') return
  URL.revokeObjectURL(url)
}
