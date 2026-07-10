import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ExplorerProps } from './types'
import { buildStateTree, findFirstSelectable, findNode, getDefaultExpanded } from './build-tree'

export function useExplorerSelection(value: ExplorerProps['value']) {
  const { t } = useTranslation()
  const tree = useMemo(() => buildStateTree(value, t), [t, value])

  const [selectedId, setSelectedId] = useState('')
  const [expandedIds, setExpandedIds] = useState<Set<string>>(() => getDefaultExpanded(tree))

  // Ensure selectedId is valid
  useEffect(() => {
    if (selectedId && findNode(tree, selectedId)) return
    const first = findFirstSelectable(tree)
    setSelectedId(first?.id || '')
  }, [tree, selectedId])

  const selectedNode = useMemo(
    () => (selectedId ? findNode(tree, selectedId) : null),
    [tree, selectedId],
  )

  const toggleExpanded = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const expandAncestors = useCallback((ids: string[]) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      for (const id of ids) next.add(id)
      return next
    })
  }, [])

  const selectNode = useCallback((id: string) => {
    setSelectedId(id)
  }, [])

  const remapNodeId = useCallback((previousId: string, nextId: string) => {
    if (previousId === nextId) return
    setSelectedId((current) => current === previousId ? nextId : current)
    setExpandedIds((current) => {
      if (!current.has(previousId)) return current
      const next = new Set(current)
      next.delete(previousId)
      next.add(nextId)
      return next
    })
  }, [])

  return {
    tree,
    selectedId,
    selectedNode,
    expandedIds,
    toggleExpanded,
    expandAncestors,
    selectNode,
    remapNodeId,
  }
}
