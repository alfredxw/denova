import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { StorySummary } from '../types'
import { CompactResourcePicker } from './CompactResourcePicker'

interface StoryPickerProps {
  stories: StorySummary[]
  currentStoryId: string
  onSelect: (storyId: string) => void
  onCreate: () => void
  onDelete: (storyId: string) => void
  layout?: 'inline' | 'sidebar'
  hideCreate?: boolean
}

export function StoryPicker({ stories, currentStoryId, onSelect, onCreate, onDelete, layout = 'inline', hideCreate = false }: StoryPickerProps) {
  const { t } = useTranslation()
  const createButton = hideCreate ? null : <Button type="button" variant="ghost" size="xs" className="nova-nav-item" onClick={onCreate}><Plus data-icon="inline-start" />{t('chat.new')}</Button>

  return (
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
      trailingAction={createButton}
      renderFooter={currentStoryId ? (close) => (
        <div className="mt-1 border-t border-[var(--nova-border)] pt-1">
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className="w-full justify-start gap-1.5 px-2 text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
            onClick={() => {
              close()
              onDelete(currentStoryId)
            }}
          >
            <Trash2 data-icon="inline-start" />
            {t('storyPicker.delete')}
          </Button>
        </div>
      ) : undefined}
      onSelect={onSelect}
    />
  )
}
