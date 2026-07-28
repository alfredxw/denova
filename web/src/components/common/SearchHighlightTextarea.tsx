import { forwardRef, useLayoutEffect, useRef } from 'react'
import { Textarea, type TextareaProps } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { HighlightedText } from '@/components/common/HighlightedText'

interface SearchHighlightTextareaProps extends TextareaProps {
  /** 搜索关键词，为空时退化为普通 Textarea。 */
  highlightQuery?: string
  /** 固定高度编辑器需要让外层参与 flex 布局。 */
  containerClassName?: string
}

/**
 * 在 Textarea 基础上叠加一层搜索关键词高亮。
 *
 * 当 `highlightQuery` 非空时，textarea 的文字被设为透明（光标仍可见），
 * 下方叠加一层 `<pre>` 显示相同文本并把匹配关键词包裹在 `<mark>` 中。
 * 两层共享完全一致的字体、padding、line-height，因此视觉上重合。
 *
 * 自动高度与固定高度 Textarea 均可使用。固定高度时会同步两层滚动位置，
 * 让长正文的高亮始终与可编辑文字重合。
 */
export const SearchHighlightTextarea = forwardRef<HTMLTextAreaElement, SearchHighlightTextareaProps>(function SearchHighlightTextarea({
  highlightQuery,
  containerClassName,
  className,
  value,
  onScroll,
  style,
  ...props
}, forwardedRef) {
  const query = highlightQuery?.trim() || ''
  const text = String(value ?? '')
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const overlayRef = useRef<HTMLPreElement | null>(null)

  const syncScroll = () => {
    const textarea = textareaRef.current
    const overlay = overlayRef.current
    if (!textarea || !overlay) return
    overlay.scrollTop = textarea.scrollTop
    overlay.scrollLeft = textarea.scrollLeft
  }

  useLayoutEffect(syncScroll, [query, text])

  return (
    <div className={cn('relative w-full', containerClassName)}>
      {query ? (
        <pre
          ref={overlayRef}
          aria-hidden="true"
          className={cn(
            className,
            'pointer-events-none absolute inset-0 m-0 overflow-hidden whitespace-pre-wrap break-words border-transparent bg-transparent',
          )}
        >
          <HighlightedText text={text} query={query} />
        </pre>
      ) : null}
      <Textarea
        ref={(node) => {
          textareaRef.current = node
          if (typeof forwardedRef === 'function') forwardedRef(node)
          else if (forwardedRef) forwardedRef.current = node
        }}
        value={value}
        className={className}
        style={query ? {
          ...style,
          color: 'transparent',
          caretColor: 'var(--nova-text)',
        } : style}
        onScroll={(event) => {
          syncScroll()
          onScroll?.(event)
        }}
        {...props}
      />
    </div>
  )
})
