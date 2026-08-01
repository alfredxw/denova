import { useEffect, useMemo } from 'react'
import { useQuery, type QueryClient } from '@tanstack/react-query'
import { fetchSettings, refreshSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import { getSkills } from '@/lib/api'
import type { SkillSnapshot } from '@/lib/api'
import { queryClient } from '@/lib/query-client'

interface AgentSkillCatalog {
  settings: LayeredSettings
  skills: SkillSnapshot
}

type AgentSkillCatalogSubscription = {
  consumers: number
  onSkillsUpdated: () => void
  onSettingsUpdated: () => void
}

export const agentSkillCatalogKeys = {
  all: ['agent-skill-catalog'] as const,
  skills: () => [...agentSkillCatalogKeys.all, 'skills'] as const,
  skillsForWorkspace: (workspace: string) => [...agentSkillCatalogKeys.skills(), workspace] as const,
  settings: () => [...agentSkillCatalogKeys.all, 'settings'] as const,
  settingsForWorkspace: (workspace: string) => [...agentSkillCatalogKeys.settings(), workspace] as const,
}

const subscriptions = new WeakMap<QueryClient, AgentSkillCatalogSubscription>()

/**
 * Shares the Skills/settings catalog used to derive Agent commands.
 *
 * A workbench can keep several conversations mounted, and each conversation
 * consumes this catalog in more than one place. React Query owns one request
 * per workspace while this module owns one pair of global invalidation
 * listeners per QueryClient, preventing a global event from creating an
 * N-conversation request fan-out.
 */
export function useAgentSkillCatalog(workspace?: string, enabled = true) {
  const scope = workspace?.trim() || ''
  const skills = useQuery({
    queryKey: agentSkillCatalogKeys.skillsForWorkspace(scope),
    queryFn: getSkills,
    enabled,
  }, queryClient)
  const settings = useQuery({
    queryKey: agentSkillCatalogKeys.settingsForWorkspace(scope),
    queryFn: fetchSettings,
    enabled,
  }, queryClient)

  useEffect(() => {
    if (!enabled) return
    return subscribeAgentSkillCatalogEvents(queryClient)
  }, [enabled])
  const data = useMemo<AgentSkillCatalog | undefined>(
    () => enabled && skills.data && settings.data ? { skills: skills.data, settings: settings.data } : undefined,
    [enabled, settings.data, skills.data],
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
    onSkillsUpdated: () => {
      void queryClient.invalidateQueries({ queryKey: agentSkillCatalogKeys.skills() })
    },
    onSettingsUpdated: () => {
      void refreshSettings().then(
        (snapshot) => { queryClient.setQueriesData({ queryKey: agentSkillCatalogKeys.settings() }, snapshot) },
        () => { void queryClient.invalidateQueries({ queryKey: agentSkillCatalogKeys.settings() }) },
      )
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
