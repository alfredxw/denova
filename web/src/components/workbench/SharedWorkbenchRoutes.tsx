import { memo } from 'react'
import loadable from '@loadable/component'
import type { ComponentProps } from 'react'
import type { ResourceTarget } from '@/lib/api'
import { RetainedWorkbenchRouteLayer as WorkbenchRouteLayer, workbenchLoadingOptions } from './WorkbenchRouteHost'
import type { WorkbenchRouteId, WorkbenchRoutePresentation } from './WorkbenchRouteHost'
import type { ToolNavigationIntent } from '@/components/Chat/tool-navigation'

const HomeView = memo(loadable(() => import('@/components/Home/HomeView').then((module) => ({ default: module.HomeView })), workbenchLoadingOptions))
const ResourceCenterView = memo(loadable(() => import('@/features/market/ResourceCenterView').then((module) => ({ default: module.ResourceCenterView })), workbenchLoadingOptions))
const AgentsView = memo(loadable(() => import('@/features/agents/AgentsView').then((module) => ({ default: module.AgentsView })), workbenchLoadingOptions))
const AutomationsView = memo(loadable(() => import('@/features/automations/AutomationsView').then((module) => ({ default: module.AutomationsView })), workbenchLoadingOptions))
const SkillsView = memo(loadable(() => import('@/features/skills/SkillsView').then((module) => ({ default: module.SkillsView })), workbenchLoadingOptions))
const SettingsView = memo(loadable(() => import('@/features/settings/SettingsView').then((module) => ({ default: module.SettingsView })), workbenchLoadingOptions))
const TrajectoryPage = memo(loadable(() => import('@/features/trajectory/TrajectoryPage').then((module) => ({ default: module.TrajectoryPage })), workbenchLoadingOptions))

const StatisticsPage = memo(loadable(() => import('@/features/statistics/StatisticsPage').then((module) => ({ default: module.StatisticsPage })), workbenchLoadingOptions))

interface SharedWorkbenchRoutesProps {
  route: WorkbenchRouteId
  isMounted: WorkbenchRoutePresentation['isMounted']
  loadingLabel: string
  home: ComponentProps<typeof HomeView>
  automations: ComponentProps<typeof AutomationsView>
  resourceTarget: ResourceTarget
  toolNavigationIntent: ToolNavigationIntent | null
}

/** Renders the full-workbench routes shared by Writing and Game modes. */
export function SharedWorkbenchRoutes({
  route,
  isMounted,
  loadingLabel,
  home,
  automations,
  resourceTarget,
  toolNavigationIntent,
}: SharedWorkbenchRoutesProps) {
  const retentionKey = resourceTarget.kind === 'project' ? resourceTarget.projectId : 'global'
  return (
    <>
      {isMounted('market') && <WorkbenchRouteLayer visible={route === 'market'} loadingLabel={loadingLabel} retentionKey={retentionKey}><ResourceCenterView projectID={resourceTarget.kind === 'project' ? resourceTarget.projectId : undefined} visible={route === 'market'} /></WorkbenchRouteLayer>}
      {isMounted('books') && (
        <WorkbenchRouteLayer visible={route === 'books'} loadingLabel={loadingLabel} retentionKey={home.workspace}>
          <HomeView {...home} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('skills') && (
        <WorkbenchRouteLayer visible={route === 'skills'} loadingLabel={loadingLabel} retentionKey={retentionKey}>
          <SkillsView target={resourceTarget} toolNavigationIntent={toolNavigationIntent} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('agents') && (
        <WorkbenchRouteLayer visible={route === 'agents'} loadingLabel={loadingLabel} retentionKey={retentionKey}>
          <AgentsView target={resourceTarget} toolNavigationIntent={toolNavigationIntent} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('automations') && (
        <WorkbenchRouteLayer visible={route === 'automations'} loadingLabel={loadingLabel} retentionKey={retentionKey}>
          <AutomationsView {...automations} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('trajectory') && (
        <WorkbenchRouteLayer visible={route === 'trajectory'} loadingLabel={loadingLabel}>
          <TrajectoryPage />
        </WorkbenchRouteLayer>
      )}
      {isMounted('statistics') && (
        <WorkbenchRouteLayer visible={route === 'statistics'} loadingLabel={loadingLabel}>
          <StatisticsPage visible={route === 'statistics'} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('settings') && (
        <WorkbenchRouteLayer visible={route === 'settings'} loadingLabel={loadingLabel}>
          <SettingsView visible={route === 'settings'} />
        </WorkbenchRouteLayer>
      )}
    </>
  )
}
