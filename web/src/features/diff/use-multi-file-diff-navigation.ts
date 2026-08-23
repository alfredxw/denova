import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

interface MultiFileDiffNavigationOptions {
  identity: string
  paths: readonly string[]
  preferredPath?: string | null
}

/** Shared scroll, active-file, collapse, and navigator state for continuous multi-file Diff surfaces. */
export function useMultiFileDiffNavigation({ identity, paths, preferredPath }: MultiFileDiffNavigationOptions) {
  const [activePath, setActivePath] = useState('')
  const [collapsedPaths, setCollapsedPaths] = useState<ReadonlySet<string>>(() => new Set())
  const [navigatorVisible, setNavigatorVisible] = useState(true)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const fileSectionRefs = useRef(new Map<string, HTMLElement>())
  const scrollFrameRef = useRef<number | null>(null)
  const jumpFrameRef = useRef<number | null>(null)
  const pendingJumpPathRef = useRef('')

  useEffect(() => {
    setActivePath('')
    setCollapsedPaths(new Set())
    setNavigatorVisible(true)
    fileSectionRefs.current.clear()
  }, [identity])

  useEffect(() => {
    if (!paths.length) {
      setActivePath('')
      return
    }
    if (paths.includes(activePath)) return
    setActivePath(preferredPath && paths.includes(preferredPath) ? preferredPath : paths[0])
  }, [activePath, paths, preferredPath])

  useEffect(() => {
    const available = new Set(paths)
    setCollapsedPaths((current) => {
      const next = new Set([...current].filter((path) => available.has(path)))
      return next.size === current.size ? current : next
    })
  }, [paths])

  const preRenderPaths = useMemo(() => {
    const result = new Set<string>()
    const index = paths.indexOf(activePath)
    if (index < 0) return result
    if (index > 0) result.add(paths[index - 1])
    result.add(paths[index])
    if (index < paths.length - 1) result.add(paths[index + 1])
    return result
  }, [activePath, paths])
  const allDiffsCollapsed = paths.length > 0 && paths.every((path) => collapsedPaths.has(path))

  const registerFileSection = useCallback((path: string, node: HTMLElement | null) => {
    if (node) fileSectionRefs.current.set(path, node)
    else fileSectionRefs.current.delete(path)
  }, [])

  const toggleFile = useCallback((path: string) => {
    setActivePath(path)
    setCollapsedPaths((current) => {
      const next = new Set(current)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }, [])

  const toggleAllDiffs = useCallback(() => {
    setCollapsedPaths(allDiffsCollapsed ? new Set() : new Set(paths))
  }, [allDiffsCollapsed, paths])

  const cancelPendingJump = useCallback(() => {
    pendingJumpPathRef.current = ''
    if (jumpFrameRef.current !== null) {
      window.cancelAnimationFrame(jumpFrameRef.current)
      jumpFrameRef.current = null
    }
  }, [])

  const jumpToFile = useCallback((path: string) => {
    setActivePath(path)
    setCollapsedPaths((current) => {
      if (!current.has(path)) return current
      const next = new Set(current)
      next.delete(path)
      return next
    })
    pendingJumpPathRef.current = path
    if (jumpFrameRef.current !== null) window.cancelAnimationFrame(jumpFrameRef.current)
    jumpFrameRef.current = window.requestAnimationFrame(() => {
      jumpFrameRef.current = null
      const pendingPath = pendingJumpPathRef.current
      pendingJumpPathRef.current = ''
      fileSectionRefs.current.get(pendingPath)?.scrollIntoView({ behavior: 'auto', block: 'start', inline: 'nearest' })
    })
  }, [])

  const syncActivePathFromScroll = useCallback(() => {
    scrollFrameRef.current = null
    const scroll = scrollRef.current
    if (!scroll || paths.length === 0 || pendingJumpPathRef.current) return
    const activationLine = scroll.getBoundingClientRect().top + 48
    let nextPath = paths[0]
    for (const path of paths) {
      const section = fileSectionRefs.current.get(path)
      if (!section || section.getBoundingClientRect().top > activationLine) break
      nextPath = path
    }
    if (scroll.scrollHeight - scroll.scrollTop - scroll.clientHeight <= 2) nextPath = paths[paths.length - 1]
    setActivePath((current) => current === nextPath ? current : nextPath)
  }, [paths])

  const handleScroll = useCallback(() => {
    cancelPendingJump()
    if (scrollFrameRef.current !== null) return
    scrollFrameRef.current = window.requestAnimationFrame(syncActivePathFromScroll)
  }, [cancelPendingJump, syncActivePathFromScroll])

  useEffect(() => () => {
    if (scrollFrameRef.current !== null) window.cancelAnimationFrame(scrollFrameRef.current)
    cancelPendingJump()
  }, [cancelPendingJump])

  return {
    activePath,
    collapsedPaths,
    preRenderPaths,
    allDiffsCollapsed,
    navigatorVisible,
    setNavigatorVisible,
    scrollRef,
    registerFileSection,
    toggleFile,
    toggleAllDiffs,
    jumpToFile,
    handleScroll,
    cancelPendingJump,
  }
}

export type MultiFileDiffNavigation = ReturnType<typeof useMultiFileDiffNavigation>
