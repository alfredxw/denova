import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  applyProjectFileOperations,
  resolveProjectFileTree,
  type ProjectFileEntryType,
  type ProjectFileOperation,
  type ProjectFileTreeResolveResult,
  type ProjectFileTreeResolveTarget,
} from '@/lib/api-client/project-files'
import {
  buildProjectFileExplorerNodes,
  mergeProjectDirectories,
  type CachedProjectDirectory,
  type ProjectFileExplorerNode,
} from './model'

export type { ProjectFileExplorerNode } from './model'

const MAX_TARGETS_PER_REQUEST = 256
interface ProjectExplorerOptions {
  projectId: string
  expandedPaths: readonly string[]
  selectedPath: string | null
}

export interface ProjectExplorerState {
  nodes: ProjectFileExplorerNode[]
  loading: boolean
  loadingPaths: ReadonlySet<string>
  error: string | null
  loadDirectory: (path: string) => Promise<void>
  loadMore: (path: string) => Promise<void>
  refresh: () => Promise<void>
  createItem: (path: string, type: ProjectFileEntryType) => Promise<void>
  deleteItem: (path: string) => Promise<void>
  renameItem: (path: string, newName: string) => Promise<string>
  copyItem: (from: string, to: string) => Promise<void>
  moveItem: (from: string, to: string) => Promise<void>
}

interface ResolveOptions {
  appendPath?: string
  surfaceErrors?: boolean
  recursive?: boolean
}

/**
 * Owns a normalized directory cache. Tree branches are resolved in batches and
 * mutations invalidate only their affected parents, keeping large projects
 * responsive without coupling explorer state to editor drafts.
 */
export function useProjectExplorer({
  projectId,
  expandedPaths,
  selectedPath,
}: ProjectExplorerOptions): ProjectExplorerState {
  const [directories, setDirectories] = useState<ReadonlyMap<string, CachedProjectDirectory>>(() => new Map())
  const directoriesRef = useRef(directories)
  directoriesRef.current = directories
  const [loadingPaths, setLoadingPaths] = useState<ReadonlySet<string>>(() => new Set())
  const [error, setError] = useState<string | null>(null)
  const sourceVersionRef = useRef(0)
  const selectionHintRef = useRef(selectedPath)
  const requestVersionsRef = useRef(new Map<string, number>())
  const loadingCountsRef = useRef(new Map<string, number>())

  const resolveTargets = useCallback(async (
    targets: readonly ProjectFileTreeResolveTarget[],
    options: ResolveOptions = {},
  ): Promise<ProjectFileTreeResolveResult[]> => {
    const uniqueTargets = deduplicateTargets(targets)
    if (uniqueTargets.length === 0) return []
    const sourceVersion = sourceVersionRef.current
    const chunks = chunkTargets(uniqueTargets)
    const loading = uniqueTargets.map((target) => target.path)
    const requestVersions = new Map<string, number>()
    for (const path of loading) {
      const version = (requestVersionsRef.current.get(path) ?? 0) + 1
      requestVersionsRef.current.set(path, version)
      requestVersions.set(path, version)
      loadingCountsRef.current.set(path, (loadingCountsRef.current.get(path) ?? 0) + 1)
    }
    setLoadingPaths(new Set(loadingCountsRef.current.keys()))
    try {
      const responses = await Promise.all(chunks.map((chunk) => resolveProjectFileTree(projectId, {
        targets: chunk,
        include_ignored: true,
        recursive: options.recursive === true,
      })))
      if (sourceVersionRef.current !== sourceVersion) return []
      const results = responses.flatMap((response) => response.results).filter((result) => (
        requestVersionsRef.current.get(result.path) === requestVersions.get(result.path)
      ))
      setDirectories((current) => {
        const next = mergeProjectDirectories(current, results, options.appendPath)
        directoriesRef.current = next
        return next
      })
      const failed = results.find((result) => !result.ok && result.code !== 'cursor_stale')
      if (options.surfaceErrors !== false) setError(failed?.error ?? null)
      return results
    } catch (cause) {
      if (sourceVersionRef.current === sourceVersion) {
        setError(cause instanceof Error ? cause.message : String(cause))
      }
      throw cause
    } finally {
      if (sourceVersionRef.current === sourceVersion) {
        for (const path of loading) {
          const remaining = (loadingCountsRef.current.get(path) ?? 1) - 1
          if (remaining > 0) loadingCountsRef.current.set(path, remaining)
          else loadingCountsRef.current.delete(path)
        }
        setLoadingPaths(new Set(loadingCountsRef.current.keys()))
      }
    }
  }, [projectId])

  useEffect(() => {
    sourceVersionRef.current += 1
    selectionHintRef.current = selectedPath
    requestVersionsRef.current.clear()
    loadingCountsRef.current.clear()
    const emptyDirectories = new Map<string, CachedProjectDirectory>()
    directoriesRef.current = emptyDirectories
    setDirectories(emptyDirectories)
    setLoadingPaths(new Set())
    setError(null)
    const hints = bootstrapTargets(expandedPaths, selectedPath).filter((target) => target.path !== '')
    void resolveTargets([{ path: '' }], { surfaceErrors: false, recursive: true })
      .then(async (results) => {
        const root = results.find((result) => result.path === '')
        if (root && !root.ok) setError(root.error ?? null)
        if (!root?.ok || hints.length === 0) return
        const resolvedPaths = new Set(
          results.flatMap((result) => result.ok ? result.directories.map((directory) => directory.path) : []),
        )
        const unresolvedHints = hints.filter((target) => !resolvedPaths.has(target.path))
        if (unresolvedHints.length === 0) return
        console.info('[features/project-explorer/use-project-explorer.ts] recursive bootstrap reached its boundary; resolving restored branches', {
          projectId,
          paths: unresolvedHints.map((target) => target.path),
        })
        await resolveTargets(unresolvedHints, { surfaceErrors: false, recursive: true })
      })
      .catch((cause) => {
        console.error('[features/project-explorer/use-project-explorer.ts] loading project tree failed', {
          projectId,
          cause,
        })
      })
  // Expanded paths and selection are bootstrap hints, not live query inputs.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, resolveTargets])

  useEffect(() => {
    if (selectionHintRef.current === selectedPath) return
    selectionHintRef.current = selectedPath
    const targets = ancestorDirectories(selectedPath)
      .filter((path) => !directoriesRef.current.has(path))
      .map((path) => ({ path }))
    if (targets.length === 0) return
    void resolveTargets(targets, { surfaceErrors: false, recursive: true }).catch((cause) => {
      console.error('[features/project-explorer/use-project-explorer.ts] resolving selected file ancestors failed', {
        projectId,
        selectedPath,
        cause,
      })
    })
  }, [projectId, resolveTargets, selectedPath])

  const loadDirectory = useCallback(async (path: string) => {
    if (directoriesRef.current.has(path)) return
    const results = await resolveTargets([{ path }], { recursive: true })
    throwForFailedTarget(results, path)
  }, [resolveTargets])

  const refreshDirectories = useCallback(async (paths: readonly string[]) => {
    const targets = [...new Set(paths)].map((path) => ({ path }))
    const results = await resolveTargets(targets)
    const failedPaths = new Set(results.filter((result) => !result.ok).map((result) => result.path))
    if (failedPaths.size > 0) {
      setDirectories((current) => {
        const next = removeDirectoryBranches(current, [...failedPaths])
        directoriesRef.current = next
        return next
      })
    }
    const firstFailure = results.find((result) => !result.ok)
    if (firstFailure) throw new Error(firstFailure.error || 'Project directory refresh failed')
  }, [resolveTargets])

  const loadMore = useCallback(async (path: string) => {
    const continuation = directoriesRef.current.get(path)?.continuation
    if (!continuation) return
    const results = await resolveTargets([{ path, cursor: continuation }], { appendPath: path, recursive: true })
    const result = results.find((item) => item.path === path)
    if (result?.code === 'cursor_stale') {
      await refreshDirectories([path])
      return
    }
    throwForFailedTarget(results, path)
  }, [refreshDirectories, resolveTargets])

  const refresh = useCallback(async () => {
    const paths = ['', ...directoriesRef.current.keys()]
    const results = await resolveTargets(
      [...new Set(paths)].map((path) => ({ path })),
      { surfaceErrors: false },
    )
    const failedBranches = results
      .filter((result) => result.path !== '' && !result.ok)
      .map((result) => result.path)
    if (failedBranches.length > 0) {
      // External writers may remove a loaded branch between refreshes. Its
      // authoritative parent remains usable, so evict the stale cache branch
      // without turning a successful project refresh into an operation error.
      setDirectories((current) => {
        const next = removeDirectoryBranches(current, failedBranches)
        directoriesRef.current = next
        return next
      })
    }
    const root = results.find((result) => result.path === '')
    setError(root && !root.ok ? root.error ?? null : null)
    throwForFailedTarget(results, '')
  }, [resolveTargets])

  const applyOperation = useCallback(async (
    operation: ProjectFileOperation,
    affectedParents: readonly string[],
    evictedBranches: readonly string[] = [],
  ) => {
    const [result] = await applyProjectFileOperations(projectId, [operation])
    if (!result?.ok) throw new Error(result?.error || 'Project file operation failed')
    if (evictedBranches.length > 0) {
      setDirectories((current) => {
        const next = removeDirectoryBranches(current, evictedBranches)
        directoriesRef.current = next
        return next
      })
    }
    await refreshDirectories(affectedParents)
    return result.path || operation.path
  }, [projectId, refreshDirectories])

  const createItem = useCallback(async (path: string, type: ProjectFileEntryType) => {
    const hierarchy = directoryHierarchy(parentPath(path))
    const refreshParent = hierarchy.findLast((candidate) => directoriesRef.current.has(candidate)) ?? ''
    await applyOperation({ kind: 'create', path, type }, [refreshParent])
    const unresolvedParents = hierarchy.filter((candidate) => (
      candidate !== refreshParent && !directoriesRef.current.has(candidate)
    ))
    if (unresolvedParents.length > 0) {
      const results = await resolveTargets(
        unresolvedParents.map((candidate) => ({ path: candidate })),
        { recursive: true },
      )
      const failure = results.find((result) => !result.ok)
      if (failure) throw new Error(failure.error || 'Created project directory could not be resolved')
    }
  }, [applyOperation, resolveTargets])
  const deleteItem = useCallback(async (path: string) => {
    await applyOperation({ kind: 'delete', path }, [parentPath(path)], [path])
  }, [applyOperation])
  const renameItem = useCallback((path: string, newName: string) => (
    applyOperation({ kind: 'rename', path, new_name: newName }, [parentPath(path)], [path])
  ), [applyOperation])
  const copyItem = useCallback(async (from: string, to: string) => {
    await applyOperation({ kind: 'copy', path: from, to }, [parentPath(to)])
  }, [applyOperation])
  const moveItem = useCallback(async (from: string, to: string) => {
    await applyOperation({ kind: 'move', path: from, to }, [parentPath(from), parentPath(to)], [from])
  }, [applyOperation])

  const nodes = useMemo(
    () => buildProjectFileExplorerNodes('', directories, loadingPaths),
    [directories, loadingPaths],
  )
  return {
    nodes,
    loading: loadingPaths.has('') && !directories.has(''),
    loadingPaths,
    error,
    loadDirectory,
    loadMore,
    refresh,
    createItem,
    deleteItem,
    renameItem,
    copyItem,
    moveItem,
  }
}

function bootstrapTargets(expandedPaths: readonly string[], selectedPath: string | null): ProjectFileTreeResolveTarget[] {
  const paths = ['', ...expandedPaths, ...ancestorDirectories(selectedPath)]
  return [...new Set(paths)].map((path) => ({ path }))
}

function ancestorDirectories(path: string | null): string[] {
  if (!path) return []
  const components = path.split('/').filter(Boolean)
  const ancestors: string[] = []
  for (let index = 1; index < components.length; index += 1) {
    ancestors.push(components.slice(0, index).join('/'))
  }
  return ancestors
}

function directoryHierarchy(path: string): string[] {
  const components = path.split('/').filter(Boolean)
  return ['', ...components.map((_, index) => components.slice(0, index + 1).join('/'))]
}

function deduplicateTargets(targets: readonly ProjectFileTreeResolveTarget[]) {
  const unique = new Map<string, ProjectFileTreeResolveTarget>()
  for (const target of targets) {
    unique.set(target.path, target)
  }
  return [...unique.values()]
}

function chunkTargets(targets: readonly ProjectFileTreeResolveTarget[]) {
  const chunks: ProjectFileTreeResolveTarget[][] = []
  for (let index = 0; index < targets.length; index += MAX_TARGETS_PER_REQUEST) {
    chunks.push(targets.slice(index, index + MAX_TARGETS_PER_REQUEST))
  }
  return chunks
}

function removeDirectoryBranches(current: ReadonlyMap<string, CachedProjectDirectory>, prefixes: readonly string[]) {
  const next = new Map(current)
  for (const path of next.keys()) {
    if (prefixes.some((prefix) => path === prefix || path.startsWith(`${prefix}/`))) next.delete(path)
  }
  return next
}

function throwForFailedTarget(results: readonly ProjectFileTreeResolveResult[], path: string) {
  const failed = results.find((result) => result.path === path && !result.ok)
  if (failed) throw new Error(failed.error || 'Project directory resolution failed')
}

function parentPath(path: string) {
  const separator = path.lastIndexOf('/')
  return separator < 0 ? '' : path.slice(0, separator)
}
