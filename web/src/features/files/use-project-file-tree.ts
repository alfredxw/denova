import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { FileNode } from '@/hooks/useWorkspace'
import {
  applyProjectFileOperations,
  listProjectDirectory,
  type ProjectDirectory,
  type ProjectFileEntryType,
  type ProjectFileOperation,
} from './api'

interface ProjectFileTreeOptions {
  projectId: string
  includeIgnored: boolean
}

interface ProjectFileTreeState {
  nodes: FileNode[]
  loading: boolean
  loadingPaths: ReadonlySet<string>
  error: string | null
  loadDirectory: (path: string) => Promise<void>
  refresh: () => Promise<void>
  createItem: (path: string, type: ProjectFileEntryType) => Promise<void>
  deleteItem: (path: string) => Promise<void>
  renameItem: (path: string, newName: string) => Promise<string>
  copyItem: (from: string, to: string) => Promise<void>
  moveItem: (from: string, to: string) => Promise<void>
}

/** Owns the lazy directory cache and project-scoped mutations for one Files tab. */
export function useProjectFileTree({ projectId, includeIgnored }: ProjectFileTreeOptions): ProjectFileTreeState {
  const [directories, setDirectories] = useState<ReadonlyMap<string, ProjectDirectory>>(() => new Map())
  const directoriesRef = useRef(directories)
  directoriesRef.current = directories
  const [loadingPaths, setLoadingPaths] = useState<ReadonlySet<string>>(() => new Set())
  const [error, setError] = useState<string | null>(null)
  const requestVersionsRef = useRef(new Map<string, number>())
  const sourceVersionRef = useRef(0)

  const loadDirectorySnapshot = useCallback(async (path: string, force = false) => {
    if (!force && directoriesRef.current.has(path)) return
    const sourceVersion = sourceVersionRef.current
    const requestVersion = (requestVersionsRef.current.get(path) ?? 0) + 1
    requestVersionsRef.current.set(path, requestVersion)
    setLoadingPaths((current) => new Set(current).add(path))
    try {
      const directory = await listProjectDirectory(projectId, path, includeIgnored)
      if (sourceVersionRef.current !== sourceVersion || requestVersionsRef.current.get(path) !== requestVersion) return
      setDirectories((current) => {
        const next = new Map(current)
        next.set(path, directory)
        return next
      })
      setError(null)
    } catch (cause) {
      if (sourceVersionRef.current === sourceVersion && requestVersionsRef.current.get(path) === requestVersion) {
        setError(cause instanceof Error ? cause.message : String(cause))
      }
      throw cause
    } finally {
      if (sourceVersionRef.current === sourceVersion && requestVersionsRef.current.get(path) === requestVersion) {
        setLoadingPaths((current) => {
          const next = new Set(current)
          next.delete(path)
          return next
        })
      }
    }
  }, [includeIgnored, projectId])

  useEffect(() => {
    sourceVersionRef.current += 1
    requestVersionsRef.current.clear()
    setDirectories(new Map())
    setLoadingPaths(new Set())
    setError(null)
    void loadDirectorySnapshot('', true).catch((cause) => {
      console.error('[features/files/use-project-file-tree.ts] loading project root failed', { projectId, cause })
    })
  }, [includeIgnored, loadDirectorySnapshot, projectId])

  const refresh = useCallback(async () => {
    const paths = [...directoriesRef.current.keys()].filter(Boolean)
    await loadDirectorySnapshot('', true)
    const results = await Promise.allSettled(paths.map((path) => loadDirectorySnapshot(path, true)))
    const unavailablePaths = paths.filter((_, index) => results[index]?.status === 'rejected')
    if (unavailablePaths.length > 0) {
      setDirectories((current) => {
        const next = new Map(current)
        unavailablePaths.forEach((path) => next.delete(path))
        return next
      })
    }
    setError(null)
  }, [loadDirectorySnapshot])

  const applyOperation = useCallback(async (operation: ProjectFileOperation) => {
    const [result] = await applyProjectFileOperations(projectId, [operation])
    if (!result?.ok) throw new Error(result?.error || 'Project file operation failed')
    await refresh()
    return result.path || operation.path
  }, [projectId, refresh])

  const createItem = useCallback(async (path: string, type: ProjectFileEntryType) => {
    await applyOperation({ kind: 'create', path, type })
  }, [applyOperation])
  const deleteItem = useCallback(async (path: string) => {
    await applyOperation({ kind: 'delete', path })
  }, [applyOperation])
  const renameItem = useCallback((path: string, newName: string) => (
    applyOperation({ kind: 'rename', path, new_name: newName })
  ), [applyOperation])
  const copyItem = useCallback(async (from: string, to: string) => {
    await applyOperation({ kind: 'copy', path: from, to })
  }, [applyOperation])
  const moveItem = useCallback(async (from: string, to: string) => {
    await applyOperation({ kind: 'move', path: from, to })
  }, [applyOperation])

  const nodes = useMemo(() => directoryNodes('', directories), [directories])
  return {
    nodes,
    loading: loadingPaths.has('') && !directories.has(''),
    loadingPaths,
    error,
    loadDirectory: loadDirectorySnapshot,
    refresh,
    createItem,
    deleteItem,
    renameItem,
    copyItem,
    moveItem,
  }
}

function directoryNodes(path: string, directories: ReadonlyMap<string, ProjectDirectory>): FileNode[] {
  return (directories.get(path)?.entries ?? []).map((entry) => ({
    name: entry.name,
    type: entry.type,
    children: entry.type === 'dir' && directories.has(entry.path)
      ? directoryNodes(entry.path, directories)
      : undefined,
  }))
}
