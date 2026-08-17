import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import type { LoreItem } from './types'
import {
  createProjectLoreItem,
  deleteProjectLoreItem,
  getProjectBookSnapshot,
  getProjectBookSummary,
  getProjectBookTree,
  getProjectLoreItems,
  setProjectChapterConfirmed,
  updateProjectLoreItem,
} from './project-book'

describe('project Book API', () => {
  it('uses one encoded Project resource for Writing and Lore projections', async () => {
    const requests: Array<{ method: string; path: string; body?: unknown }> = []
    const base = '/api/projects/book%20one/book'
    const summary = {
      title: 'Book One',
      author: '',
      chapter_count: 1,
      total_words: 2,
      chapters: null,
      chapter_plans: null,
    }
    const item: LoreItem = {
      id: 'hero',
      enabled: true,
      type: 'character',
      type_source: 'manual',
      name: 'Hero',
      importance: 'major',
      tags: [],
      brief_description: '',
      keywords: [],
      load_mode: 'resident',
      content: 'Body',
      created_at: 'r1',
      updated_at: 'r1',
    }
    server.use(
      http.get(base, () => HttpResponse.json({
        project_id: 'book one',
        workspace: '/books/one',
        tree: null,
        summary,
      })),
      http.get(`${base}/tree`, () => HttpResponse.json({ project_id: 'book one', tree: null })),
      http.get(`${base}/summary`, () => HttpResponse.json({ project_id: 'book one', summary })),
      http.patch(`${base}/chapter-status`, async ({ request }) => {
        requests.push({ method: request.method, path: new URL(request.url).pathname, body: await request.json() })
        return HttpResponse.json({ project_id: 'book one', path: 'chapters/ch01.md', confirmed: true })
      }),
      http.get(`${base}/lore/items`, () => HttpResponse.json({ project_id: 'book one', items: [item] })),
      http.post(`${base}/lore/items`, async ({ request }) => {
        requests.push({ method: request.method, path: new URL(request.url).pathname, body: await request.json() })
        return HttpResponse.json({ project_id: 'book one', item })
      }),
      http.put(`${base}/lore/items/hero`, async ({ request }) => {
        requests.push({ method: request.method, path: new URL(request.url).pathname, body: await request.json() })
        return HttpResponse.json({ project_id: 'book one', item: { ...item, name: 'Heroine', updated_at: 'r2' } })
      }),
      http.delete(`${base}/lore/items/hero`, ({ request }) => {
        requests.push({ method: request.method, path: new URL(request.url).pathname })
        return HttpResponse.json({ project_id: 'book one', status: 'ok' })
      }),
    )

    await expect(getProjectBookSnapshot('book one')).resolves.toMatchObject({
      project_id: 'book one',
      tree: [],
      summary: { chapters: [], chapter_plans: [] },
    })
    await expect(getProjectBookTree('book one')).resolves.toEqual([])
    await expect(getProjectBookSummary('book one')).resolves.toMatchObject({ chapters: [], chapter_plans: [] })
    await setProjectChapterConfirmed('book one', 'chapters/ch01.md', true)
    await expect(getProjectLoreItems('book one')).resolves.toEqual([item])
    await createProjectLoreItem('book one', { name: 'Hero' })
    await updateProjectLoreItem('book one', 'hero', item, 'r1')
    await deleteProjectLoreItem('book one', 'hero')

    expect(requests).toEqual([
      {
        method: 'PATCH',
        path: `${base}/chapter-status`,
        body: { path: 'chapters/ch01.md', confirmed: true },
      },
      { method: 'POST', path: `${base}/lore/items`, body: { name: 'Hero' } },
      { method: 'PUT', path: `${base}/lore/items/hero`, body: { ...item, base_revision: 'r1' } },
      { method: 'DELETE', path: `${base}/lore/items/hero` },
    ])
  })

  it('rejects a response bound to a different Project', async () => {
    server.use(
      http.get('/api/projects/book-a/book/tree', () => HttpResponse.json({
        project_id: 'book-b',
        workspace: '/books/b',
        tree: [],
      })),
    )

    await expect(getProjectBookTree('book-a')).rejects.toThrow('Project response scope mismatch')
  })
})
