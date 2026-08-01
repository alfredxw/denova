import { useMemo } from 'react'
import type { AgentSkillSettings } from '@/features/settings/types'
import type { SkillSummary } from '@/lib/api'
import { skillAvailableForAgent } from '@/features/agents/agent-registry'
import { useAgentSkillCatalog } from './useAgentSkillCatalog'

export const DEFAULT_WRITING_SKILL = 'novel-lite'
export const BUILTIN_WRITING_SKILLS = ['novel-lite', 'novel-standard'] as const
export const WRITING_WORKFLOW_CAPABILITY = 'writing-workflow'

export type WritingSkillOption = Pick<SkillSummary, 'name' | 'description' | 'scope' | 'path' | 'active' | 'agent' | 'capabilities'>

export function resolveWritingSkillSelection(configured: string, options: WritingSkillOption[]): string {
  const selected = configured.trim()
  if (options.length === 0) return selected || DEFAULT_WRITING_SKILL
  if (selected && options.some((option) => option.name === selected)) return selected
  return options.find((option) => option.name === DEFAULT_WRITING_SKILL)?.name || options[0].name
}

export function useWritingSkillOptions(workspace?: string, enabled = true): WritingSkillOption[] {
  const catalog = useAgentSkillCatalog(workspace, enabled).data
  return useMemo(
    () => writingSkillOptionsFromSnapshot(catalog?.skills.skills || [], catalog?.settings.effective?.agent_skills),
    [catalog],
  )
}

export function writingSkillOptionsFromSnapshot(skills: SkillSummary[], agentSkills?: AgentSkillSettings): WritingSkillOption[] {
  const active = skills
    .filter((skill) => skill.active)
    .filter((skill) => skillAvailableForAgent(skill, 'ide', agentSkills))
    .filter((skill) => (skill.capabilities || []).includes(WRITING_WORKFLOW_CAPABILITY))
  return active.sort((a, b) => {
    if (a.name === DEFAULT_WRITING_SKILL || b.name === DEFAULT_WRITING_SKILL) return a.name === DEFAULT_WRITING_SKILL ? -1 : 1
    if (a.scope !== b.scope && (a.scope === 'builtin' || b.scope === 'builtin')) return a.scope === 'builtin' ? -1 : 1
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
