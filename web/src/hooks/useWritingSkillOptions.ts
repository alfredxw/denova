import { useEffect, useState } from 'react'
import { fetchSettings } from '@/features/settings/api'
import type { AgentSkillSettings } from '@/features/settings/types'
import { getSkills } from '@/lib/api'
import type { SkillSummary } from '@/lib/api'
import { skillAvailableForAgent } from '@/features/agents/agent-registry'

export const DEFAULT_WRITING_SKILL = 'novel-standard'
export const BUILTIN_WRITING_SKILLS = ['novel-lite', 'novel-standard', 'novel-heavy'] as const

const builtinWritingSkillOrder = new Map(BUILTIN_WRITING_SKILLS.map((name, index) => [name, index]))

export type WritingSkillOption = Pick<SkillSummary, 'name' | 'description' | 'scope' | 'path' | 'active' | 'agent'>

export function useWritingSkillOptions(workspace?: string): WritingSkillOption[] {
  const [options, setOptions] = useState<WritingSkillOption[]>([])

  useEffect(() => {
    let cancelled = false
    const load = () => {
      Promise.all([getSkills(), fetchSettings()])
        .then(([snapshot, settings]) => {
          if (cancelled) return
          setOptions(writingSkillOptionsFromSnapshot(snapshot.skills || [], settings.effective?.agent_skills))
        })
        .catch((error) => {
          console.warn('[skills] load writing skill options failed', { error })
          if (!cancelled) setOptions([])
        })
    }
    load()
    window.addEventListener('nova:skills-updated', load)
    window.addEventListener('nova:settings-updated', load)
    return () => {
      cancelled = true
      window.removeEventListener('nova:skills-updated', load)
      window.removeEventListener('nova:settings-updated', load)
    }
  }, [workspace])

  return options
}

export function writingSkillOptionsFromSnapshot(skills: SkillSummary[], agentSkills?: AgentSkillSettings): WritingSkillOption[] {
  const active = skills
    .filter((skill) => skill.active)
    .filter((skill) => skillAvailableForAgent(skill, 'ide', agentSkills))
    .filter((skill) => {
      if (skill.scope === 'builtin') return builtinWritingSkillOrder.has(skill.name as typeof BUILTIN_WRITING_SKILLS[number])
      return true
    })
  return active.sort((a, b) => {
    const aBuiltin = builtinWritingSkillOrder.get(a.name as typeof BUILTIN_WRITING_SKILLS[number])
    const bBuiltin = builtinWritingSkillOrder.get(b.name as typeof BUILTIN_WRITING_SKILLS[number])
    if (aBuiltin !== undefined || bBuiltin !== undefined) {
      if (aBuiltin === undefined) return 1
      if (bBuiltin === undefined) return -1
      return aBuiltin - bBuiltin
    }
    if (a.name !== b.name) return a.name.localeCompare(b.name)
    return sourceRank(b.scope) - sourceRank(a.scope)
  })
}

function sourceRank(scope: string) {
  switch (scope) {
    case 'workspace':
      return 3
    case 'user':
      return 2
    case 'builtin':
      return 1
    default:
      return 0
  }
}
