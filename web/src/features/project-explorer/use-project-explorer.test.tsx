import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { apiRoute, installApiMock, jsonResponse } from '@/test/api-mock'
import { TestQueryClientProvider } from '@/test/query-client'
import { useProjectExplorer } from './use-project-explorer'

let apiMock: ReturnType<typeof installApiMock>

describe('useProjectExplorer', () => {
  beforeEach(() => {
    apiMock = installApiMock(
      apiRoute.get('/api/projects/project-one/versions/status', () => jsonResponse({
        has_versions: false,
        clean: true,
        changes: [],
        auto: { timed_enabled: false, timed_interval_minutes: 10, retention: 100 },
      })),
    )
  })

  afterEach(() => vi.unstubAllGlobals())

  it('loads an ordinary nested tree in one recursive bootstrap request', async () => {
    const requestBodies: unknown[] = []
    apiMock.use(
      apiRoute.get('/api/projects/project-one/versions/status', () => jsonResponse({
        has_versions: true,
        clean: false,
        changes: [
          { path: 'a/nested/one.md', status: 'modified' },
          { path: 'unsupported.md', status: 'unsupported' },
        ],
        auto: { timed_enabled: false, timed_interval_minutes: 10, retention: 100 },
      })),
      apiRoute.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        requestBodies.push(await request.json())
        return jsonResponse({
          project_id: 'project-one',
          results: [{
            path: '',
            ok: true,
            directories: [
              {
                path: '',
                revision: 'root',
                entries: [{ name: 'a', path: 'a', type: 'dir' }],
                children_state: 'complete',
              },
              {
                path: 'a',
                revision: 'a',
                entries: [{ name: 'nested', path: 'a/nested', type: 'dir' }],
                children_state: 'complete',
              },
              {
                path: 'a/nested',
                revision: 'nested',
                entries: [{ name: 'one.md', path: 'a/nested/one.md', type: 'file' }],
                children_state: 'complete',
              },
            ],
          }],
        })
      }),
    )

    const { result } = renderHook(() => useProjectExplorer({
      projectId: 'project-one',
      expandedPaths: ['a', 'a/nested'],
      selectedPath: 'a/nested/one.md',
    }), { wrapper: TestQueryClientProvider })

    await waitFor(() => expect(result.current.nodes[0]?.children?.[0]?.children?.[0]?.path).toBe('a/nested/one.md'))
    await waitFor(() => expect(result.current.gitStatus).toEqual([{ path: 'a/nested/one.md', status: 'modified' }]))
    await act(() => result.current.loadDirectory('a/nested'))

    expect(requestBodies).toEqual([{
      targets: [{ path: '' }],
      include_ignored: true,
      recursive: true,
    }])
  })

  it('batches bootstrap resolution and refreshes only mutation parents', async () => {
    const resolvedTargets: string[][] = []
    const includeIgnoredValues: unknown[] = []
    apiMock.use(
      apiRoute.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        const body = await request.json() as { targets: Array<{ path: string }>; include_ignored?: boolean }
        resolvedTargets.push(body.targets.map((target) => target.path))
        includeIgnoredValues.push(body.include_ignored)
        return jsonResponse({
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
      apiRoute.post('/api/projects/project-one/files/operations', () => jsonResponse({
        project_id: 'project-one',
        results: [{ kind: 'create', ok: true, path: 'a/new.ts' }],
      })),
    )
    const { result } = renderHook(() => useProjectExplorer({
      projectId: 'project-one',
      expandedPaths: ['a'],
      selectedPath: 'b/current.ts',
    }), { wrapper: TestQueryClientProvider })

    await waitFor(() => expect(result.current.loading).toBe(false))
    await act(() => result.current.createItem('a/new.ts', 'file'))

    expect(resolvedTargets).toEqual([[''], ['a', 'b'], ['a']])
    expect(includeIgnoredValues).toEqual([true, true, true])
  })

  it('keeps loaded branches cached across file switches and resolves only missing ancestors', async () => {
    const resolvedTargets: string[][] = []
    apiMock.use(
      apiRoute.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        const body = await request.json() as { targets: Array<{ path: string }> }
        resolvedTargets.push(body.targets.map((target) => target.path))
        return jsonResponse({
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
    }), { initialProps: { selectedPath: 'a/one.md' }, wrapper: TestQueryClientProvider })

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(resolvedTargets).toEqual([[''], ['a']])

    rerender({ selectedPath: 'a/two.md' })
    await waitFor(() => expect(result.current.nodes[0]?.children?.[1]?.path).toBe('a/two.md'))
    expect(resolvedTargets).toEqual([[''], ['a']])

    rerender({ selectedPath: 'b/three.md' })
    await waitFor(() => expect(resolvedTargets).toEqual([[''], ['a'], ['b']]))
  })

  it('evicts a stale loaded branch without failing an otherwise successful refresh', async () => {
    let branchRemoved = false
    apiMock.use(
      apiRoute.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        const body = await request.json() as { targets: Array<{ path: string }> }
        return jsonResponse({
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
    }), { wrapper: TestQueryClientProvider })

    await waitFor(() => expect(result.current.nodes[0]?.loaded).toBe(true))
    branchRemoved = true
    await act(() => result.current.refresh())

    await waitFor(() => expect(result.current.nodes[0]?.loaded).toBe(false))
    expect(result.current.error).toBeNull()
  })

  it('resolves intermediate folders created from one inline path', async () => {
    let created = false
    const resolvedTargets: string[][] = []
    apiMock.use(
      apiRoute.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        const body = await request.json() as { targets: Array<{ path: string }> }
        resolvedTargets.push(body.targets.map((target) => target.path))
        return jsonResponse({
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
      apiRoute.post('/api/projects/project-one/files/operations', () => {
        created = true
        return jsonResponse({
          project_id: 'project-one',
          results: [{ kind: 'create', ok: true, path: 'nested/deep/story.md' }],
        })
      }),
    )
    const { result } = renderHook(() => useProjectExplorer({
      projectId: 'project-one',
      expandedPaths: [],
      selectedPath: null,
    }), { wrapper: TestQueryClientProvider })

    await waitFor(() => expect(result.current.loading).toBe(false))
    await act(() => result.current.createItem('nested/deep/story.md', 'file'))

    await waitFor(() => expect(result.current.nodes[0]?.children?.[0]?.children?.[0]?.path).toBe('nested/deep/story.md'))
    expect(resolvedTargets).toEqual([[''], [''], ['nested', 'nested/deep']])
  })
})
