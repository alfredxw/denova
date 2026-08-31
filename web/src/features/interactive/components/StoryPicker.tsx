import { Check, ListChecks, PencilLine, Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { formatDateTime } from '@/i18n'
import type { StorySummary } from '../types'
import { CompactResourcePicker } from './CompactResourcePicker'

interface StoryPickerProps {
  stories: StorySummary[]
  currentStoryId: string
  onSelect: (storyId: string) => void
  onCreate: () => void
  onDeleteStories: (storyIds: string[]) => void | Promise<void>
  onRenameStory?: (storyId: string, title: string) => void | Promise<void>
  layout?: 'inline' | 'sidebar'
  hideCreate?: boolean
}

export function StoryPicker({ stories, currentStoryId, onSelect, onCreate, onDeleteStories, onRenameStory, layout = 'inline', hideCreate = false }: StoryPickerProps) {
  const { t } = useTranslation()
  const [selectingForDelete, setSelectingForDelete] = useState(false)
  const [deleteSelection, setDeleteSelection] = useState<Set<string>>(() => new Set())
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [renameStory, setRenameStory] = useState<StorySummary | null>(null)
  const [renameTitle, setRenameTitle] = useState('')
  const [renameError, setRenameError] = useState('')
  const [renaming, setRenaming] = useState(false)
  const selectedStories = stories.filter((story) => deleteSelection.has(story.id))
  const currentStory = stories.find((story) => story.id === currentStoryId)
  const allStoriesSelected = stories.length > 0 && selectedStories.length === stories.length
  const createButton = hideCreate ? null : <Button type="button" variant="ghost" size="xs" className="nova-nav-item" onClick={onCreate}><Plus data-icon="inline-start" />{t('chat.new')}</Button>

  const beginDeleteSelection = () => {
    const initialStoryId = stories.some((story) => story.id === currentStoryId) ? currentStoryId : stories[0]?.id
    setDeleteSelection(new Set(initialStoryId ? [initialStoryId] : []))
    setSelectingForDelete(true)
  }

  const cancelDeleteSelection = () => {
    setDeleteSelection(new Set())
    setSelectingForDelete(false)
  }

  const toggleDeleteSelection = (storyId: string) => {
    setDeleteSelection((current) => {
      const next = new Set(current)
      if (next.has(storyId)) next.delete(storyId)
      else next.add(storyId)
      return next
    })
  }

  const confirmDeleteStories = async () => {
    const storyIds = selectedStories.map((story) => story.id)
    if (storyIds.length === 0) return false
    await onDeleteStories(storyIds)
    cancelDeleteSelection()
  }

  const beginRename = (close: () => void) => {
    if (!currentStory || !onRenameStory) return
    close()
    setRenameTitle(currentStory.title)
    setRenameError('')
    setRenameStory(currentStory)
  }

  const submitRename = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!renameStory || !onRenameStory || renaming) return
    const title = renameTitle.trim()
    if (!title) {
      setRenameError(t('storyPicker.renameRequired'))
      return
    }
    if (title === renameStory.title.trim()) {
      setRenameStory(null)
      return
    }
    setRenameError('')
    setRenaming(true)
    try {
      await onRenameStory(renameStory.id, title)
      setRenameStory(null)
    } catch (error) {
      console.error('[story-picker] Failed to rename story', { storyId: renameStory.id, error })
      setRenameError(t('storyPicker.renameFailed'))
    } finally {
      setRenaming(false)
    }
  }

  return (
    <>
      <CompactResourcePicker
        items={stories}
        selectedId={currentStoryId}
        getId={(story) => story.id}
        getLabel={(story) => story.title}
        label={t('storyPicker.label')}
        ariaLabel={t('storyPicker.placeholder')}
        placeholder={t('storyPicker.placeholder')}
        emptyLabel={t('storyPicker.empty')}
        layout={layout}
        contentClassName="w-[min(calc(100vw-2rem),22rem)]"
        trailingAction={createButton}
        renderItem={selectingForDelete ? (_story, { id, label }) => {
          const checked = deleteSelection.has(id)
          return (
            <button
              type="button"
              aria-pressed={checked}
              className={`flex w-full min-w-0 items-center gap-2 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-xs leading-5 transition-colors ${checked ? 'bg-[var(--nova-danger-bg)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}
              onClick={() => toggleDeleteSelection(id)}
            >
              <span
                aria-hidden="true"
                className={`flex size-3.5 shrink-0 items-center justify-center rounded-[4px] border ${checked ? 'border-[var(--nova-danger)] bg-[var(--nova-danger)] text-white' : 'border-[var(--nova-border-strong)] bg-[var(--nova-surface)]'}`}
              >
                {checked ? <Check className="size-2.5" strokeWidth={3} /> : null}
              </span>
              <span className="min-w-0 flex-1 truncate">{label}</span>
            </button>
          )
        } : (story, { label, selected, select }) => {
          const lastTurnTime = story.updated_at ? formatDateTime(story.updated_at) : ''
          return (
            <button
              type="button"
              aria-label={label}
              aria-current={selected ? 'true' : undefined}
              className={`flex w-full min-w-0 flex-col gap-0.5 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-xs leading-5 transition-colors ${selected ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}
              onClick={select}
            >
              <span className="flex min-w-0 items-center gap-2">
                <span className="min-w-0 flex-1 truncate">{label}</span>
                {selected ? <Check className="size-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : null}
              </span>
              <span className="flex min-w-0 items-center gap-2 text-[11px] leading-4 text-[var(--nova-text-faint)]">
                <span className="shrink-0">{t('storyPicker.turnCount', { count: story.turn_count })}</span>
                {lastTurnTime ? (
                  <>
                    <span aria-hidden="true">·</span>
                    <span className="min-w-0 truncate">{t('storyPicker.lastTurn', { time: lastTurnTime })}</span>
                  </>
                ) : null}
              </span>
            </button>
          )
        }}
        renderFooter={stories.length > 0 ? (close) => selectingForDelete ? (
          <div className="sticky bottom-0 mt-1 space-y-1 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] pt-1">
            <div className="flex items-center justify-between gap-2 px-2 py-0.5 text-[11px] text-[var(--nova-text-faint)]">
              <span>{t('storyPicker.selectedCount', { count: selectedStories.length })}</span>
              <Button
                type="button"
                variant="ghost"
                size="xs"
                className="-mr-1"
                onClick={() => setDeleteSelection(allStoriesSelected ? new Set() : new Set(stories.map((story) => story.id)))}
              >
                {t(allStoriesSelected ? 'storyPicker.clearSelection' : 'storyPicker.selectAll')}
              </Button>
            </div>
            <div className="flex gap-1">
              <Button type="button" variant="ghost" size="xs" className="flex-1" onClick={cancelDeleteSelection}>
                {t('common.cancel')}
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="xs"
                className="flex-1"
                disabled={selectedStories.length === 0}
                onClick={() => {
                  close()
                  setDeleteDialogOpen(true)
                }}
              >
                <Trash2 data-icon="inline-start" />
                {t('storyPicker.deleteSelected', { count: selectedStories.length })}
              </Button>
            </div>
          </div>
        ) : (
          <div className="sticky bottom-0 mt-1 space-y-0.5 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] pt-1">
            {currentStory && onRenameStory ? (
              <Button
                type="button"
                variant="ghost"
                size="xs"
                className="w-full justify-start gap-1.5 px-2 text-[var(--nova-text-muted)]"
                onClick={() => beginRename(close)}
              >
                <PencilLine data-icon="inline-start" />
                {t('storyPicker.renameCurrent')}
              </Button>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              size="xs"
              className="w-full justify-start gap-1.5 px-2 text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
              onClick={beginDeleteSelection}
            >
              <ListChecks data-icon="inline-start" />
              {t('storyPicker.batchDelete')}
            </Button>
          </div>
        ) : undefined}
        onSelect={onSelect}
      />
      <ConfirmDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        title={t('storyPicker.confirmDeleteTitle', { count: selectedStories.length })}
        description={t('storyPicker.confirmDeleteDescription')}
        confirmLabel={t('common.delete')}
        tone="danger"
        details={selectedStories.map((story) => story.title)}
        onConfirm={confirmDeleteStories}
      />
      <Dialog
        open={Boolean(renameStory)}
        onOpenChange={(open) => {
          if (!open && !renaming) {
            setRenameStory(null)
            setRenameError('')
          }
        }}
      >
        <DialogContent showCloseButton={false} className="max-w-md">
          <form className="grid gap-4" onSubmit={(event) => void submitRename(event)}>
            <DialogHeader>
              <DialogTitle>{t('storyPicker.renameTitle')}</DialogTitle>
              <DialogDescription>{t('storyPicker.renameDescription')}</DialogDescription>
            </DialogHeader>
            <Field data-invalid={Boolean(renameError)}>
              <FieldLabel htmlFor="story-rename-title">{t('storyPicker.renameLabel')}</FieldLabel>
              <Input
                id="story-rename-title"
                value={renameTitle}
                maxLength={80}
                autoFocus
                aria-invalid={Boolean(renameError)}
                disabled={renaming}
                onChange={(event) => {
                  setRenameTitle(event.target.value)
                  if (renameError) setRenameError('')
                }}
              />
              <FieldError>{renameError}</FieldError>
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" disabled={renaming} onClick={() => setRenameStory(null)}>{t('common.cancel')}</Button>
              <Button type="submit" disabled={renaming}>
                {renaming ? <Spinner /> : <PencilLine data-icon="inline-start" />}
                {renaming ? t('storyPicker.renameSaving') : t('storyPicker.renameSave')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
