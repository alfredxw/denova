import { X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'

interface ReferenceChipsProps {
  files: string[]
  onRemove?: (path: string) => void
  prefix?: string
  tone?: 'file' | 'style' | 'lore'
}

/** 引用文件标签列表，展示本次 Chat 会随消息发送的文件。 */
export function ReferenceChips({ files, onRemove, prefix = '@', tone = 'file' }: ReferenceChipsProps) {
  if (files.length === 0) return null
  const toneClass = tone === 'style'
    ? 'bg-[var(--nova-style-chip-bg)] text-[var(--nova-style-chip-text)] hover:bg-[var(--nova-style-chip-hover-bg)]'
    : tone === 'lore'
      ? 'bg-[var(--nova-lore-chip-bg)] text-[var(--nova-lore-chip-text)] hover:bg-[var(--nova-lore-chip-hover-bg)]'
      : 'bg-[var(--nova-chip-bg)] text-[var(--nova-text)] hover:bg-[var(--nova-chip-hover-bg)]'
  const closeClass = 'text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'

  return (
    <div className="mb-2 flex flex-wrap gap-1.5">
      {files.map((file) => (
        <Badge
          key={file}
          variant="secondary"
          className={`max-w-full gap-1 ${toneClass}`}
        >
          <span className="truncate">{prefix}{file}</span>
          {onRemove && (
            <button
              type="button"
              className={`rounded ${closeClass}`}
              onClick={() => onRemove(file)}
            >
              <X className="h-3 w-3" />
            </button>
          )}
        </Badge>
      ))}
    </div>
  )
}
