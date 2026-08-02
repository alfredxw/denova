import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import {
  applyProjectFileOperations,
  listProjectDirectory,
  projectFileAssetURL,
  readProjectFile,
  saveProjectFile,
} from './api'

describe('project files API', () => {
  it('keeps project identity and paths encoded at the transport boundary', async () => {
    const requests: Array<{ method: string; path: string; body?: unknown }> = []
    server.use(
      http.get('/api/projects/project%20one/files', ({ request }) => {
        const url = new URL(request.url)
        requests.push({ method: request.method, path: `${url.pathname}${url.search}` })
        return HttpResponse.json({ project_id: 'project one', path: 'src', entries: null })
      }),
      http.get('/api/projects/project%20one/files/file', ({ request }) => {
        const url = new URL(request.url)
        requests.push({ method: request.method, path: `${url.pathname}${url.search}` })
        return HttpResponse.json({
          project_id: 'project one',
          path: 'src/main file.ts',
          content: 'before',
          revision: 'r1',
          kind: 'text',
          mime_type: 'text/typescript',
          size: 6,
          editable: true,
        })
      }),
      http.put('/api/projects/project%20one/files/file', async ({ request }) => {
        requests.push({ method: request.method, path: new URL(request.url).pathname, body: await request.json() })
        return HttpResponse.json({ project_id: 'project one', path: 'src/main file.ts', revision: 'r2', changed: true })
      }),
    )

    await expect(listProjectDirectory('project one', 'src', true)).resolves.toMatchObject({ entries: [] })
    const document = await readProjectFile('project one', 'src/main file.ts')
    await saveProjectFile('project one', document.path, 'after', document.revision)

    expect(requests).toEqual([
      { method: 'GET', path: '/api/projects/project%20one/files?path=src&include_ignored=true' },
      { method: 'GET', path: '/api/projects/project%20one/files/file?path=src%2Fmain+file.ts' },
      {
        method: 'PUT',
        path: '/api/projects/project%20one/files/file',
        body: { path: 'src/main file.ts', content: 'after', base_revision: 'r1' },
      },
    ])
    expect(projectFileAssetURL('project one', 'art/cover image.png', 'sha256:1')).toBe(
      '/api/projects/project%20one/files/asset?path=art%2Fcover+image.png&revision=sha256%3A1',
    )
  })

  it('preserves per-item partial operation results', async () => {
    server.use(
      http.post('/api/projects/p1/files/operations', async ({ request }) => {
        expect(await request.json()).toEqual({
          operations: [
            { id: 'one', kind: 'create', path: 'one.txt', type: 'file' },
            { id: 'two', kind: 'delete', path: 'missing.txt' },
          ],
        })
        return HttpResponse.json({
          results: [
            { id: 'one', kind: 'create', ok: true, path: 'one.txt' },
            { id: 'two', kind: 'delete', ok: false, path: 'missing.txt', code: 'not_found', error: 'Missing' },
          ],
        })
      }),
    )

    await expect(applyProjectFileOperations('p1', [
      { id: 'one', kind: 'create', path: 'one.txt', type: 'file' },
      { id: 'two', kind: 'delete', path: 'missing.txt' },
    ])).resolves.toEqual([
      { id: 'one', kind: 'create', ok: true, path: 'one.txt' },
      { id: 'two', kind: 'delete', ok: false, path: 'missing.txt', code: 'not_found', error: 'Missing' },
    ])
  })
})
