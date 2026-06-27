import { useState } from 'react'
import { Check, ChevronDown, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Textarea } from '@/components/ui/textarea'
import { DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS, type StoryCreateInput } from '../opening'
import type { StorySummary, Teller } from '../types'

interface StoryPickerProps {
  stories: StorySummary[]
  currentStoryId: string
  tellers: Teller[]
  onSelect: (storyId: string) => void
  onCreate: (input: StoryCreateInput) => void
  onDelete: (storyId: string) => void
  layout?: 'inline' | 'sidebar'
}

export function StoryPicker({ stories, currentStoryId, tellers, onSelect, onCreate, onDelete, layout = 'inline' }: StoryPickerProps) {
  const { t } = useTranslation()
  const [creating, setCreating] = useState(false)
  const [selectorOpen, setSelectorOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [origin, setOrigin] = useState('')
  const [replyTargetChars, setReplyTargetChars] = useState(String(DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS))
  const defaultTeller = tellers[0]?.id || 'classic'
  const sidebar = layout === 'sidebar'
  const suggestedTitle = defaultStoryTitle(stories, t)
  const selectedStory = stories.find((story) => story.id === currentStoryId) || null

  const closeCreate = () => {
    setTitle('')
    setOrigin('')
    setReplyTargetChars(String(DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS))
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
    <Popover open={selectorOpen} onOpenChange={setSelectorOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={`nova-field ${sidebar ? 'w-full' : 'w-[190px]'} justify-between px-3 py-0.5 text-xs font-normal text-[var(--nova-text)] focus:ring-0`}
          aria-label={t('storyPicker.placeholder')}
          aria-expanded={selectorOpen}
        >
          <span className="min-w-0 flex-1 truncate text-left">{selectedStory?.title || t('storyPicker.placeholder')}</span>
          <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform ${selectorOpen ? 'rotate-180' : ''}`} />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={6}
        className={`${sidebar ? 'w-[min(calc(100vw-2rem),24rem)]' : 'w-[190px]'} max-h-[min(70dvh,28rem)] overflow-y-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1 text-[var(--nova-text)] shadow-[var(--nova-shadow)]`}
      >
        <div role="listbox" aria-label={t('storyPicker.placeholder')} className="space-y-1">
          {stories.length === 0 ? (
            <div className="px-2 py-2 text-xs text-[var(--nova-text-faint)]">{t('storyPicker.empty')}</div>
          ) : (
            stories.map((story) => {
              const selected = story.id === currentStoryId
              return (
                <button
                  key={story.id}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  className={`flex w-full min-w-0 items-center gap-2 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-xs leading-5 ${selected ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}
                  onClick={() => {
                    setSelectorOpen(false)
                    if (story.id !== currentStoryId) onSelect(story.id)
                  }}
                >
                  <span className="min-w-0 flex-1 truncate">{story.title}</span>
                  {selected ? <Check className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : null}
                </button>
              )
            })
          )}
        </div>
        {currentStoryId ? (
          <div className="mt-1 border-t border-[var(--nova-border)] pt-1">
            <Button
              type="button"
              variant="ghost"
              size="xs"
              className="w-full justify-start gap-1.5 px-2 text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
              onClick={() => {
                setSelectorOpen(false)
                onDelete(currentStoryId)
              }}
              aria-label={t('storyPicker.delete')}
            >
              <Trash2 className="h-3 w-3" />
              {t('storyPicker.delete')}
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
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
          <Input className="nova-field text-xs" type="number" min={1} value={replyTargetChars} onChange={(event) => setReplyTargetChars(event.target.value)} placeholder={String(DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS)} />
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

  if (sidebar) {
    return (
      <div className="flex min-w-0 flex-col gap-1.5">
        <div className="flex items-center justify-between gap-2">
          <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.label')}</span>
          <div className="flex shrink-0 items-center gap-1">{createButton}</div>
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
    </div>
  )
}

function normalizeReplyTargetChars(value: string) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS
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
