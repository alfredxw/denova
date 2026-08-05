import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { importCharacterCard, previewCharacterCard } from './workspace'

describe('character card API', () => {
  it('keeps preview global and binds existing-Book imports to a stable Project', async () => {
    const projectId = 'project-背景书籍'
    const visited: string[] = []
    server.use(
      http.post('/api/imports/character-card/preview', () => {
        visited.push('preview')
        return HttpResponse.json({ name: 'Character', entry_count: 0 })
      }),
      http.post('/api/projects/:projectId/book/import-character-card', async ({ params, request }) => {
        expect(params.projectId).toBe(projectId)
        const form = await request.formData()
        expect(form.get('lore_classification')).toBe('heuristic')
        expect(form.has('target_mode')).toBe(false)
        visited.push('project-import')
        return HttpResponse.json({ project_id: projectId, name: 'Character', item_ids: [] })
      }),
    )
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
    server.use(
      http.post('/api/books/import-character-card', async ({ request }) => {
        const form = await request.formData()
        expect(form.get('book_title')).toBe('New Book')
        return HttpResponse.json({ project_id: 'created-project', name: 'Character', item_ids: [] })
      }),
    )

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
