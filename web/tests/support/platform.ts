import { cp, readFile, readdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, type APIRequestContext } from './fixtures'

/** Complete applications are explicit fixtures, separate from neutral initialization. */
export async function createExampleSource(request: APIRequestContext, example: string, kind: 'plugin' | 'game', id: string, name = { 'zh-CN': id, 'en-US': id }) {
  const response = await request.post('/api/agent-chat/projects/directory', { data: { name: id } })
  expect(response.ok(), await response.text()).toBe(true)
  const project = await response.json()
  const index = await (await request.get('/api/agent-chat/projects')).json()
  const directory = index.projects.find((item: { id: string }) => item.id === project.id).path
  for (const source of [example, 'common']) {
    const fixture = new URL('../../../internal/platform/templates/' + source + '/', import.meta.url)
    for (const entry of await readdir(fixture)) {
      await cp(new URL(entry, fixture), path.join(directory, entry), { recursive: true, force: false, errorOnExist: true })
    }
  }
  const manifestPath = path.join(directory, `denova.${kind}.json`)
  const manifest = JSON.parse(await readFile(manifestPath, 'utf8'))
  await writeFile(manifestPath, JSON.stringify({ ...manifest, id, name }))
  const sources = await (await request.get('/api/platform/manage/development')).json()
  const development = sources.find((source: { projectId: string }) => source.projectId === project.id)
  expect(development).toBeDefined()
  return development
}
