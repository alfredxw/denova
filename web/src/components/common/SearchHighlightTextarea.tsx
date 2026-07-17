import { Textarea, type TextareaProps } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { HighlightedText } from '@/components/common/HighlightedText'

interface SearchHighlightTextareaProps extends TextareaProps {
  /** 搜索关键词，为空时退化为普通 Textarea。 */
  highlightQuery?: string
}

/**
 * 在 Textarea 基础上叠加一层搜索关键词高亮。
 *
 * 当 `highlightQuery` 非空时，textarea 的文字被设为透明（光标仍可见），
 * 下方叠加一层 `<pre>` 显示相同文本并把匹配关键词包裹在 `<mark>` 中。
 * 两层共享完全一致的字体、padding、line-height，因此视觉上重合。
 *
 * 依赖 Textarea 的 autoResize：父容器高度由 textarea 撑开，
 * 高亮层以 `absolute inset-0` 填充，故高度自动跟随。
 */
export function SearchHighlightTextarea({ highlightQuery, className, value, ...props }: SearchHighlightTextareaProps) {
  const query = highlightQuery?.trim() || ''
  const text = String(value ?? '')

  return (
    <div className="relative w-full">
      {query ? (
        <pre
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
        value={value}
        className={className}
        style={query ? { color: 'transparent', caretColor: 'currentColor' } : undefined}
        {...props}
      />
    </div>
  )
}
