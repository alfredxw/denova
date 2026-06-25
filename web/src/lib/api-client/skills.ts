import { jsonHeaders, requestJSON } from './client'
import type { SkillDocument, SkillScope, SkillSnapshot } from './types'

export interface SkillSaveTarget {
  scope: SkillScope
  name: string
}

export async function getSkills(): Promise<SkillSnapshot> {
  const data = await requestJSON<SkillSnapshot>('/api/skills')
  return {
    scopes: data.scopes || [],
    skills: data.skills || [],
  }
}

export async function getSkillDocument(scope: SkillScope, name: string): Promise<SkillDocument> {
  const query = new URLSearchParams({ scope, name })
  return requestJSON(`/api/skills/document?${query.toString()}`)
}

export async function createSkill(scope: SkillScope, name: string, description = '', agents: string[] = []): Promise<SkillDocument> {
  return requestJSON('/api/skills', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ scope, name, description, agents }),
  })
}

export async function saveSkillDocument(scope: SkillScope, name: string, content: string, target?: SkillSaveTarget): Promise<SkillDocument> {
  return requestJSON('/api/skills/document', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({
      scope,
      name,
      content,
      target_scope: target?.scope,
      target_name: target?.name,
    }),
  })
}

export async function deleteSkillDocument(scope: SkillScope, name: string): Promise<void> {
  const query = new URLSearchParams({ scope, name })
  await requestJSON(`/api/skills/document?${query.toString()}`, { method: 'DELETE' })
}
