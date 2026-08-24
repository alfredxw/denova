import { useEffect, useState } from 'react'
import { FilePlus2, FolderPlus, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { fetchProjectSettings } from '@/features/settings/api'
import type { ChapterSummary } from '@/lib/api'
import type { ChapterOutlineVolume } from './ChapterOutlineList'
import {
  allocateChapterPath,
  allocateVolumePath,
  InvalidManuscriptFormatError,
} from './manuscript-paths'

export interface ManuscriptCreateRequest {
  kind: 'chapter' | 'volume'
  volumePath?: string
}

export function ManuscriptCreateMenu({ onSelect }: { onSelect: (request: ManuscriptCreateRequest) => void }) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon-xs" className="shrink-0" aria-label={t('planning.create.openMenu')}>
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => onSelect({ kind: 'chapter' })}>
          <FilePlus2 />{t('planning.create.chapter')}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onSelect({ kind: 'volume' })}>
          <FolderPlus />{t('planning.create.volume')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

interface ManuscriptCreateDialogProps {
  projectId: string
  request: ManuscriptCreateRequest | null
  chapters: ChapterSummary[]
  volumes: ChapterOutlineVolume[]
  defaultVolumePath: string
  onOpenChange: (open: boolean) => void
  onCreateFile: (path: string) => Promise<void>
  onCreated: (path: string) => void | Promise<void>
}

export function ManuscriptCreateDialog({
  projectId,
  request,
  chapters,
  volumes,
  defaultVolumePath,
  onOpenChange,
  onCreateFile,
  onCreated,
}: ManuscriptCreateDialogProps) {
  const { t } = useTranslation()
  const [title, setTitle] = useState('')
  const [firstChapterTitle, setFirstChapterTitle] = useState('')
  const [volumePath, setVolumePath] = useState(defaultVolumePath)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const kind = request?.kind || 'chapter'
  const volumeOptions = volumes.some((volume) => volume.key === 'chapters')
    ? volumes
    : [{ key: 'chapters', label: t('planning.unvolumed'), chapters: [] }, ...volumes]

  useEffect(() => {
    if (!request) return
    setTitle('')
    setFirstChapterTitle('')
    setVolumePath(request.volumePath || defaultVolumePath)
    setError('')
  }, [defaultVolumePath, request])

  const handleSubmit = async () => {
    const trimmedTitle = title.trim()
    const trimmedFirstChapterTitle = firstChapterTitle.trim()
    if (!trimmedTitle || (kind === 'volume' && !trimmedFirstChapterTitle) || submitting) return
    if (kind === 'volume' && volumes.some((volume) => volume.key !== 'chapters' && volume.label.trim().toLocaleLowerCase() === trimmedTitle.toLocaleLowerCase())) {
      setError(t('planning.create.volumeExists'))
      return
    }
    setSubmitting(true)
    setError('')
    try {
      const settings = await fetchProjectSettings(projectId)
      const targetVolumePath = kind === 'volume'
        ? allocateVolumePath({
            volumePaths: volumes.map((volume) => volume.key),
            title: trimmedTitle,
            format: settings.effective.volume_dir_format,
          })
        : volumePath
      const path = allocateChapterPath({
        chapters,
        title: kind === 'volume' ? trimmedFirstChapterTitle : trimmedTitle,
        volumePath: targetVolumePath,
        format: settings.effective.chapter_filename_format,
        chapterLabel: (order) => t('planning.create.chapterOrdinal', { order }),
      })
      await onCreateFile(path)
      onOpenChange(false)
      await onCreated(path)
    } catch (cause) {
      console.error('[components/workbench/outline/ManuscriptCreateDialog.tsx] creating manuscript item failed', {
        projectId,
        kind,
        cause,
      })
      setError(cause instanceof InvalidManuscriptFormatError
        ? t('planning.create.invalidFormat')
        : cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={request !== null} onOpenChange={(open) => { if (!submitting) onOpenChange(open) }}>
      <DialogContent className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
        <DialogHeader>
          <DialogTitle>{t(kind === 'chapter' ? 'planning.create.chapter' : 'planning.create.volume')}</DialogTitle>
          <DialogDescription className="text-[var(--nova-text-muted)]">
            {t(kind === 'chapter' ? 'planning.create.chapterDescription' : 'planning.create.volumeDescription')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <label className="block space-y-2 text-xs text-[var(--nova-text-muted)]" htmlFor="manuscript-title">
            <span>{t(kind === 'chapter' ? 'planning.create.chapterTitle' : 'planning.create.volumeTitle')}</span>
            <Input
              id="manuscript-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && kind === 'chapter') {
                  event.preventDefault()
                  void handleSubmit()
                }
              }}
              className="border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]"
              autoFocus
            />
          </label>
          {kind === 'chapter' ? (
            <label className="block space-y-2 text-xs text-[var(--nova-text-muted)]">
              <span>{t('planning.create.targetVolume')}</span>
              <Select value={volumePath} onValueChange={setVolumePath}>
                <SelectTrigger className="w-full border-[var(--nova-border)] bg-[var(--nova-surface-2)]" aria-label={t('planning.create.targetVolume')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {volumeOptions.map((volume) => <SelectItem key={volume.key} value={volume.key}>{volume.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </label>
          ) : (
            <label className="block space-y-2 text-xs text-[var(--nova-text-muted)]" htmlFor="manuscript-first-chapter-title">
              <span>{t('planning.create.firstChapterTitle')}</span>
              <Input
                id="manuscript-first-chapter-title"
                value={firstChapterTitle}
                onChange={(event) => setFirstChapterTitle(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    void handleSubmit()
                  }
                }}
                className="border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]"
              />
            </label>
          )}
          {error ? <div role="alert" className="text-xs text-[var(--nova-danger)]">{error}</div> : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>{t('common.cancel')}</Button>
          <Button
            onClick={() => void handleSubmit()}
            disabled={!title.trim() || (kind === 'volume' && !firstChapterTitle.trim()) || submitting}
          >
            {submitting ? t('common.saving') : t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
