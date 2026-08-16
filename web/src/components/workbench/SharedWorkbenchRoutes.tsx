import { lazy, memo } from 'react'
import type { ComponentProps } from 'react'
import type { ResourceTarget } from '@/lib/api'
import { WorkbenchRouteLayer } from './WorkbenchRouteHost'
import type { WorkbenchRouteId, WorkbenchRoutePresentation } from './WorkbenchRouteHost'

const HomeView = memo(lazy(() => import('@/components/Home/HomeView').then((module) => ({ default: module.HomeView }))))
const AgentsView = memo(lazy(() => import('@/features/agents/AgentsView').then((module) => ({ default: module.AgentsView }))))
const AutomationsView = memo(lazy(() => import('@/features/automations/AutomationsView').then((module) => ({ default: module.AutomationsView }))))
const SkillsView = memo(lazy(() => import('@/features/skills/SkillsView').then((module) => ({ default: module.SkillsView }))))
const SettingsView = memo(lazy(() => import('@/features/settings/SettingsView').then((module) => ({ default: module.SettingsView }))))
const TrajectoryPage = memo(lazy(() => import('@/features/trajectory/TrajectoryPage').then((module) => ({ default: module.TrajectoryPage }))))

interface SharedWorkbenchRoutesProps {
  route: WorkbenchRouteId
  isMounted: WorkbenchRoutePresentation['isMounted']
  loadingLabel: string
  home: ComponentProps<typeof HomeView>
  automations: ComponentProps<typeof AutomationsView>
  resourceTarget: ResourceTarget
  onReturnToContentMode: () => void
  onCloseSettings: () => void
}

/** Renders the full-workbench routes shared by Writing and Game modes. */
export function SharedWorkbenchRoutes({
  route,
  isMounted,
  loadingLabel,
  home,
  automations,
  resourceTarget,
  onReturnToContentMode,
  onCloseSettings,
}: SharedWorkbenchRoutesProps) {
  return (
    <>
      {isMounted('books') && (
        <WorkbenchRouteLayer visible={route === 'books'} loadingLabel={loadingLabel}>
          <HomeView {...home} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('skills') && (
        <WorkbenchRouteLayer visible={route === 'skills'} loadingLabel={loadingLabel}>
          <SkillsView target={resourceTarget} onClose={onReturnToContentMode} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('agents') && (
        <WorkbenchRouteLayer visible={route === 'agents'} loadingLabel={loadingLabel}>
          <AgentsView target={resourceTarget} onClose={onReturnToContentMode} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('automations') && (
        <WorkbenchRouteLayer visible={route === 'automations'} loadingLabel={loadingLabel}>
          <AutomationsView {...automations} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('trajectory') && (
        <WorkbenchRouteLayer visible={route === 'trajectory'} loadingLabel={loadingLabel}>
          <TrajectoryPage onClose={onReturnToContentMode} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('settings') && (
        <WorkbenchRouteLayer visible={route === 'settings'} loadingLabel={loadingLabel}>
          <SettingsView onClose={onCloseSettings} />
        </WorkbenchRouteLayer>
      )}
    </>
  )
}
