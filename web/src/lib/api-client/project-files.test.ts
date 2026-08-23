import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import {
  applyProjectFileOperations,
  projectFileAssetURL,
  readOptionalProjectFile,
  readProjectFile,
  revealProjectFile,
  resolveProjectFileTree,
  saveProjectFile,
} from './project-files'

describe('project files API', () => {
  it('keeps project identity and paths encoded at the transport boundary', async () => {
    const requests: Array<{ method: string; path: string; body?: unknown }> = []
    server.use(
      http.post('/api/projects/project%20one/files/resolve', async ({ request }) => {
        const url = new URL(request.url)
        requests.push({ method: request.method, path: `${url.pathname}${url.search}`, body: await request.json() })
        return HttpResponse.json({
          project_id: 'project one',
          results: [{ id: 'source', path: 'src', ok: true, directories: [{ path: 'src', revision: 'r1', entries: null, children_state: 'complete' }] }],
        })
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

    await expect(resolveProjectFileTree('project one', {
      targets: [{ id: 'source', path: 'src' }],
      include_ignored: true,
      follow_single_child_directories: true,
      recursive: true,
      entry_budget: 4096,
    })).resolves.toMatchObject({ results: [{ directories: [{ entries: [] }] }] })
    const document = await readProjectFile('project one', 'src/main file.ts')
    await saveProjectFile('project one', document.path, 'after', document.revision)

    expect(requests).toEqual([
      {
        method: 'POST',
        path: '/api/projects/project%20one/files/resolve',
        body: {
          targets: [{ id: 'source', path: 'src' }],
          include_ignored: true,
          follow_single_child_directories: true,
          recursive: true,
          entry_budget: 4096,
        },
      },
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
          project_id: 'p1',
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

  it('reveals only a project-relative path through the scoped endpoint', async () => {
    server.use(
      http.post('/api/projects/p1/files/reveal', async ({ request }) => {
        expect(await request.json()).toEqual({ path: 'chapters/one.md' })
        return HttpResponse.json({ project_id: 'p1', path: 'chapters/one.md' })
      }),
    )

    await expect(revealProjectFile('p1', 'chapters/one.md')).resolves.toBeUndefined()
  })

  it('treats an optional missing file as data and shares concurrent reads', async () => {
    let requests = 0
    server.use(
      http.get('/api/projects/project-one/files/file', ({ request }) => {
        const url = new URL(request.url)
        if (url.searchParams.get('optional') !== 'true') return new HttpResponse(null, { status: 500 })
        requests += 1
        return HttpResponse.json({
          project_id: 'project-one',
          path: 'setting/optional.json',
          found: false,
        })
      }),
    )

    await expect(Promise.all([
      readOptionalProjectFile('project-one', 'setting/optional.json'),
      readOptionalProjectFile('project-one', 'setting/optional.json'),
    ])).resolves.toEqual([null, null])
    expect(requests).toBe(1)
  })

  it('rejects responses from a different Project scope', async () => {
    server.use(
      http.get('/api/projects/project-one/files/file', () => HttpResponse.json({
        project_id: 'project-two',
        path: 'story.md',
        content: 'wrong project',
        revision: 'r1',
        kind: 'text',
        mime_type: 'text/markdown',
        size: 13,
        editable: true,
      })),
    )

    await expect(readProjectFile('project-one', 'story.md')).rejects.toThrow(
      'Project file response scope mismatch',
    )
  })
})
