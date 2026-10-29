import { useEffect, useState } from 'react'
import { FilePlus2, FolderPlus, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { fetchProjectSettings } from '@/features/settings/api'
import type { ChapterStructure, ChapterSummary } from '@/lib/api'
import type { ChapterOutlineVolume } from './ChapterOutlineList'
import { allocateChapterPath, allocateVolumePath, InvalidManuscriptFormatError } from './manuscript-paths'
import '@/components/file-tree/FileTreeMenu.css'

export interface ManuscriptCreateRequest {
  kind: 'chapter' | 'volume' | 'section'
  parentPath?: string
}

export function ManuscriptCreateMenu({ structure, onSelect }: { structure: ChapterStructure; onSelect: (request: ManuscriptCreateRequest) => void }) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon-xs" aria-label={t('planning.create.openMenu')}><Plus /></Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="nova-file-tree-menu-surface nova-file-tree-menu-radix">
        <DropdownMenuGroup>
          {structure !== 'chapters' && <DropdownMenuItem className="nova-file-tree-menu-item" onSelect={() => onSelect({ kind: 'volume' })}><FolderPlus />{t('planning.create.volume')}</DropdownMenuItem>}
          <DropdownMenuItem className="nova-file-tree-menu-item" onSelect={() => onSelect({ kind: 'chapter' })}><FilePlus2 />{t('planning.create.chapter')}</DropdownMenuItem>
          {structure === 'sections' && <DropdownMenuItem className="nova-file-tree-menu-item" onSelect={() => onSelect({ kind: 'section' })}><FilePlus2 />{t('bookStructure.section')}</DropdownMenuItem>}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

interface ManuscriptCreateDialogProps {
  projectId: string
  structure: ChapterStructure
  request: ManuscriptCreateRequest | null
  chapters: ChapterSummary[]
  containers: ChapterOutlineVolume[]
  defaultParentPath: string
  onOpenChange: (open: boolean) => void
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onCreated: (path: string, type: 'file' | 'dir') => void | Promise<void>
}

export function ManuscriptCreateDialog({ projectId, structure, request, chapters, containers, defaultParentPath, onOpenChange, onCreateItem, onCreated }: ManuscriptCreateDialogProps) {
  const { t } = useTranslation()
  const [title, setTitle] = useState('')
  const [parentPath, setParentPath] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const kind = request?.kind || 'chapter'
  const root = kind === 'volume' || (kind === 'chapter' && structure === 'chapters')
  const parentDepth = kind === 'section' ? 2 : 1
  const parents = containers.filter((item) => item.key.split('/').length - 1 === parentDepth)
  const dialogTitle = t(kind === 'section' ? 'bookStructure.section' : kind === 'volume' ? 'planning.create.volume' : 'planning.create.chapter')
  useEffect(() => {
    if (!request) return
    setTitle(''); setError('')
    setParentPath(root ? 'chapters' : request.parentPath || parents.find((item) => defaultParentPath === item.key || defaultParentPath.startsWith(`${item.key}/`))?.key || parents[0]?.key || '')
    // The request opens a fresh dialog; updates from background snapshots must
    // not reset author input while it is open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [request])

  const submit = async () => {
    if (!title.trim() || !parentPath || submitting) return
    setSubmitting(true); setError('')
    try {
      const settings = await fetchProjectSettings(projectId)
      let path: string
      const type = kind === 'volume' || (kind === 'chapter' && structure === 'sections') ? 'dir' : 'file'
      if (kind === 'volume') {
        path = allocateVolumePath({ volumePaths: containers.filter((item) => item.key.split('/').length === 2).map((item) => item.key), title, format: settings.effective.volume_dir_format })
      } else {
        const existing = kind === 'section' ? chapters.filter((chapter) => chapter.path.slice(0, chapter.path.lastIndexOf('/')) === parentPath)
          : structure === 'sections' ? containers.filter((item) => item.key.split('/').length === 3).flatMap((item) => ['md', 'txt'].map((extension) => ({ path: `${item.key}.${extension}`, index: item.index || 0 }))) : chapters
        path = allocateChapterPath({ chapters: existing, title, volumePath: parentPath,
          format: kind === 'section' ? 's{order:05}-{chapter}-{title}.md' : settings.effective.chapter_filename_format,
          chapterLabel: (order) => t(kind === 'section' ? 'bookStructure.sectionOrdinal' : 'planning.create.chapterOrdinal', { order }),
        })
        if (type === 'dir') path = path.replace(/\.(md|txt)$/i, '')
      }
      await onCreateItem(path, type)
      onOpenChange(false)
      await onCreated(path, type)
    } catch (cause) {
      console.error('Creating manuscript item failed', { projectId, kind, cause })
      setError(cause instanceof InvalidManuscriptFormatError ? t('planning.create.invalidFormat') : cause instanceof Error ? cause.message : String(cause))
    } finally { setSubmitting(false) }
  }

  return <Dialog open={request !== null} onOpenChange={(open) => { if (!submitting) onOpenChange(open) }}>
    <DialogContent>
      <DialogHeader><DialogTitle>{dialogTitle}</DialogTitle><DialogDescription className="sr-only">{t('bookStructure.manuscript')}</DialogDescription></DialogHeader>
      <FieldGroup>
        <Field><FieldLabel htmlFor="manuscript-title">{t('bookStructure.itemTitle')}</FieldLabel>
          <Input id="manuscript-title" value={title} autoFocus disabled={submitting} onChange={(event) => setTitle(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void submit() } }} />
        </Field>
        {!root && <Field><FieldLabel>{t('bookStructure.parent')}</FieldLabel>
          <Select value={parentPath} onValueChange={setParentPath} disabled={submitting}>
            <SelectTrigger className="w-full" aria-label={t('bookStructure.parent')}><SelectValue /></SelectTrigger>
            <SelectContent><SelectGroup>{parents.map((parent) => <SelectItem key={parent.key} value={parent.key}>{parent.breadcrumb || parent.label}</SelectItem>)}</SelectGroup></SelectContent>
          </Select>
          {parents.length === 0 && <p className="text-sm text-muted-foreground">{t('bookStructure.noParent')}</p>}
        </Field>}
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      </FieldGroup>
      <DialogFooter><Button variant="outline" disabled={submitting} onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
        <Button disabled={!title.trim() || !parentPath || submitting} onClick={() => void submit()}>{t(submitting ? 'common.saving' : 'common.create')}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
