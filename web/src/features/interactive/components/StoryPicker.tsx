import { Check, ChevronDown, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import type { StorySummary } from '../types'

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
  const [open, setOpen] = useState(false)
  const sidebar = layout === 'sidebar'
  const selectedStory = stories.find((story) => story.id === currentStoryId)
  const selector = (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" className={`nova-field ${sidebar ? 'w-full' : 'w-[190px]'} justify-between px-3 py-0.5 text-xs font-normal`} aria-label={t('storyPicker.placeholder')}>
          <span className="min-w-0 flex-1 truncate text-left">{selectedStory?.title || t('storyPicker.placeholder')}</span>
          <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform ${open ? 'rotate-180' : ''}`} />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" sideOffset={6} className={`${sidebar ? 'w-[min(calc(100vw-2rem),24rem)]' : 'w-[190px]'} max-h-[min(70dvh,28rem)] overflow-y-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1 text-[var(--nova-text)] shadow-[var(--nova-shadow)]`}>
        <div role="listbox" aria-label={t('storyPicker.placeholder')} className="space-y-1">
          {stories.length === 0 ? <div className="px-2 py-2 text-xs text-[var(--nova-text-faint)]">{t('storyPicker.empty')}</div> : stories.map((story) => (
            <button key={story.id} type="button" role="option" aria-selected={story.id === currentStoryId} className={`flex w-full min-w-0 items-center gap-2 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-xs leading-5 ${story.id === currentStoryId ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`} onClick={() => { setOpen(false); onSelect(story.id) }}>
              <span className="min-w-0 flex-1 truncate">{story.title}</span>
              {story.id === currentStoryId ? <Check className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : null}
            </button>
          ))}
        </div>
        {currentStoryId ? <div className="mt-1 border-t border-[var(--nova-border)] pt-1"><Button type="button" variant="ghost" size="xs" className="w-full justify-start gap-1.5 px-2 text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]" onClick={() => { setOpen(false); onDelete(currentStoryId) }}><Trash2 className="h-3 w-3" />{t('storyPicker.delete')}</Button></div> : null}
      </PopoverContent>
    </Popover>
  )
  const createButton = hideCreate ? null : <Button type="button" variant="ghost" size="xs" className="nova-nav-item" onClick={onCreate}><Plus className="h-3 w-3" />{t('chat.new')}</Button>

  if (sidebar) return <div className="flex min-w-0 flex-col gap-1.5"><div className="flex items-center justify-between gap-2"><span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.label')}</span>{createButton}</div>{selector}</div>
  return <div className="flex min-w-0 items-center gap-1.5"><span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.label')}</span>{selector}{createButton}</div>
}
