import { useEffect, useRef, useState } from 'react'
import type { Editor } from '@tiptap/react'
import { MessageSquareQuote } from 'lucide-react'
import { useTranslation } from 'react-i18next'

/** 选区浮动工具条，定位在光标（选区 head 端）旁边。 */
export function SelectionToolbar({ editor, onQuote }: { editor: Editor; onQuote: () => void }) {
  const { t } = useTranslation()
  const [coords, setCoords] = useState<{ top: number; left: number } | null>(null)
  const toolbarRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const updatePosition = () => {
      const { from, to, head } = editor.state.selection
      if (from === to) {
        setCoords(null)
        return
      }
      try {
        const headCoords = editor.view.coordsAtPos(head)
        const containerEl = editor.view.dom.closest('.relative') as HTMLElement | null
        if (!containerEl) { setCoords(null); return }
        const containerRect = containerEl.getBoundingClientRect()
        const scrollTop = containerEl.scrollTop
        const toolbarWidth = toolbarRef.current?.offsetWidth ?? 100
        // coordsAtPos 返回视口坐标，需加上 scrollTop 转换为容器内容区域坐标
        let top = headCoords.bottom - containerRect.top + scrollTop + 4
        let left = headCoords.left - containerRect.left
        // 防止溢出右侧
        const maxLeft = containerRect.width - toolbarWidth - 8
        if (left > maxLeft) left = maxLeft
        if (left < 4) left = 4
        // 如果下方空间不够（相对当前可见区域），改为显示在光标行上方
        const toolbarHeight = toolbarRef.current?.offsetHeight ?? 32
        const visibleBottom = scrollTop + containerRect.height
        if (top + toolbarHeight > visibleBottom) {
          top = headCoords.top - containerRect.top + scrollTop - toolbarHeight - 4
        }
        setCoords({ top: Math.max(scrollTop, top), left })
      } catch {
        setCoords(null)
      }
    }
    updatePosition()
    editor.on('selectionUpdate', updatePosition)
    return () => { editor.off('selectionUpdate', updatePosition) }
  }, [editor])

  if (!coords) return null

  return (
    <div
      ref={toolbarRef}
      className="absolute z-30 flex items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-menu-bg)] px-1.5 py-1 shadow-xl backdrop-blur"
      style={{ top: coords.top, left: coords.left }}
    >
      <button
        type="button"
        className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-menu-item-hover-bg)] hover:text-[var(--nova-text)]"
        onClick={onQuote}
        title={t('editor.quoteSelectionShortcut')}
      >
        <MessageSquareQuote className="h-3.5 w-3.5" />
        <span>{t('editor.quoteSelection')}</span>
      </button>
    </div>
  )
}
