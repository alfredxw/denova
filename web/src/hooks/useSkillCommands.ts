import { useMemo } from 'react'
import type { AgentModelSettings, ResolvedAgentToolCapability } from '@/features/settings/types'
import type { SkillSummary } from '@/lib/api'
import { skillAvailableForAgent } from '@/features/agents/agent-registry'
import { useAgentSkillCatalog } from './useAgentSkillCatalog'

type SkillAgentKey = Exclude<keyof AgentModelSettings, 'default'>

interface UseSkillCommandsOptions {
  agentKey: SkillAgentKey
  workspace?: string
  enabled?: boolean
}

export function useSkillCommands({
  agentKey,
  workspace,
  enabled = true,
}: UseSkillCommandsOptions): Array<Pick<SkillSummary, 'name' | 'description'>> {
  const catalog = useAgentSkillCatalog(workspace, enabled).data
  return useMemo(() => {
    if (!catalog || !agentSkillsEnabled(catalog.settings.resolved_agent_tool_manifests?.[agentKey])) return []
    return catalog.skills.skills
      .filter((skill) => skill.active)
      .filter((skill) => skillAvailableForAgent(skill, agentKey, catalog.settings.effective?.agent_skills))
      .map((skill) => ({ name: skill.name, description: skill.description }))
  }, [agentKey, catalog])
}

function agentSkillsEnabled(manifest: ResolvedAgentToolCapability[] | undefined) {
  const capability = manifest?.find((entry) => entry.capability === 'skills')
  return capability?.allowed === true && capability.availability !== 'unavailable'
}
