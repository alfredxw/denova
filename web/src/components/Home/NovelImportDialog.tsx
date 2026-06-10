import { FileText, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { importNovel, previewNovelImport, type NovelImportPreview, type NovelImportResult } from '@/lib/api'

interface NovelImportDialogProps {
  open: boolean
  novaDir: string
  onOpenChange: (open: boolean) => void
  onImported: (result: NovelImportResult) => void
}

const fieldCls = 'nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[#3a3a3a] focus:bg-[var(--nova-surface-3)]'
const ghostButtonCls = 'nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]'

export function NovelImportDialog({ open, novaDir, onOpenChange, onImported }: NovelImportDialogProps) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<NovelImportPreview | null>(null)
  const [bookTitle, setBookTitle] = useState('')
  const [author, setAuthor] = useState('')
  const [description, setDescription] = useState('')
  const [previewing, setPreviewing] = useState(false)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState('')

  const reset = () => {
    setFile(null)
    setPreview(null)
    setBookTitle('')
    setAuthor('')
    setDescription('')
    setPreviewing(false)
    setImporting(false)
    setError('')
    if (inputRef.current) inputRef.current.value = ''
  }

  const handleOpenChange = (next: boolean) => {
    if (!next && importing) return
    if (!next) reset()
    onOpenChange(next)
  }

  const handleFileSelected = async (selected: File | undefined) => {
    if (!selected) return
    setFile(selected)
    setPreview(null)
    setError('')
    setPreviewing(true)
    try {
      const data = await previewNovelImport(selected)
      setPreview(data)
      setBookTitle(data.title)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('novelImport.previewFailed'))
    } finally {
      setPreviewing(false)
    }
  }

  const handleImport = async () => {
    if (!file || !preview) {
      setError(t('novelImport.chooseFileFirst'))
      return
    }
    setImporting(true)
    setError('')
    try {
      const result = await importNovel(file, {
        bookTitle: bookTitle.trim() || preview.title,
        author: author.trim() || undefined,
        description: description.trim() || undefined,
      })
      onImported(result)
      reset()
      onOpenChange(false)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('novelImport.importFailed'))
    } finally {
      setImporting(false)
    }
  }

  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept=".txt,.md,.markdown,text/plain,text/markdown,text/x-markdown"
        className="hidden"
        onChange={(event) => void handleFileSelected(event.target.files?.[0])}
      />
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent
          className="nova-panel flex max-h-[min(760px,calc(100vh-2rem))] w-[min(620px,calc(100vw-2rem))] max-w-[min(620px,calc(100vw-2rem))] flex-col rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
          aria-describedby="novel-import-desc"
        >
          <div className="border-b border-[var(--nova-border)] px-4 py-3">
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('novelImport.title')}</DialogTitle>
            <DialogDescription id="novel-import-desc" className="mt-1 text-xs text-[var(--nova-text-faint)]">
              {t('novelImport.description')}
            </DialogDescription>
          </div>

          <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4 text-xs">
            <div className="flex min-w-0 items-center gap-2">
              <Button
                type="button"
                size="xs"
                variant="ghost"
                className={ghostButtonCls}
                onClick={() => inputRef.current?.click()}
                disabled={previewing || importing}
              >
                <Upload className="h-3.5 w-3.5" />
                {t('novelImport.chooseFile')}
              </Button>
              <div className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">{file ? file.name : t('novelImport.noFile')}</div>
              {previewing && <span className="shrink-0 text-[var(--nova-text-muted)]">{t('novelImport.parsing')}</span>}
            </div>

            {preview && (
              <div className="space-y-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3">
                <div className="flex min-w-0 items-center gap-2">
                  <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                  <div className="min-w-0 flex-1 truncate text-sm font-medium text-[var(--nova-text)]">{preview.title}</div>
                </div>
                <div className="flex flex-wrap items-center gap-2 text-[11px] text-[var(--nova-text-faint)]">
                  <span>{t('novelImport.chapterCount', { count: preview.chapter_count })}</span>
                  <span>{t('novelImport.totalChars', { count: preview.total_chars.toLocaleString() })}</span>
                </div>
                <div className="max-h-36 space-y-1 overflow-y-auto border-t border-[var(--nova-border)] pt-2">
                  {preview.chapters.map((chapter) => (
                    <div key={`${chapter.index}-${chapter.title}`} className="flex items-center gap-2 text-[11px] text-[var(--nova-text-muted)]">
                      <span className="w-8 shrink-0 text-[var(--nova-text-faint)]">{chapter.index}</span>
                      <span className="min-w-0 flex-1 truncate">{chapter.title}</span>
                      <span className="shrink-0 text-[var(--nova-text-faint)]">{t('novelImport.chapterChars', { count: chapter.chars.toLocaleString() })}</span>
                    </div>
                  ))}
                  {preview.chapter_count > preview.chapters.length && (
                    <div className="text-[11px] text-[var(--nova-text-faint)]">{t('novelImport.moreChapters', { count: preview.chapter_count - preview.chapters.length })}</div>
                  )}
                </div>
                {preview.warnings?.map((warning) => (
                  <div key={warning} className="text-[11px] text-amber-200">{warning}</div>
                ))}
              </div>
            )}

            {preview && (
              <div className="space-y-2">
                <Input
                  value={bookTitle}
                  onChange={(event) => setBookTitle(event.target.value)}
                  placeholder={t('novelImport.bookTitle')}
                  className={fieldCls}
                  disabled={importing}
                />
                <Input
                  value={author}
                  onChange={(event) => setAuthor(event.target.value)}
                  placeholder={t('novelImport.author')}
                  className={fieldCls}
                  disabled={importing}
                />
                <Textarea
                  autoResize
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  placeholder={t('novelImport.descriptionField')}
                  rows={1}
                  className={`${fieldCls} min-h-0 resize-none`}
                  disabled={importing}
                />
                <div className="truncate text-[11px] text-[var(--nova-text-faint)]">{t('novelImport.createIn', { dir: novaDir || t('importCard.novaDir') })}</div>
              </div>
            )}

            {error && (
              <div className="rounded-[var(--nova-radius)] border border-red-500/30 bg-red-500/10 px-3 py-2 text-red-200">
                {error}
              </div>
            )}
          </div>

          <div className="flex items-center justify-end gap-2 border-t border-[var(--nova-border)] px-4 py-3">
            <Button
              type="button"
              size="xs"
              variant="ghost"
              className="nova-nav-item border border-transparent text-[var(--nova-text-muted)]"
              onClick={() => handleOpenChange(false)}
              disabled={importing}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              size="xs"
              className="border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]"
              onClick={() => void handleImport()}
              disabled={!file || !preview || previewing || importing}
            >
              {importing ? t('novelImport.importing') : t('novelImport.import')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
