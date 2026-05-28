import { X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'

interface ReferenceChipsProps {
  files: string[]
  onRemove?: (path: string) => void
  prefix?: '@' | '#'
  tone?: 'file' | 'style'
}

/** 引用文件标签列表，展示本次 Chat 会随消息发送的文件。 */
export function ReferenceChips({ files, onRemove, prefix = '@', tone = 'file' }: ReferenceChipsProps) {
  if (files.length === 0) return null
  const toneClass = tone === 'style'
    ? 'bg-[#8b5cf6]/20 text-[#ddd6fe] hover:bg-[#8b5cf6]/25'
    : 'bg-[#4a4d54]/20 text-[#d7dbe2] hover:bg-[#4a4d54]/25'
  const closeClass = tone === 'style' ? 'text-[#c5c9d1] hover:text-white' : 'text-[#c5c9d1] hover:text-white'

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
