import { memo, Suspense, useDeferredValue, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { LoadingState } from '@/components/common/LoadingState'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import type { WorkbenchPresentedLayout } from './WorkbenchShell'

export type WorkbenchRouteId =
  | 'settings'
  | 'skills'
  | 'agents'
  | 'automations'
  | 'agentchat'
  | 'trajectory'
  | 'books'
  | 'interactive'
  | 'versions'
  | 'ide-lore'
  | 'ide-teller'
  | 'ide-writing'

const PRESENTED_LAYOUT_BY_ROUTE = {
  settings: 'full',
  skills: 'full',
  agents: 'full',
  automations: 'full',
  agentchat: 'full',
  trajectory: 'full',
  books: 'full',
  interactive: 'interactive',
  versions: 'full',
  'ide-lore': 'full',
  'ide-teller': 'full',
  'ide-writing': 'writing',
} satisfies Record<WorkbenchRouteId, WorkbenchPresentedLayout>

interface WorkbenchRouteSelection {
  mode: WorkspaceMode
  rightPanel: RightPanel
  settingsOpen: boolean
}

export interface WorkbenchRoutePresentation {
  route: WorkbenchRouteId
  rightPanel: RightPanel
  layout: WorkbenchPresentedLayout
  isMounted: (route: WorkbenchRouteId) => boolean
}

/**
 * Owns route selection, deferred presentation, and retained mounting. Navigation
 * state remains in workspace-store; the host only projects it for heavy routes.
 */
export function useWorkbenchRouteHost({
  mode,
  rightPanel,
  settingsOpen,
}: WorkbenchRouteSelection): WorkbenchRoutePresentation {
  const selectedRoute = selectWorkbenchRoute({ mode, rightPanel, settingsOpen })
  const selectedPresentation = useMemo(() => ({
    route: selectedRoute,
    rightPanel,
  }), [rightPanel, selectedRoute])
  // Menu state updates synchronously in workspace-store. Only expensive route
  // presentation is deferred so navigation feedback cannot be held hostage.
  // Route and panel form one snapshot to prevent a mixed transition frame.
  const presented = useDeferredValue(selectedPresentation)
  const presentedRoute = presented.route
  const [mountedRoutes, setMountedRoutes] = useState<ReadonlySet<WorkbenchRouteId>>(
    () => new Set(['ide-writing', selectedRoute]),
  )
  const renderedRoutes = useMemo(() => {
    if (mountedRoutes.has(presentedRoute)) return mountedRoutes
    return new Set([...mountedRoutes, presentedRoute])
  }, [mountedRoutes, presentedRoute])

  useEffect(() => {
    setMountedRoutes((current) => {
      if (current.has(selectedRoute)) return current
      const next = new Set(current)
      next.add(selectedRoute)
      return next
    })
  }, [selectedRoute])

  return useMemo(() => ({
    route: presentedRoute,
    rightPanel: presented.rightPanel,
    layout: PRESENTED_LAYOUT_BY_ROUTE[presentedRoute],
    isMounted: (route: WorkbenchRouteId) => renderedRoutes.has(route),
  }), [presented, presentedRoute, renderedRoutes])
}

export function selectWorkbenchRoute({
  mode,
  rightPanel,
  settingsOpen,
}: WorkbenchRouteSelection): WorkbenchRouteId {
  if (settingsOpen) return 'settings'
  switch (mode) {
    case 'skills':
    case 'agents':
    case 'automations':
    case 'agentchat':
    case 'trajectory':
    case 'books':
    case 'interactive':
      return mode
    case 'ide':
      if (rightPanel === 'versions') return 'versions'
      if (rightPanel === 'lore' || rightPanel === 'teller') return `ide-${rightPanel}`
      return 'ide-writing'
  }
}

interface WorkbenchRouteLayerProps {
  visible: boolean
  loadingLabel: string
  children: ReactNode
  /** Hidden retained routes must refresh immediately when their resource owner changes. */
  retentionKey?: string
}

export function WorkbenchRouteLayer({ visible, loadingLabel, children }: WorkbenchRouteLayerProps) {
  return (
    <section hidden={!visible} aria-hidden={!visible} className="absolute inset-0 flex min-h-0 flex-col">
      <Suspense fallback={<LoadingState label={loadingLabel} className="h-full min-h-0" />}>
        {children}
      </Suspense>
    </section>
  )
}

/**
 * Keeps route state and effects alive while preventing unrelated foreground
 * renders from reconciling a large hidden subtree.
 */
export const RetainedWorkbenchRouteLayer = memo(WorkbenchRouteLayer, (previous, next) => (
  !previous.visible
  && !next.visible
  && previous.retentionKey === next.retentionKey
  && previous.loadingLabel === next.loadingLabel
))
