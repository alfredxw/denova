import { Loader2, Pencil } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS } from '../opening'
import type { StorySummary } from '../types'

export function ReplyTargetCharsControl({ story, onChange, layout = 'inline' }: { story?: StorySummary; onChange?: (replyTargetChars: number) => void | Promise<void>; layout?: 'inline' | 'console' }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(String(normalizeReplyTargetChars(story?.reply_target_chars)))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const currentValue = normalizeReplyTargetChars(story?.reply_target_chars)

  useEffect(() => {
    if (!open) {
      setDraft(String(currentValue))
      setError('')
    }
  }, [currentValue, open])

  const save = async () => {
    const nextValue = Number(draft)
    if (!Number.isFinite(nextValue) || nextValue <= 0) {
      setError(t('storyStage.replyTarget.invalid'))
      return
    }
    setSaving(true)
    setError('')
    try {
      await onChange?.(Math.floor(nextValue))
      setOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyStage.replyTarget.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={!story || !onChange} className={`${layout === 'console' ? 'h-7 min-w-0 flex-1 justify-between' : 'h-7'} gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]`} aria-label={t('storyStage.replyTarget.open')}>
          <Pencil className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="truncate">{t('storyStage.replyTarget.compact', { count: currentValue })}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" sideOffset={6} className="nova-panel w-64 border border-[var(--nova-border)] p-3 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
        <div className="mb-2 text-xs font-medium">{t('storyStage.replyTarget.title')}</div>
        <Input className="nova-field text-xs" type="number" min={1} value={draft} onChange={(event) => { setDraft(event.target.value); setError('') }} onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void save() } }} />
        {error ? <div className="mt-2 text-[11px] leading-4 text-[var(--nova-danger)]">{error}</div> : null}
        <div className="mt-3 flex justify-end gap-2"><Button variant="ghost" size="xs" onClick={() => setOpen(false)}>{t('common.cancel')}</Button><Button size="xs" disabled={saving} onClick={() => void save()}>{saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}{t('common.save')}</Button></div>
      </PopoverContent>
    </Popover>
  )
}

function normalizeReplyTargetChars(value?: number) {
  return value && value > 0 ? value : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS
}
