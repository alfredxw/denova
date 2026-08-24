import { describe, expect, it } from 'vitest'
import type { ChapterSummary } from '@/lib/api'
import {
  allocateChapterPath,
  allocateVolumePath,
  InvalidManuscriptFormatError,
} from './manuscript-paths'

function chapter(path: string, index: number): ChapterSummary {
  return {
    path,
    file_name: path.split('/').at(-1) || '',
    display_title: '',
    index,
    words: 0,
    status: 'not_started',
    confirmed: false,
    updated_at: '',
    volume: '',
    volume_path: path.split('/').slice(0, -1).join('/'),
  }
}

describe('manuscript path allocation', () => {
  it('appends a chapter with the configured hidden order', () => {
    expect(allocateChapterPath({
      chapters: [chapter('chapters/ch00002-第二章-旧路.md', 2)],
      title: '潮声',
      volumePath: 'chapters',
      chapterLabel: (order) => `第${order}章`,
    })).toBe('chapters/ch00003-第3章-潮声.md')
  })

  it('creates a volume and first chapter with custom formats', () => {
    const volumePath = allocateVolumePath({
      volumePaths: ['chapters/v00002-旧卷'],
      title: '荒原 / 重逢',
      format: 'volume-{order:03}-{volume}',
    })
    expect(volumePath).toBe('chapters/volume-003-荒原-重逢')
    expect(allocateChapterPath({
      chapters: [chapter('chapters/v00002-旧卷/ch00008-旧章.md', 8)],
      title: '相逢',
      volumePath,
      format: 'chapter-{order:03}-{title}.txt',
      chapterLabel: (order) => `Chapter ${order}`,
    })).toBe('chapters/volume-003-荒原-重逢/chapter-009-相逢.txt')
  })

  it('advances past an occupied path and rejects a non-unique format', () => {
    const chapters = [
      chapter('chapters/ch00001-第1章-起点.md', 1),
      chapter('chapters/ch00002-第2章-潮声.md', 1),
    ]
    expect(allocateChapterPath({
      chapters,
      title: '潮声',
      volumePath: 'chapters',
      chapterLabel: (order) => `第${order}章`,
    })).toBe('chapters/ch00003-第3章-潮声.md')
    expect(() => allocateChapterPath({
      chapters: [chapter('chapters/固定.md', 1)],
      title: '固定',
      volumePath: 'chapters',
      format: '{title}.md',
      chapterLabel: (order) => `第${order}章`,
    })).toThrow(InvalidManuscriptFormatError)
  })
})
