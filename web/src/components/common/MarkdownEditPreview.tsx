import type { ReactNode } from 'react'
import { Eye, PencilLine } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ThemedMarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

interface MarkdownViewToggleProps {
  preview: boolean
  onPreviewChange: (preview: boolean) => void
  /** 非 Markdown 内容禁用预览 */
  previewDisabled?: boolean
  previewDisabledReason?: string
  className?: string
}

/** 编辑 / 预览分段切换按钮，配合 MarkdownEditPreview 使用。 */
export function MarkdownViewToggle({ preview, onPreviewChange, previewDisabled = false, previewDisabledReason, className }: MarkdownViewToggleProps) {
  const { t } = useTranslation()
  return (
    <div className={cn('inline-flex shrink-0 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5', className)}>
      <button
        type="button"
        onClick={() => onPreviewChange(true)}
        disabled={previewDisabled}
        aria-pressed={preview}
        title={previewDisabled ? previewDisabledReason : t('common.preview')}
        className={cn(
          'nova-nav-item inline-flex h-6 items-center gap-1 rounded px-2 text-[11px] disabled:cursor-not-allowed disabled:opacity-45',
          preview ? 'is-active' : 'text-[var(--nova-text-muted)]',
        )}
      >
        <Eye className="h-3.5 w-3.5" />
        {t('common.preview')}
      </button>
      <button
        type="button"
        onClick={() => onPreviewChange(false)}
        aria-pressed={!preview}
        className={cn(
          'nova-nav-item inline-flex h-6 items-center gap-1 rounded px-2 text-[11px]',
          !preview ? 'is-active' : 'text-[var(--nova-text-muted)]',
        )}
      >
        <PencilLine className="h-3.5 w-3.5" />
        {t('common.raw')}
      </button>
    </div>
  )
}

interface MarkdownEditPreviewProps {
  value: string
  onChange: (value: string) => void
  preview: boolean
  readOnly?: boolean
  /** 预览渲染用的内容（缺省用 value，可用于剔除 frontmatter 等） */
  previewContent?: string
  /** 自定义编辑器（如带搜索高亮的 textarea），缺省为等宽 Textarea */
  renderEditor?: (props: { value: string; onChange: (value: string) => void; readOnly: boolean }) => ReactNode
  className?: string
}

/** Markdown 编辑 / 预览主体区：preview 时渲染 Markdown，否则渲染编辑器。 */
export function MarkdownEditPreview({ value, onChange, preview, readOnly = false, previewContent, renderEditor, className }: MarkdownEditPreviewProps) {
  if (preview) {
    return (
      <div className={cn('min-h-0 flex-1 overflow-y-auto bg-[var(--nova-bg)] px-5 py-4', className)}>
        <ThemedMarkdownRenderer content={previewContent ?? value} className="max-w-4xl text-xs leading-5" />
      </div>
    )
  }
  if (renderEditor) {
    return <>{renderEditor({ value, onChange, readOnly })}</>
  }
  return (
    <Textarea
      autoResize={false}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      readOnly={readOnly}
      spellCheck={false}
      className={cn('min-h-0 flex-1 resize-none rounded-none border-0 bg-[var(--nova-bg)] px-5 py-4 font-mono text-xs leading-5 text-[var(--nova-text)] shadow-none focus-visible:ring-0', className)}
    />
  )
}
