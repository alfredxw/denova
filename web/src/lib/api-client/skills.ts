import { jsonHeaders, requestJSON } from './client'
import type { SkillDocument, SkillInstallPreview, SkillInstallResult, SkillScope, SkillSnapshot } from './types'

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

export async function previewSkillZipInstall(file: File, scope: SkillScope): Promise<SkillInstallPreview> {
  const form = new FormData()
  form.append('file', file)
  form.append('scope', scope)
  const data = await requestJSON<SkillInstallPreview>('/api/skills/install/zip/preview', {
    method: 'POST',
    body: form,
  })
  return { candidates: data.candidates || [] }
}

export async function installSkillZip(file: File, scope: SkillScope, candidateIds: string[]): Promise<SkillInstallResult> {
  const form = new FormData()
  form.append('file', file)
  form.append('scope', scope)
  form.append('candidate_ids', JSON.stringify(candidateIds))
  const data = await requestJSON<SkillInstallResult>('/api/skills/install/zip', {
    method: 'POST',
    body: form,
  })
  return { installed: data.installed || [] }
}

export async function previewSkillGitHubInstall(input: {
  url: string
  ref?: string
  subdir?: string
  scope: SkillScope
}): Promise<SkillInstallPreview> {
  const data = await requestJSON<SkillInstallPreview>('/api/skills/install/github/preview', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      url: input.url,
      ref: input.ref || '',
      subdir: input.subdir || '',
      scope: input.scope,
    }),
  })
  return { candidates: data.candidates || [] }
}

export async function installSkillGitHub(input: {
  url: string
  ref?: string
  subdir?: string
  scope: SkillScope
  candidateIds: string[]
}): Promise<SkillInstallResult> {
  const data = await requestJSON<SkillInstallResult>('/api/skills/install/github', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      url: input.url,
      ref: input.ref || '',
      subdir: input.subdir || '',
      scope: input.scope,
      candidate_ids: input.candidateIds,
    }),
  })
  return { installed: data.installed || [] }
}
