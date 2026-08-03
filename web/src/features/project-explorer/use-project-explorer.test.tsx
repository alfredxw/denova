import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { useProjectExplorer } from './use-project-explorer'

describe('useProjectExplorer', () => {
  it('batches bootstrap resolution and refreshes only mutation parents', async () => {
    const resolvedTargets: string[][] = []
    const includeIgnoredValues: unknown[] = []
    server.use(
      http.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        const body = await request.json() as { targets: Array<{ path: string }>; include_ignored?: boolean }
        resolvedTargets.push(body.targets.map((target) => target.path))
        includeIgnoredValues.push(body.include_ignored)
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
    const { result } = renderHook(() => useProjectExplorer({
      projectId: 'project-one',
      expandedPaths: ['a'],
      selectedPath: 'b/current.ts',
    }))

    await waitFor(() => expect(result.current.loading).toBe(false))
    await act(() => result.current.createItem('a/new.ts', 'file'))

    expect(resolvedTargets).toEqual([['', 'a', 'b'], ['a']])
    expect(includeIgnoredValues).toEqual([true, true])
  })

  it('evicts a stale loaded branch without failing an otherwise successful refresh', async () => {
    let branchRemoved = false
    server.use(
      http.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        const body = await request.json() as { targets: Array<{ path: string }> }
        return HttpResponse.json({
          project_id: 'project-one',
          results: body.targets.map((target) => {
            if (branchRemoved && target.path === 'drafts') {
              return { path: target.path, ok: false, code: 'not_found', error: 'Directory no longer exists' }
            }
            return {
              path: target.path,
              ok: true,
              directories: [{
                path: target.path,
                revision: branchRemoved ? 'revision-2' : 'revision-1',
                entries: target.path === ''
                  ? [{ name: 'drafts', path: 'drafts', type: 'dir' }]
                  : [{ name: 'old.md', path: 'drafts/old.md', type: 'file' }],
                children_state: 'complete',
              }],
            }
          }),
        })
      }),
    )
    const { result } = renderHook(() => useProjectExplorer({
      projectId: 'project-one',
      expandedPaths: ['drafts'],
      selectedPath: null,
    }))

    await waitFor(() => expect(result.current.nodes[0]?.loaded).toBe(true))
    branchRemoved = true
    await act(() => result.current.refresh())

    await waitFor(() => expect(result.current.nodes[0]?.loaded).toBe(false))
    expect(result.current.error).toBeNull()
  })
})
