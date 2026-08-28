import { expect, type APIRequestContext, type APIResponse } from '@playwright/test'

export interface E2EBook {
  projectId: string
  workspace: string
}

export interface E2EStory {
  id: string
}

export interface E2EStoryTurn {
  user?: string
  narrative?: string
}

async function expectSuccessful(response: APIResponse): Promise<void> {
  const failureDetails = response.ok() ? undefined : await response.text()
  expect(response.ok(), failureDetails).toBe(true)
}

export async function createAndOpenBook(request: APIRequestContext, title: string): Promise<E2EBook> {
  const created = await request.post('/api/books/create', {
    data: { title, author: 'Denova E2E' },
  })
  await expectSuccessful(created)
  const body = await created.json() as { project_id: string; workspace: string }
  const switched = await request.post('/api/workspace/switch', { data: { path: body.workspace } })
  await expectSuccessful(switched)
  return { projectId: body.project_id, workspace: body.workspace }
}

export async function createProjectFile(
  request: APIRequestContext,
  projectId: string,
  path: string,
  content: string,
): Promise<void> {
  const response = await request.post(`/api/projects/${encodeURIComponent(projectId)}/files/operations`, {
    data: { operations: [{ id: 'seed', kind: 'create', path, type: 'file', content }] },
  })
  await expectSuccessful(response)
  const body = await response.json() as { results?: Array<{ ok: boolean; error?: string }> }
  expect(body.results?.[0]).toMatchObject({ ok: true })
}

export async function readProjectFile(
  request: APIRequestContext,
  projectId: string,
  path: string,
): Promise<{ content: string; revision: string }> {
  const query = new URLSearchParams({ path })
  const response = await request.get(`/api/projects/${encodeURIComponent(projectId)}/files/file?${query}`)
  await expectSuccessful(response)
  return response.json()
}

export async function saveProjectFile(
  request: APIRequestContext,
  projectId: string,
  path: string,
  content: string,
): Promise<void> {
  const current = await readProjectFile(request, projectId, path)
  const response = await request.put(`/api/projects/${encodeURIComponent(projectId)}/files/file`, {
    data: { path, content, base_revision: current.revision },
  })
  await expectSuccessful(response)
}

export async function getCurrentWorkspace(
  request: APIRequestContext,
): Promise<{ workspace: string; project_id: string; has_state: boolean }> {
  const response = await request.get('/api/workspace/current')
  await expectSuccessful(response)
  return response.json()
}

export async function getProjectLoreItems(
  request: APIRequestContext,
  projectId: string,
): Promise<Array<{ id: string; name: string; content: string }>> {
  const response = await request.get(`/api/projects/${encodeURIComponent(projectId)}/book/lore/items`)
  await expectSuccessful(response)
  const body = await response.json() as { items?: Array<{ id: string; name: string; content: string }> }
  return body.items ?? []
}

export async function createStory(request: APIRequestContext, title: string): Promise<E2EStory> {
  const response = await request.post('/api/interactive/stories', {
    data: {
      title,
      origin: '一扇石门挡在旧车站入口。',
      choice_count: 2,
      director_run_policy: { mode: 'manual' },
      state_schema_policy: { mode: 'fixed_template' },
    },
  })
  await expectSuccessful(response)
  return response.json()
}

export async function getStorySnapshot(
  request: APIRequestContext,
  storyId: string,
  branch = 'main',
): Promise<{ turns: E2EStoryTurn[] }> {
  const query = new URLSearchParams({ branch })
  const response = await request.get(`/api/interactive/stories/${encodeURIComponent(storyId)}/snapshot?${query}`)
  await expectSuccessful(response)
  const body = await response.json() as { turns?: E2EStoryTurn[] }
  return { turns: body.turns ?? [] }
}

export async function getStoryBranches(
  request: APIRequestContext,
  storyId: string,
): Promise<Array<{ id: string; title: string; current?: boolean }>> {
  const response = await request.get(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches`)
  await expectSuccessful(response)
  const body = await response.json() as { branches?: Array<{ id: string; title: string; current?: boolean }> }
  return body.branches ?? []
}
