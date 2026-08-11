import { afterEach, describe, expect, it, vi } from 'vitest'
import { importNovel } from './books'

describe('importNovel stream', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('resolves with the result from the final done event', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(
      'event: progress\ndata: {"step":"uploaded"}\n\nevent: done\ndata: {"workspace":"/books/imported","title":"测试书","chapter_count":2,"total_chars":10,"chapter_paths":["chapters/ch01.md"]}\n\n',
      { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
    )))

    const result = await importNovel(new File(['x'], 'novel.txt'), { bookTitle: '测试书' })

    expect(result.workspace).toBe('/books/imported')
    expect(result.chapter_count).toBe(2)
    expect(result.title).toBe('测试书')
  })

  it('rejects with the server error on an error event', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(
      'event: error\ndata: {"error":"parse failed"}\n\n',
      { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
    )))

    await expect(importNovel(new File(['x'], 'novel.txt'), {})).rejects.toThrow('parse failed')
  })
})
