import { jsonHeaders, requestJSON } from './client'
import { GLOBAL_RESOURCE_TARGET, projectAPIPath, projectResourceTarget, resourceTargetKey } from './project-scope'
import type { ResourceTarget } from './project-scope'
import type { SkillCreateMetadata, SkillDocument, SkillFileDocument, SkillScope, SkillSnapshot, SkillPreferenceChange } from './types'

export interface SkillSaveTarget {
  scope: SkillScope
  name: string
}

/** Selects the global Skill catalog or the catalog merged with one Project layer. */
export type SkillCatalogTarget = ResourceTarget

export const GLOBAL_SKILL_TARGET: SkillCatalogTarget = GLOBAL_RESOURCE_TARGET

export function projectSkillTarget(projectId: string): SkillCatalogTarget {
  return projectResourceTarget(projectId)
}

export function skillCatalogTargetKey(target: SkillCatalogTarget): string {
  return resourceTargetKey(target)
}

function skillsPath(target: SkillCatalogTarget, suffix = ''): string {
  return target.kind === 'project'
    ? projectAPIPath(target.projectId, `skills${suffix}`)
    : `/api/skills${suffix}`
}

export async function getSkills(target: SkillCatalogTarget): Promise<SkillSnapshot> {
  const data = await requestJSON<SkillSnapshot>(skillsPath(target))
  return {
    scopes: data.scopes || [],
    skills: data.skills || [],
    shared_enabled: data.shared_enabled ?? false,
  }
}

export async function setSkillPreference(target: SkillCatalogTarget, change: SkillPreferenceChange): Promise<SkillSnapshot> {
  return requestJSON(skillsPath(target, '/preferences'), { method: 'PATCH', headers: jsonHeaders, body: JSON.stringify(change) })
}

export async function getSkillDocument(target: SkillCatalogTarget, scope: SkillScope, name: string): Promise<SkillDocument> {
  const query = new URLSearchParams({ scope, name })
  const data = await requestJSON<SkillDocument>(`${skillsPath(target, '/document')}?${query.toString()}`)
  return { ...data, files: data.files || [] }
}

export async function getSkillFileDocument(target: SkillCatalogTarget, scope: SkillScope, name: string, path: string): Promise<SkillFileDocument> {
  const query = new URLSearchParams({ scope, name, path })
  return requestJSON(`${skillsPath(target, '/file')}?${query.toString()}`)
}

export async function createSkill(target: SkillCatalogTarget, scope: SkillScope, name: string, description = '', agents: string[] = [], metadata: SkillCreateMetadata = {}): Promise<SkillDocument> {
  return requestJSON(skillsPath(target), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ scope, name, description, agents, category: metadata.category, capabilities: metadata.capabilities }),
  })
}

export async function saveSkillDocument(catalogTarget: SkillCatalogTarget, scope: SkillScope, name: string, content: string, target?: SkillSaveTarget, baseRevision?: string): Promise<SkillDocument> {
  const data = await requestJSON<SkillDocument>(skillsPath(catalogTarget, '/document'), {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({
      scope,
      name,
      content,
      target_scope: target?.scope,
      target_name: target?.name,
      base_revision: baseRevision,
    }),
  })
  return { ...data, files: data.files || [] }
}

export async function saveSkillFileDocument(target: SkillCatalogTarget, scope: SkillScope, name: string, path: string, content: string, baseRevision?: string): Promise<SkillFileDocument> {
  return requestJSON(skillsPath(target, '/file'), {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({ scope, name, path, content, base_revision: baseRevision }),
  })
}

export async function deleteSkillDocument(target: SkillCatalogTarget, scope: SkillScope, name: string): Promise<void> {
  const query = new URLSearchParams({ scope, name })
  await requestJSON(`${skillsPath(target, '/document')}?${query.toString()}`, { method: 'DELETE' })
}
