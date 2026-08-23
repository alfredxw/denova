import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ReactNode, type RefCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { lineDiffStats } from '@/features/changes/diff-stats'
import { estimateUnifiedReviewLineCount } from '@/features/changes/review/monaco/unified-review-projection'
import type { DiffFileDocument, DiffLayout } from './types'

interface DiffFileSectionProps {
  file: DiffFileDocument
  layout: DiffLayout
  active: boolean
  preRender?: boolean
  collapsed: boolean
  pinned?: boolean
  sectionRef: RefCallback<HTMLElement>
  onToggle: () => void
  headerMeta?: ReactNode
  action?: ReactNode
  banner?: ReactNode
  renderContent: (props: { initialHeight: number; onHeightChange: (height: number) => void }) => ReactNode
}

/** One independently collapsible file in a shared, continuous multi-file Diff surface. */
export function DiffFileSection({ file, layout, active, preRender = false, collapsed, pinned = false, sectionRef, onToggle, headerMeta, action, banner, renderContent }: DiffFileSectionProps) {
  const { t } = useTranslation()
  const contentRef = useRef<HTMLDivElement | null>(null)
  const [nearViewport, setNearViewport] = useState(() => typeof window === 'undefined' || !('IntersectionObserver' in window))
  const stats = useMemo(() => file.additions === undefined || file.deletions === undefined
    ? lineDiffStats(file.before_content, file.after_content)
    : { additions: file.additions, deletions: file.deletions }, [file.additions, file.after_content, file.before_content, file.deletions])
  const estimatedHeight = useMemo(() => diffEditorHeight(file, layout), [file, layout])
  const [measuredHeight, setMeasuredHeight] = useState(estimatedHeight)
  const measuredHeightRef = useRef(estimatedHeight)
  const deleted = file.after_exists === false
  const renderEditor = pinned || (!collapsed && (nearViewport || active || preRender))

  useEffect(() => {
    measuredHeightRef.current = estimatedHeight
    setMeasuredHeight(estimatedHeight)
  }, [estimatedHeight, file.base_revision, file.path, file.revision, layout])

  useEffect(() => {
    if (active || preRender) setNearViewport(true)
  }, [active, preRender])

  useEffect(() => {
    if (collapsed) {
      if (!pinned) setNearViewport(false)
      return
    }
    const node = contentRef.current
    if (!node || !('IntersectionObserver' in window)) {
      setNearViewport(true)
      return
    }
    const observer = new window.IntersectionObserver((entries) => {
      setNearViewport(entries.some((entry) => entry.isIntersecting))
    }, { rootMargin: '640px 0px' })
    observer.observe(node)
    return () => observer.disconnect()
  }, [collapsed, pinned])

  return (
    <section ref={sectionRef} data-review-file={file.path} className="scroll-mt-1 border-b border-[var(--nova-border)] bg-[var(--nova-bg)]" aria-label={file.path}>
      <div className={`sticky top-0 z-20 flex min-h-9 items-center border-l-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] pr-2 ${active ? 'border-l-[var(--nova-accent-blue)]' : 'border-l-transparent'}`}>
        <button
          type="button"
          aria-expanded={!collapsed}
          aria-label={t(collapsed ? 'changes.expandFile' : 'changes.collapseFile', { path: file.path })}
          onClick={onToggle}
          className="flex min-w-0 flex-1 items-center gap-2 self-stretch px-2 text-left hover:bg-[var(--nova-hover)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-[var(--nova-accent-blue)]"
        >
          {collapsed ? <ChevronRight className="h-3.5 w-3.5 shrink-0" /> : <ChevronDown className="h-3.5 w-3.5 shrink-0" />}
          <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[var(--nova-text)]">{file.path}</span>
        </button>
        {headerMeta}
        {deleted && <span className="mr-2 text-[10px] text-[var(--nova-danger)]">{t('changes.fileDeleted')}</span>}
        <span className="mr-1.5 font-mono text-[10px] text-[var(--nova-success)]">+{stats.additions}</span>
        <span className="mr-1.5 font-mono text-[10px] text-[var(--nova-danger)]">−{stats.deletions}</span>
        {action}
      </div>

      <div ref={contentRef} data-review-file-content={file.path} hidden={collapsed}>
        {banner}
        <div style={{ height: measuredHeight }}>
          {renderEditor ? renderContent({
            initialHeight: estimatedHeight,
            onHeightChange: (height) => {
              if (height <= 0 || measuredHeightRef.current === height) return
              measuredHeightRef.current = height
              setMeasuredHeight(height)
            },
          }) : <DiffEditorLoading label={t('changes.loading')} />}
        </div>
      </div>
    </section>
  )
}

function diffEditorHeight(file: Pick<DiffFileDocument, 'before_content' | 'after_content' | 'binary'>, layout: DiffLayout): number {
  if (file.binary) return 160
  const lines = layout === 'unified'
    ? estimateUnifiedReviewLineCount(file.before_content, file.after_content)
    : Math.max(lineCount(file.before_content), lineCount(file.after_content))
  return Math.max(160, 28 + lines * 18)
}

function lineCount(value: string): number {
  return value ? value.split('\n').length : 1
}

function DiffEditorLoading({ label }: { label: string }) {
  return <div className="flex h-full items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]"><Loader2 className="h-4 w-4 animate-spin" />{label}</div>
}
