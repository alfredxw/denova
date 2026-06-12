import { jsonHeaders, requestJSON } from './client'
import type { SkillDocument, SkillScope, SkillSnapshot } from './types'

export async function getStyles(): Promise<string[]> {
  const data = await requestJSON<{ styles: string[] }>('/api/styles')
  return data.styles || []
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

export async function saveSkillDocument(scope: SkillScope, name: string, content: string): Promise<SkillDocument> {
  return requestJSON('/api/skills/document', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({ scope, name, content }),
  })
}
