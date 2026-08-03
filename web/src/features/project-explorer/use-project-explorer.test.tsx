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

  it('keeps loaded branches cached across file switches and resolves only missing ancestors', async () => {
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
              revision: `revision-${target.path || 'root'}`,
              entries: target.path === ''
                ? [{ name: 'a', path: 'a', type: 'dir' }, { name: 'b', path: 'b', type: 'dir' }]
                : target.path === 'a'
                  ? [{ name: 'one.md', path: 'a/one.md', type: 'file' }, { name: 'two.md', path: 'a/two.md', type: 'file' }]
                  : [{ name: 'three.md', path: 'b/three.md', type: 'file' }],
              children_state: 'complete',
            }],
          })),
        })
      }),
    )
    const { rerender, result } = renderHook(({ selectedPath }: { selectedPath: string }) => useProjectExplorer({
      projectId: 'project-one',
      expandedPaths: ['a'],
      selectedPath,
    }), { initialProps: { selectedPath: 'a/one.md' } })

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(resolvedTargets).toEqual([['', 'a']])

    rerender({ selectedPath: 'a/two.md' })
    await waitFor(() => expect(result.current.nodes[0]?.children?.[1]?.path).toBe('a/two.md'))
    expect(resolvedTargets).toEqual([['', 'a']])

    rerender({ selectedPath: 'b/three.md' })
    await waitFor(() => expect(resolvedTargets).toEqual([['', 'a'], ['b']]))
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

  it('resolves intermediate folders created from one inline path', async () => {
    let created = false
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
              revision: `${created ? 'after' : 'before'}-${target.path || 'root'}`,
              entries: !created ? [] : target.path === ''
                ? [{ name: 'nested', path: 'nested', type: 'dir' }]
                : target.path === 'nested'
                  ? [{ name: 'deep', path: 'nested/deep', type: 'dir' }]
                  : [{ name: 'story.md', path: 'nested/deep/story.md', type: 'file' }],
              children_state: 'complete',
            }],
          })),
        })
      }),
      http.post('/api/projects/project-one/files/operations', () => {
        created = true
        return HttpResponse.json({
          results: [{ kind: 'create', ok: true, path: 'nested/deep/story.md' }],
        })
      }),
    )
    const { result } = renderHook(() => useProjectExplorer({
      projectId: 'project-one',
      expandedPaths: [],
      selectedPath: null,
    }))

    await waitFor(() => expect(result.current.loading).toBe(false))
    await act(() => result.current.createItem('nested/deep/story.md', 'file'))

    await waitFor(() => expect(result.current.nodes[0]?.children?.[0]?.children?.[0]?.path).toBe('nested/deep/story.md'))
    expect(resolvedTargets).toEqual([[''], [''], ['nested', 'nested/deep']])
  })
})
