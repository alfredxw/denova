import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { useProjectFileExplorer } from './use-project-file-explorer'

describe('useProjectFileExplorer', () => {
  it('batches bootstrap resolution and refreshes only mutation parents', async () => {
    const resolvedTargets: string[][] = []
    server.use(
      http.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        const body = await request.json() as { targets: Array<{ path: string }> }
        resolvedTargets.push(body.targets.map((target) => target.path))
        return HttpResponse.json({
          project_id: 'project-one',
          results: body.targets.map((target) => ({
            path: target.path,
            ok: true,
            directories: [{
              path: target.path,
              revision: `revision-${resolvedTargets.length}-${target.path}`,
              entries: target.path === ''
                ? [{ name: 'a', path: 'a', type: 'dir' }, { name: 'b', path: 'b', type: 'dir' }]
                : [],
              children_state: 'complete',
            }],
          })),
        })
      }),
      http.post('/api/projects/project-one/files/operations', () => HttpResponse.json({
        results: [{ kind: 'create', ok: true, path: 'a/new.ts' }],
      })),
    )
    const { result } = renderHook(() => useProjectFileExplorer({
      projectId: 'project-one',
      includeIgnored: false,
      expandedPaths: ['a'],
      selectedPath: 'b/current.ts',
    }))

    await waitFor(() => expect(result.current.loading).toBe(false))
    await act(() => result.current.createItem('a/new.ts', 'file'))

    expect(resolvedTargets).toEqual([['', 'a', 'b'], ['a']])
  })
})
