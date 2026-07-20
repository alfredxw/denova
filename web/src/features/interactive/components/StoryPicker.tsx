import { Check, ListChecks, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import type { StorySummary } from '../types'
import { CompactResourcePicker } from './CompactResourcePicker'

interface StoryPickerProps {
  stories: StorySummary[]
  currentStoryId: string
  onSelect: (storyId: string) => void
  onCreate: () => void
  onDeleteStories: (storyIds: string[]) => void | Promise<void>
  layout?: 'inline' | 'sidebar'
  hideCreate?: boolean
}

export function StoryPicker({ stories, currentStoryId, onSelect, onCreate, onDeleteStories, layout = 'inline', hideCreate = false }: StoryPickerProps) {
  const { t } = useTranslation()
  const [selectingForDelete, setSelectingForDelete] = useState(false)
  const [deleteSelection, setDeleteSelection] = useState<Set<string>>(() => new Set())
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const selectedStories = stories.filter((story) => deleteSelection.has(story.id))
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
        } : undefined}
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
          <div className="sticky bottom-0 mt-1 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] pt-1">
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
    </>
  )
}
