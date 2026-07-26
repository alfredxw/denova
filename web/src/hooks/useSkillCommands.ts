import { useEffect, useState } from 'react'
import { fetchSettings } from '@/features/settings/api'
import type { AgentModelSettings, ResolvedAgentToolCapability } from '@/features/settings/types'
import { getSkills } from '@/lib/api'
import type { SkillSummary } from '@/lib/api'
import { skillAvailableForAgent } from '@/features/agents/agent-registry'

type SkillAgentKey = Exclude<keyof AgentModelSettings, 'default'>

interface UseSkillCommandsOptions {
  agentKey: SkillAgentKey
  workspace?: string
}

export function useSkillCommands({
  agentKey,
  workspace,
}: UseSkillCommandsOptions): Array<Pick<SkillSummary, 'name' | 'description'>> {
  const [skillCommands, setSkillCommands] = useState<Array<Pick<SkillSummary, 'name' | 'description'>>>([])

  useEffect(() => {
    let cancelled = false
    let requestSeq = 0
    const loadSkills = () => {
      const requestId = ++requestSeq
      Promise.all([getSkills(), fetchSettings()])
        .then(([data, settings]) => {
          if (cancelled || requestId !== requestSeq) return
          if (!agentSkillsEnabled(settings.resolved_agent_tool_manifests[agentKey])) {
            setSkillCommands([])
            return
          }
          setSkillCommands(data.skills
            .filter((skill) => skill.active)
            .filter((skill) => skillAvailableForAgent(skill, agentKey, settings.effective?.agent_skills))
            .map((skill) => ({ name: skill.name, description: skill.description })))
        })
        .catch((error) => {
          console.warn('[skills] load skill commands failed', { agentKey, error })
          if (!cancelled && requestId === requestSeq) setSkillCommands([])
        })
    }
    loadSkills()
    window.addEventListener('nova:skills-updated', loadSkills)
    window.addEventListener('nova:settings-updated', loadSkills)
    return () => {
      cancelled = true
      window.removeEventListener('nova:skills-updated', loadSkills)
      window.removeEventListener('nova:settings-updated', loadSkills)
    }
  }, [agentKey, workspace])

  return skillCommands
}

function agentSkillsEnabled(manifest: ResolvedAgentToolCapability[] | undefined) {
  const capability = manifest?.find((entry) => entry.capability === 'skills')
  return capability?.allowed === true && capability.availability !== 'unavailable'
}
