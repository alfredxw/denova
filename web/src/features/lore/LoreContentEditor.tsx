import { FileCode2, Type } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  MarkdownContentEditor,
  type MarkdownContentEditorReview,
} from '@/components/Editor/MarkdownContentEditor'
import { cn } from '@/lib/utils'

interface LoreContentEditorProps {
  projectId: string
  resourceKey: string
  value: string
  onChange: (value: string) => void
  onSaveShortcut?: () => void
  highlightQuery?: string
  review?: MarkdownContentEditorReview
  richAriaLabel: string
  sourceAriaLabel?: string
  className?: string
  editorClassName?: string
}

/** One content editor for Lore across Writing and Game, including raw Markdown mode. */
export function LoreContentEditor({
  projectId,
  resourceKey,
  value,
  onChange,
  onSaveShortcut,
  highlightQuery,
  review,
  richAriaLabel,
  sourceAriaLabel = richAriaLabel,
  className,
  editorClassName,
}: LoreContentEditorProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'rich' | 'raw'>('rich')

  return (
    <div className={cn('flex min-h-0 min-w-0 flex-1 bg-[var(--nova-bg)] p-2 sm:p-3', className)}>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-bg)]">
        <div className="flex h-9 shrink-0 items-center border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3">
          <div
            role="group"
            aria-label={t('settingPanel.field.content')}
            className="inline-flex shrink-0 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0.5"
          >
            <button
              type="button"
              onClick={() => setMode('rich')}
              aria-pressed={mode === 'rich'}
              className={cn(
                'nova-nav-item inline-flex h-5 items-center gap-1 rounded px-1.5 text-[10px]',
                mode === 'rich' ? 'is-active' : 'text-[var(--nova-text-muted)]',
              )}
            >
              <Type className="h-3 w-3" />
              {t('settingPanel.editor.contentModeRich')}
            </button>
            <button
              type="button"
              onClick={() => setMode('raw')}
              aria-pressed={mode === 'raw'}
              className={cn(
                'nova-nav-item inline-flex h-5 items-center gap-1 rounded px-1.5 text-[10px]',
                mode === 'raw' ? 'is-active' : 'text-[var(--nova-text-muted)]',
              )}
            >
              <FileCode2 className="h-3 w-3" />
              {t('common.raw')}
            </button>
          </div>
        </div>
        <MarkdownContentEditor
          projectId={projectId}
          key={resourceKey}
          mode={mode === 'raw' ? 'source' : 'rich'}
          value={value}
          onChange={onChange}
          onSaveShortcut={onSaveShortcut}
          highlightQuery={highlightQuery}
          review={review}
          aria-label={mode === 'raw' ? sourceAriaLabel : richAriaLabel}
          className={cn('flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto', editorClassName)}
        />
      </div>
    </div>
  )
}
