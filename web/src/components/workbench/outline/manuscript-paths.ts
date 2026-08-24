import type { ChapterSummary } from '@/lib/api'

export const DEFAULT_CHAPTER_FILENAME_FORMAT = 'ch{order:05}-{chapter}-{title}.md'
export const DEFAULT_VOLUME_DIR_FORMAT = 'v{order:05}-{volume}'

export class InvalidManuscriptFormatError extends Error {}

interface ChapterPathInput {
  chapters: ChapterSummary[]
  title: string
  volumePath: string
  format?: string
  chapterLabel: (order: number) => string
}

interface VolumePathInput {
  volumePaths: readonly string[]
  title: string
  format?: string
}

/** Allocates the next append-only chapter path without renaming existing prose. */
export function allocateChapterPath({ chapters, title, volumePath, format, chapterLabel }: ChapterPathInput) {
  const existingPaths = new Set(chapters.map((chapter) => chapter.path))
  const attemptedPaths = new Set<string>()
  let order = Math.max(0, ...chapters.map((chapter) => chapter.index)) + 1
  while (true) {
    const filename = formatName(format || DEFAULT_CHAPTER_FILENAME_FORMAT, {
      order,
      chapter: chapterLabel(order),
      title,
    })
    if (!/\.(?:md|txt)$/i.test(filename)) throw new InvalidManuscriptFormatError('Chapter files must use .md or .txt')
    const path = `${volumePath || 'chapters'}/${filename}`
    if (!existingPaths.has(path)) return path
    if (attemptedPaths.has(path)) throw new InvalidManuscriptFormatError('Chapter filename format cannot allocate a unique path')
    attemptedPaths.add(path)
    order++
  }
}

/** Allocates a new volume directory; the caller creates its first chapter at the returned path. */
export function allocateVolumePath({ volumePaths, title, format }: VolumePathInput) {
  const existingPaths = new Set(volumePaths.filter((path) => path !== 'chapters'))
  const attemptedPaths = new Set<string>()
  let order = Math.max(existingPaths.size, ...Array.from(existingPaths, hiddenVolumeOrder)) + 1
  while (true) {
    const dirname = formatName(format || DEFAULT_VOLUME_DIR_FORMAT, { order, volume: title })
    const path = `chapters/${dirname}`
    if (!existingPaths.has(path)) return path
    if (attemptedPaths.has(path)) throw new InvalidManuscriptFormatError('Volume directory format cannot allocate a unique path')
    attemptedPaths.add(path)
    order++
  }
}

function formatName(format: string, values: { order: number; chapter?: string; title?: string; volume?: string }) {
  const replacements = {
    chapter: safeFilenamePart(values.chapter || ''),
    title: safeFilenamePart(values.title || ''),
    volume: safeFilenamePart(values.volume || ''),
  }
  if ((values.title !== undefined && !replacements.title) || (values.volume !== undefined && !replacements.volume)) {
    throw new InvalidManuscriptFormatError('The manuscript title cannot form a filename')
  }
  const name = format.trim()
    .replace(/\{order(?::0?(\d+))?\}/g, (_match, width: string | undefined) => (
      width ? String(values.order).padStart(Number(width), '0') : String(values.order)
    ))
    .replaceAll('{chapter}', replacements.chapter)
    .replaceAll('{title}', replacements.title)
    .replaceAll('{volume}', replacements.volume)
    .trim()
  if (!name || name === '.' || name === '..' || name.startsWith('.') || name.length > 240 || /[\\/:*?"<>|{}\0]/.test(name)) {
    throw new InvalidManuscriptFormatError('The manuscript naming format is invalid')
  }
  return name
}

function safeFilenamePart(input: string) {
  return Array.from(input.trim()).reduce((result, character) => {
    const separator = /[\\/:*?"<>|\s]/u.test(character)
    if (separator) return result.endsWith('-') ? result : `${result}-`
    return result.length >= 48 ? result : result + character
  }, '').replace(/^[-.]+|[-.]+$/g, '')
}

function hiddenVolumeOrder(path: string) {
  const match = /(?:^|\/)v(\d{5})(?:[-_ ]|$)/i.exec(path)
  return match ? Number(match[1]) : 0
}
