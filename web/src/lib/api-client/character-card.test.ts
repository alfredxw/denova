import { afterEach, describe, expect, it, vi } from 'vitest'
import { importCharacterCard, previewCharacterCard } from './workspace'

describe('character card API', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps preview global and binds existing-Book imports to a stable Project', async () => {
    const projectId = 'project-背景书籍'
    const visited: string[] = []
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = requestPath(input)
      if (path === '/api/imports/character-card/preview') {
        visited.push('preview')
        return jsonResponse({ name: 'Character', entry_count: 0 })
      }
      if (path === `/api/projects/${encodeURIComponent(projectId)}/book/import-character-card`) {
        const form = init?.body as FormData
        expect(form.get('lore_classification')).toBe('heuristic')
        expect(form.has('target_mode')).toBe(false)
        visited.push('project-import')
        return jsonResponse({ project_id: projectId, name: 'Character', item_ids: [] })
      }
      throw new Error(`Unexpected character-card request: ${path}`)
    })
    const file = new File(['{}'], 'character.json', { type: 'application/json' })

    await previewCharacterCard(file)
    await importCharacterCard(file, {
      targetMode: 'current',
      projectId,
      loreClassification: 'heuristic',
    })

    expect(visited).toEqual(['preview', 'project-import'])
  })

  it('uses the global Book-creation endpoint only for new-Book imports', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = requestPath(input)
      expect(path).toBe('/api/books/import-character-card')
      const form = init?.body as FormData
      expect(form.get('book_title')).toBe('New Book')
      return jsonResponse({ project_id: 'created-project', name: 'Character', item_ids: [] })
    })

    await importCharacterCard(new File(['{}'], 'character.json'), {
      targetMode: 'new_book',
      bookTitle: 'New Book',
    })
  })

  it('rejects an existing-Book import without Project identity before sending a request', async () => {
    await expect(importCharacterCard(new File(['{}'], 'character.json'), {
      targetMode: 'current',
    })).rejects.toThrow('Project ID is required')
  })
})

function requestPath(input: RequestInfo | URL): string {
  return typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
}
