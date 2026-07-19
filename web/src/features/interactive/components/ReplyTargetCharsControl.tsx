import { Pencil } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { isSaveShortcut } from '@/lib/keyboard'
import { DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS } from '../opening'
import type { StorySummary } from '../types'

interface ReplyTargetDraft {
  id: string
  updated_at?: string
  raw: string
}

interface ReplyTargetPayload {
  value: number
}

interface ReplyTargetSaved extends ReplyTargetPayload {
  updated_at?: string
}

function replyTargetSignature(value: Partial<ReplyTargetDraft> | ReplyTargetPayload | ReplyTargetSaved) {
  return 'value' in value ? String(value.value) : value.raw || ''
}

export function ReplyTargetCharsControl({ story, onChange, layout = 'inline' }: { story?: StorySummary; onChange?: (replyTargetChars: number) => void | Promise<void>; layout?: 'inline' | 'console' }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(String(normalizeReplyTargetChars(story?.reply_target_chars)))
  const [error, setError] = useState('')
  const storyId = story?.id
  const storyUpdatedAt = story?.updated_at
  const currentValue = normalizeReplyTargetChars(story?.reply_target_chars)
  const baselineValueRef = useRef(currentValue)
  const baselineStoryIdRef = useRef(storyId)
  const parsedDraft = Number(draft)
  const valid = Number.isFinite(parsedDraft) && parsedDraft > 0
  const autosaveDraft = useMemo<ReplyTargetDraft | null>(() => storyId ? {
    id: storyId,
    updated_at: storyUpdatedAt,
    raw: draft,
  } : null, [draft, storyId, storyUpdatedAt])

  const autosave = useResourceAutosave<ReplyTargetDraft, ReplyTargetPayload, ReplyTargetSaved>({
    draft: autosaveDraft,
    active: open && Boolean(story && onChange),
    scopeKey: storyId || '',
    valid,
    makePayload: (value) => ({ value: Math.floor(Number(value.raw)) }),
    baselineFromSaved: (saved, submitted) => ({
      ...submitted,
      raw: String(saved.value),
      updated_at: saved.updated_at,
    }),
    signature: replyTargetSignature,
    save: async (_id, payload) => {
      await onChange?.(payload.value)
      return { value: payload.value, updated_at: storyUpdatedAt }
    },
    onSaved: (saved) => {
      baselineValueRef.current = saved.value
    },
  })

  useEffect(() => {
    const nextDraft = String(currentValue)
    const previousDraft = String(baselineValueRef.current)
    const storyChanged = baselineStoryIdRef.current !== storyId
    setDraft((current) => storyChanged || current === previousDraft || current === nextDraft ? nextDraft : current)
    baselineStoryIdRef.current = storyId
    baselineValueRef.current = currentValue
    autosave.resetBaseline(storyId ? {
      id: storyId,
      updated_at: storyUpdatedAt,
      raw: nextDraft,
    } : null)
  }, [autosave.resetBaseline, currentValue, storyId, storyUpdatedAt])

  useEffect(() => {
    if (!open) {
      setDraft(String(currentValue))
      setError('')
    }
  }, [currentValue, open])

  const flush = useCallback(async (force = false) => {
    try {
      const pending = autosave.flushPending()
      if (pending) {
        await pending
      } else if (force || autosave.status === 'error') {
        await autosave.saveNow('manual')
      }
      return true
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : t('storyStage.replyTarget.saveFailed'))
      return false
    }
  }, [autosave.flushPending, autosave.saveNow, autosave.status, t])

  const close = async () => {
    if (!await flush()) return
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={(nextOpen) => {
      if (nextOpen) setOpen(true)
      else void close()
    }}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={!story || !onChange} className={`${layout === 'console' ? 'h-7 min-w-0 flex-1 justify-between' : 'h-7'} gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]`} aria-label={t('storyStage.replyTarget.open')}>
          <Pencil className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="truncate">{t('storyStage.replyTarget.compact', { count: currentValue })}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" sideOffset={6} className="nova-panel w-64 border border-[var(--nova-border)] p-3 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
        <div className="mb-2 text-xs font-medium">{t('storyStage.replyTarget.title')}</div>
        <Input
          className="nova-field text-xs"
          type="number"
          min={1}
          value={draft}
          onChange={(event) => {
            const value = event.target.value
            setDraft(value)
            const parsed = Number(value)
            setError(Number.isFinite(parsed) && parsed > 0 ? '' : t('storyStage.replyTarget.invalid'))
          }}
          onKeyDown={(event) => {
            if (event.key !== 'Enter' && !isSaveShortcut(event)) return
            event.preventDefault()
            event.stopPropagation()
            void flush(true)
          }}
        />
        {error ? <div className="mt-2 text-[11px] leading-4 text-[var(--nova-danger)]">{error}</div> : null}
        <div className="mt-3 flex items-center justify-between gap-2">
          <AutosaveStatusIndicator status={autosave.status} error={autosave.error} onRetry={autosave.retry} />
          <Button variant="ghost" size="xs" onClick={() => void close()}>{t('common.close')}</Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function normalizeReplyTargetChars(value?: number) {
  return value && value > 0 ? value : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS
}
