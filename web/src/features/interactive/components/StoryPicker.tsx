import { useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type { StorySummary, Teller } from '../types'

interface StoryPickerProps {
  stories: StorySummary[]
  currentStoryId: string
  tellers: Teller[]
  onSelect: (storyId: string) => void
  onCreate: (input: { title: string; origin: string; story_teller_id: string; reply_target_chars: number }) => void
  onDelete: (storyId: string) => void
  layout?: 'inline' | 'sidebar'
}

export function StoryPicker({ stories, currentStoryId, tellers, onSelect, onCreate, onDelete, layout = 'inline' }: StoryPickerProps) {
  const { t } = useTranslation()
  const [creating, setCreating] = useState(false)
  const [title, setTitle] = useState('')
  const [origin, setOrigin] = useState('')
  const [replyTargetChars, setReplyTargetChars] = useState('1200')
  const defaultTeller = tellers[0]?.id || 'classic'
  const sidebar = layout === 'sidebar'
  const suggestedTitle = defaultStoryTitle(stories, t)

  const closeCreate = () => {
    setTitle('')
    setOrigin('')
    setReplyTargetChars('1200')
    setCreating(false)
  }

  const submit = () => {
    onCreate({
      title: title.trim() || suggestedTitle,
      origin: origin.trim(),
      story_teller_id: defaultTeller,
      reply_target_chars: normalizeReplyTargetChars(replyTargetChars),
    })
    closeCreate()
  }

  const selector = (
    <Select
      value={currentStoryId || '__none'}
      onValueChange={(value) => {
        if (value !== '__none' && value !== '__empty') onSelect(value)
      }}
    >
      <SelectTrigger size="sm" className={`nova-field ${sidebar ? 'w-full' : 'w-[190px]'} px-3 py-0.5 text-xs focus:ring-0`} aria-label={t('storyPicker.placeholder')}>
        <SelectValue placeholder={t('storyPicker.placeholder')} />
      </SelectTrigger>
      <SelectContent className="nova-panel border text-[var(--nova-text)]">
        <SelectItem value="__none" disabled>
          {t('storyPicker.placeholder')}
        </SelectItem>
        {stories.length === 0 ? (
          <SelectItem value="__empty" disabled>
            {t('storyPicker.empty')}
          </SelectItem>
        ) : (
          stories.map((story) => (
            <SelectItem key={story.id} value={story.id}>
              {story.title}
            </SelectItem>
          ))
        )}
      </SelectContent>
    </Select>
  )

  const createButton = (
    <Popover
      open={creating}
      onOpenChange={(open) => {
        if (!open) {
          closeCreate()
          return
        }
        setCreating(true)
        setTitle((current) => (current.trim() ? current : suggestedTitle))
      }}
    >
      <PopoverTrigger asChild>
        <Button variant="ghost" size="xs" className="nova-nav-item">
          <Plus className="h-3 w-3" />
          {t('chat.new')}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="nova-panel w-80 border p-3 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
        <div className="mb-2 text-xs font-medium">{t('storyPicker.create')}</div>
        <Input className="nova-field mb-2 text-xs" placeholder={suggestedTitle} value={title} onChange={(event) => setTitle(event.target.value)} />
        <Textarea autoResize className="nova-field mb-3 min-h-20 resize-none text-xs" placeholder={t('storyPicker.originPlaceholder')} value={origin} onChange={(event) => setOrigin(event.target.value)} />
        <div className="mb-3">
          <div className="mb-1.5 text-[11px] text-[var(--nova-text-faint)]">{t('storyPicker.replyTargetChars')}</div>
          <Input className="nova-field text-xs" type="number" min={1} value={replyTargetChars} onChange={(event) => setReplyTargetChars(event.target.value)} placeholder="1200" />
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" size="xs" onClick={closeCreate}>
            {t('common.cancel')}
          </Button>
          <Button size="xs" onClick={submit}>
            {t('common.create')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )

  const deleteButton = currentStoryId && (
    <Button variant="ghost" size="icon-xs" className="text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]" onClick={() => onDelete(currentStoryId)} aria-label={t('storyPicker.delete')}>
      <Trash2 className="h-3 w-3" />
    </Button>
  )

  if (sidebar) {
    return (
      <div className="flex min-w-0 flex-col gap-1.5">
        <div className="flex items-center justify-between gap-2">
          <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.label')}</span>
          <div className="flex shrink-0 items-center gap-1">
            {createButton}
            {deleteButton}
          </div>
        </div>
        {selector}
      </div>
    )
  }

  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.label')}</span>
      {selector}
      {createButton}
      {deleteButton}
    </div>
  )
}

function normalizeReplyTargetChars(value: string) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : 1200
}

function defaultStoryTitle(stories: StorySummary[], t: (key: string, options?: Record<string, unknown>) => string): string {
  if (stories.length === 0) return t('storyPicker.firstTitle')

  let next = stories.length + 1
  for (const story of stories) {
    const match = story.title.trim().match(/^故事线\s*(\d+)$/)
    if (!match) continue
    next = Math.max(next, Number(match[1]) + 1)
  }
  return t('storyPicker.numberedTitle', { number: Math.max(2, next) })
}
