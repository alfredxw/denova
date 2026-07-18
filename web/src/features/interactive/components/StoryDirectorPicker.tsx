import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { StoryDirector, StorySummary } from '../types'
import { CompactResourcePicker } from './CompactResourcePicker'

interface StoryDirectorPickerProps {
  story?: StorySummary
  storyDirectors: StoryDirector[]
  onChange: (directorId: string) => void
  layout?: 'inline' | 'sidebar'
}

export function StoryDirectorPicker({ story, storyDirectors, onChange, layout = 'inline' }: StoryDirectorPickerProps) {
  const { t } = useTranslation()
  const normalizedDirectors = useMemo(
    () => storyDirectors.length ? storyDirectors : [{ id: 'default', name: t('storyPicker.defaultStoryDirector') } as StoryDirector],
    [storyDirectors, t],
  )
  const selectedDirector = useMemo(
    () => normalizedDirectors.find((director) => director.id === story?.story_director_id) || normalizedDirectors[0] || null,
    [normalizedDirectors, story?.story_director_id],
  )

  return (
    <CompactResourcePicker
      items={normalizedDirectors}
      selectedId={selectedDirector?.id}
      getId={(director) => director.id}
      getLabel={(director) => director.name || director.id}
      label={layout === 'sidebar' ? t('storyPicker.storyDirector') : t('storyPicker.directorLabel')}
      ariaLabel={t('storyPicker.chooseStoryDirector')}
      placeholder={t('storyPicker.chooseStoryDirector')}
      layout={layout}
      disabled={!story}
      triggerClassName={layout === 'inline' ? 'w-[180px]' : undefined}
      contentClassName={layout === 'sidebar' ? 'w-[min(calc(100vw-2rem),22rem)]' : 'w-[210px]'}
      onSelect={(directorId) => {
        if (directorId !== story?.story_director_id) onChange(directorId)
      }}
    />
  )
}
