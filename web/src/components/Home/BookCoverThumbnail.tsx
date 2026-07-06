import { useEffect, useState } from 'react'
import { BookOpen } from 'lucide-react'
import { bookCoverURL, type BookRecord } from '@/lib/api'

interface BookCoverThumbnailProps {
  book?: Pick<BookRecord, 'name' | 'path'> | null
  version?: string
  previewURL?: string
  title?: string
  className?: string
  iconClassName?: string
}

export function BookCoverThumbnail({ book, version, previewURL, title, className, iconClassName }: BookCoverThumbnailProps) {
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setFailed(false)
  }, [book?.path, version, previewURL])

  const imageTitle = title || book?.name || book?.path || ''
  const src = previewURL || (book?.path ? bookCoverURL(book.path, version) : '')
  const showImage = Boolean(src) && !failed

  return (
    <div
      className={`relative flex min-w-0 items-center justify-center overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] ${className || ''}`}
      title={imageTitle}
    >
      {showImage ? (
        <img
          src={src}
          alt={imageTitle}
          className="h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      ) : (
        <BookOpen className={iconClassName || 'h-4 w-4 text-[var(--nova-text-muted)]'} />
      )}
    </div>
  )
}
