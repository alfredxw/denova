import { useEffect, useMemo } from 'react'
import { useQuery, type QueryClient } from '@tanstack/react-query'
import { fetchProjectSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import { getSkills, projectSkillTarget } from '@/lib/api'
import type { SkillSnapshot } from '@/lib/api'
import { queryClient } from '@/lib/query-client'

interface AgentSkillCatalog {
  settings: LayeredSettings
  skills: SkillSnapshot
}

type AgentSkillCatalogSubscription = {
  consumers: number
  onSkillsUpdated: (event: Event) => void
  onSettingsUpdated: (event: Event) => void
}

export const agentSkillCatalogKeys = {
  all: ['agent-skill-catalog'] as const,
  skills: () => [...agentSkillCatalogKeys.all, 'skills'] as const,
  skillsForProject: (projectId: string) => [...agentSkillCatalogKeys.skills(), projectId] as const,
  settings: () => [...agentSkillCatalogKeys.all, 'settings'] as const,
  settingsForProject: (projectId: string) => [...agentSkillCatalogKeys.settings(), projectId] as const,
}

const subscriptions = new WeakMap<QueryClient, AgentSkillCatalogSubscription>()

/**
 * Shares the Skills/settings catalog used to derive Agent commands.
 *
 * A workbench can keep several conversations mounted, and each conversation
 * consumes this catalog in more than one place. React Query owns one request
 * per Project while this module owns one pair of global invalidation
 * listeners per QueryClient, preventing a global event from creating an
 * N-conversation request fan-out.
 */
export function useAgentSkillCatalog(projectId: string, enabled = true) {
  const scope = projectId.trim()
  const skills = useQuery({
    queryKey: agentSkillCatalogKeys.skillsForProject(scope),
    queryFn: () => getSkills(projectSkillTarget(scope)),
    enabled: enabled && Boolean(scope),
  }, queryClient)
  const settings = useQuery({
    queryKey: agentSkillCatalogKeys.settingsForProject(scope),
    queryFn: () => fetchProjectSettings(scope),
    enabled: enabled && Boolean(scope),
  }, queryClient)

  useEffect(() => {
    if (!enabled) return
    return subscribeAgentSkillCatalogEvents(queryClient)
  }, [enabled])
  const data = useMemo<AgentSkillCatalog | undefined>(
    () => enabled && scope && skills.data && settings.data ? { skills: skills.data, settings: settings.data } : undefined,
    [enabled, scope, settings.data, skills.data],
  )
  return { data }
}

function subscribeAgentSkillCatalogEvents(queryClient: QueryClient) {
  const existing = subscriptions.get(queryClient)
  if (existing) {
    existing.consumers += 1
    return () => releaseAgentSkillCatalogSubscription(queryClient, existing)
  }

  const subscription: AgentSkillCatalogSubscription = {
    consumers: 1,
    onSkillsUpdated: (event) => {
      const targetKey = (event as CustomEvent<{ targetKey?: string }>).detail?.targetKey
      const projectId = targetKey?.startsWith('project:') ? targetKey.slice('project:'.length) : ''
      void queryClient.invalidateQueries({
        queryKey: projectId
          ? agentSkillCatalogKeys.skillsForProject(projectId)
          : agentSkillCatalogKeys.skills(),
      })
    },
    onSettingsUpdated: (event) => {
      const projectId = (event as CustomEvent<{ projectId?: string }>).detail?.projectId?.trim()
      void queryClient.invalidateQueries({
        queryKey: projectId
          ? agentSkillCatalogKeys.settingsForProject(projectId)
          : agentSkillCatalogKeys.settings(),
      })
    },
  }
  subscriptions.set(queryClient, subscription)
  window.addEventListener('nova:skills-updated', subscription.onSkillsUpdated)
  window.addEventListener('nova:settings-updated', subscription.onSettingsUpdated)
  return () => releaseAgentSkillCatalogSubscription(queryClient, subscription)
}

function releaseAgentSkillCatalogSubscription(queryClient: QueryClient, subscription: AgentSkillCatalogSubscription) {
  subscription.consumers -= 1
  if (subscription.consumers > 0) return
  window.removeEventListener('nova:skills-updated', subscription.onSkillsUpdated)
  window.removeEventListener('nova:settings-updated', subscription.onSettingsUpdated)
  subscriptions.delete(queryClient)
}
