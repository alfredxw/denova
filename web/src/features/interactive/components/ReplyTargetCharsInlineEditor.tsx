import { useEffect, useState } from 'react'
import { Loader2, Pencil } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS } from '../opening'
import type { StorySummary } from '../types'

// 每轮目标字数的行内编辑：默认直接展示当前值，点击进入行内输入，
// Enter/失焦保存，Esc 取消——不再弹出小浮层。
export function ReplyTargetCharsInlineEditor({ story, onChange }: { story?: StorySummary; onChange?: (replyTargetChars: number) => void | Promise<void> }) {
  const { t } = useTranslation()
  const currentValue = story?.reply_target_chars && story.reply_target_chars > 0 ? story.reply_target_chars : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!editing) setError('')
  }, [editing])

  const start = () => {
    setDraft(String(currentValue))
    setEditing(true)
  }

  const cancel = () => {
    setEditing(false)
    setError('')
  }

  const save = async () => {
    const nextValue = Number(draft)
    if (!Number.isFinite(nextValue) || nextValue <= 0) {
      setError(t('storyStage.replyTarget.invalid'))
      return
    }
    if (Math.floor(nextValue) === currentValue) {
      cancel()
      return
    }
    setSaving(true)
    setError('')
    try {
      await onChange?.(Math.floor(nextValue))
      setEditing(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyStage.replyTarget.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  if (!editing) {
    return (
      <button
        type="button"
        disabled={!story || !onChange}
        onClick={start}
        aria-label={t('storyStage.replyTarget.open')}
        title={t('storyStage.replyTarget.open')}
        className="group flex h-7 min-w-0 items-center gap-1.5 rounded-[8px] border border-transparent px-1.5 text-[11px] text-[var(--nova-text-muted)] transition-colors hover:border-[var(--nova-border)] hover:bg-[var(--nova-surface)] hover:text-[var(--nova-text)] disabled:opacity-45"
      >
        <span className="truncate">{t('storyStage.replyTarget.compact', { count: currentValue })}</span>
        <Pencil className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)] opacity-0 transition-opacity group-hover:opacity-100" />
      </button>
    )
  }

  return (
    <span className="flex h-7 min-w-0 items-center gap-1.5" title={error || undefined}>
      <Input
        autoFocus
        type="number"
        min={1}
        value={draft}
        disabled={saving}
        aria-label={t('storyStage.replyTarget.open')}
        className={`nova-field h-7 w-20 px-1.5 text-[11px] ${error ? 'border-[var(--nova-danger-border)]' : ''}`}
        onChange={(event) => { setDraft(event.target.value); setError('') }}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault()
            void save()
          }
          if (event.key === 'Escape') {
            event.preventDefault()
            cancel()
          }
        }}
        onBlur={() => { if (!saving) void save() }}
      />
      {saving ? <Loader2 className="h-3 w-3 shrink-0 animate-spin text-[var(--nova-text-faint)]" /> : null}
    </span>
  )
}
