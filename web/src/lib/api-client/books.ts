import { jsonHeaders, parseSSEStream, requestJSON } from './client'
import type { BookMeta, BookRecord, NovelImportPreview, NovelImportResult, SSEEvent } from './types'

export async function getBooks(): Promise<BookRecord[]> {
  const data = await requestJSON<{ books: BookRecord[] }>('/api/books')
  return data.books || []
}

export async function removeBook(path: string): Promise<{ message: string; workspace: string }> {
  return requestJSON('/api/books/remove', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ path }),
  })
}

export async function reorderBooks(paths: string[]): Promise<{ message: string }> {
  return requestJSON('/api/books/reorder', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ paths }),
  })
}

export async function previewNovelImport(
  file: File,
  options: { sampleChars?: number; splitRegex?: string; splitStrategy?: string } = {},
): Promise<NovelImportPreview> {
  const form = new FormData()
  form.append('file', file)
  if (options.sampleChars !== undefined) form.append('sample_chars', String(options.sampleChars))
  if (options.splitRegex !== undefined) form.append('split_regex', options.splitRegex)
  if (options.splitStrategy) form.append('split_strategy', options.splitStrategy)
  return requestJSON('/api/books/import-novel/preview', {
    method: 'POST',
    body: form,
  })
}

export async function previewNovelImportStream(
  file: File,
  options: { sampleChars?: number; splitRegex?: string; splitStrategy?: string } = {},
): Promise<ReadableStream<SSEEvent>> {
  const form = new FormData()
  form.append('file', file)
  if (options.sampleChars !== undefined) form.append('sample_chars', String(options.sampleChars))
  if (options.splitRegex !== undefined) form.append('split_regex', options.splitRegex)
  if (options.splitStrategy) form.append('split_strategy', options.splitStrategy)
  const res = await fetch('/api/books/import-novel/preview/stream', {
    method: 'POST',
    body: form,
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error || `HTTP ${res.status}`)
  }
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function importNovel(
  file: File,
  options: { bookTitle?: string; author?: string; description?: string; sampleChars?: number; splitRegex?: string; splitStrategy?: string } = {},
): Promise<NovelImportResult> {
  const form = new FormData()
  form.append('file', file)
  if (options.bookTitle) form.append('book_title', options.bookTitle)
  if (options.author) form.append('author', options.author)
  if (options.description) form.append('description', options.description)
  if (options.sampleChars !== undefined) form.append('sample_chars', String(options.sampleChars))
  if (options.splitRegex !== undefined) form.append('split_regex', options.splitRegex)
  if (options.splitStrategy) form.append('split_strategy', options.splitStrategy)
  return requestJSON('/api/books/import-novel', {
    method: 'POST',
    body: form,
  })
}

export async function createBook(title: string, author?: string, description?: string): Promise<{ workspace: string; book_meta: BookMeta }> {
  return requestJSON('/api/books/create', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ title, author: author ?? '', description: description ?? '' }),
  })
}

export async function getBookInfo(path: string): Promise<BookMeta> {
  return requestJSON(`/api/books/info?path=${encodeURIComponent(path)}`)
}

export async function updateBookInfo(path: string, title: string, author: string, description: string): Promise<BookMeta> {
  return requestJSON('/api/books/info', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({ path, title, author, description }),
  })
}
