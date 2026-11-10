import { useEffect, useState } from 'react'
import { BookOpen } from 'lucide-react'
import { bookCoverURL, type BookRecord } from '@/lib/api'

interface BookCoverThumbnailProps {
  book?: Pick<BookRecord, 'name' | 'path'> | null
  version?: string
  previewURL?: string
  title?: string
  decorative?: boolean
  className?: string
  iconClassName?: string
}

export function BookCoverThumbnail({ book, version, previewURL, title, decorative = false, className, iconClassName }: BookCoverThumbnailProps) {
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setFailed(false)
  }, [book?.path, version, previewURL])

  const imageTitle = title || book?.name || book?.path || ''
  // No cover timestamp means no cover exists; render the placeholder without a 404 probe.
  const src = previewURL || (book?.path && version ? bookCoverURL(book.path, version) : '')
  const showImage = Boolean(src) && !failed

  return (
    <div
      className={`relative flex min-w-0 items-center justify-center overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] ${className || ''}`}
      aria-hidden={decorative || undefined}
    >
      {showImage ? (
        <img
          loading="lazy"
          decoding="async"
          src={src}
          alt={decorative ? '' : imageTitle}
          className="absolute inset-0 h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      ) : (
        <BookOpen className={iconClassName || 'h-4 w-4 text-[var(--nova-text-muted)]'} />
      )}
    </div>
  )
}
