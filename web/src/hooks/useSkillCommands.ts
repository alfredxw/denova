import { useEffect, useState } from 'react'
import { fetchSettings } from '@/features/settings/api'
import type { AgentToolOverride, AgentToolSettings } from '@/features/settings/types'
import { getSkills } from '@/lib/api'
import type { SkillSummary } from '@/lib/api'
import { skillAvailableForAgent } from '@/features/agents/agent-registry'

type SkillAgentKey = Exclude<keyof AgentToolSettings, 'default'>

interface UseSkillCommandsOptions {
  agentKey: SkillAgentKey
  workspace?: string
  fallbackEnabled?: boolean
}

export function useSkillCommands({
  agentKey,
  workspace,
  fallbackEnabled = false,
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
          if (!agentSkillsEnabled(settings.effective?.agent_tools, agentKey, fallbackEnabled)) {
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
  }, [agentKey, fallbackEnabled, workspace])

  return skillCommands
}

function agentSkillsEnabled(settings: AgentToolSettings | undefined, agentKey: SkillAgentKey, fallbackEnabled: boolean) {
  const defaultTools: AgentToolOverride = settings?.default ?? {}
  const agentTools: AgentToolOverride = settings?.[agentKey] ?? {}
  return agentTools.skills ?? defaultTools.skills ?? fallbackEnabled
}
