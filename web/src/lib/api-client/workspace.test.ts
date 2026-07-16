import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { readFile, saveFile } from './workspace'

describe('workspace file API', () => {
  it('round-trips the canonical workspace identity from read to save', async () => {
    let saveBody: unknown
    server.use(
      http.get('/api/workspace/file', ({ request }) => {
        expect(new URL(request.url).searchParams.get('path')).toBe('chapters/ch01.md')
        return HttpResponse.json({
          workspace: '/canonical/books/demo',
          path: 'chapters/ch01.md',
          content: '正文',
          revision: 'sha256:read',
        })
      }),
      http.post('/api/workspace/file', async ({ request }) => {
        saveBody = await request.json()
        return HttpResponse.json({ path: 'chapters/ch01.md', message: 'ok', revision: 'sha256:saved' })
      }),
    )

    const document = await readFile('chapters/ch01.md')
    await saveFile(document.path, '修改后', document.revision || '', document.workspace)

    expect(saveBody).toEqual({
      workspace: '/canonical/books/demo',
      path: 'chapters/ch01.md',
      content: '修改后',
      base_revision: 'sha256:read',
    })
  })
})
